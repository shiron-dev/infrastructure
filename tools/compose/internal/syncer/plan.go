package syncer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"cmt/internal/config"
	"cmt/internal/remote"

	"github.com/pmezard/go-difflib/difflib"
)

// PlanDependencies holds injectable dependencies for BuildPlan.
type PlanDependencies struct {
	ClientFactory remote.ClientFactory
	SSHResolver   config.SSHConfigResolver
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

// ActionType describes what will happen to a file.
type ActionType int

const (
	ActionUnchanged ActionType = iota
	ActionAdd
	ActionModify
	ActionDelete
)

func (a ActionType) String() string {
	switch a {
	case ActionUnchanged:
		return "unchanged"
	case ActionAdd:
		return "add"
	case ActionModify:
		return "modify"
	case ActionDelete:
		return "delete"
	default:
		return "unknown"
	}
}

func (a ActionType) Symbol() string {
	switch a {
	case ActionUnchanged:
		return "="
	case ActionAdd:
		return "+"
	case ActionModify:
		return "~"
	case ActionDelete:
		return "-"
	default:
		return "?"
	}
}

// ---------------------------------------------------------------------------
// Plan data structures
// ---------------------------------------------------------------------------

// SyncPlan is the complete plan for all hosts and projects.
type SyncPlan struct {
	HostPlans []HostPlan
}

// HostPlan groups project plans for one host.
type HostPlan struct {
	Host     config.HostEntry
	Projects []ProjectPlan
}

// ProjectPlan describes what to sync for one project on one host.
type ProjectPlan struct {
	ProjectName     string
	RemoteDir       string // <remotePath>/<project>
	PostSyncCommand string
	Dirs            []DirPlan
	Files           []FilePlan
}

// DirPlan describes the planned action for a pre-created directory.
type DirPlan struct {
	RelativePath string // relative to remote project dir
	RemotePath   string // absolute remote path
	Exists       bool   // whether it already exists on the remote host
}

// FilePlan describes the planned action for a single file.
type FilePlan struct {
	RelativePath string // relative to remote project dir
	LocalPath    string // absolute local path (empty for deletes)
	RemotePath   string // absolute remote path
	Action       ActionType
	LocalData    []byte
	RemoteData   []byte
	Diff         string // unified diff (text files only)
}

// Manifest tracks files managed by cmt on the remote host.
type Manifest struct {
	ManagedFiles []string `json:"managedFiles"`
}

const manifestFile = ".cmt-manifest.json"

// ---------------------------------------------------------------------------
// Plan statistics
// ---------------------------------------------------------------------------

// Stats returns counts of each action type across the entire plan.
func (p *SyncPlan) Stats() (int, int, int, int, int, int) {
	hostCount := len(p.HostPlans)
	projectCount := 0
	addCount := 0
	modifyCount := 0
	deleteCount := 0
	unchangedCount := 0

	for _, hostPlan := range p.HostPlans {
		projectCount += len(hostPlan.Projects)

		for _, projectPlan := range hostPlan.Projects {
			for _, filePlan := range projectPlan.Files {
				switch filePlan.Action {
				case ActionAdd:
					addCount++
				case ActionModify:
					modifyCount++
				case ActionDelete:
					deleteCount++
				case ActionUnchanged:
					unchangedCount++
				}
			}
		}
	}

	return hostCount, projectCount, addCount, modifyCount, deleteCount, unchangedCount
}

// DirStats returns counts of directories to create and already existing.
func (p *SyncPlan) DirStats() (int, int) {
	toCreateCount := 0
	existingCount := 0

	for _, hostPlan := range p.HostPlans {
		for _, projectPlan := range hostPlan.Projects {
			for _, directoryPlan := range projectPlan.Dirs {
				if directoryPlan.Exists {
					existingCount++
				} else {
					toCreateCount++
				}
			}
		}
	}

	return toCreateCount, existingCount
}

// HasChanges returns true if the plan contains any add/modify/delete actions
// or directories to create.
func (p *SyncPlan) HasChanges() bool {
	hostCount, projectCount, add, mod, del, unchangedCount := p.Stats()
	_ = hostCount
	_ = projectCount
	_ = unchangedCount
	dirCreate, _ := p.DirStats()

	return add+mod+del+dirCreate > 0
}

// ---------------------------------------------------------------------------
// Plan display
// ---------------------------------------------------------------------------

// Print writes a human-readable plan to w.
func (p *SyncPlan) Print(w io.Writer) {
	if len(p.HostPlans) == 0 {
		_, _ = fmt.Fprintln(w, "No hosts selected.")

		return
	}

	for _, hostPlan := range p.HostPlans {
		_, _ = fmt.Fprintf(w, "\n=== Host: %s (%s@%s:%d) ===\n",
			hostPlan.Host.Name, hostPlan.Host.User, hostPlan.Host.Host, hostPlan.Host.Port)

		if len(hostPlan.Projects) == 0 {
			_, _ = fmt.Fprintln(w, "  (no projects)")

			continue
		}

		for _, projectPlan := range hostPlan.Projects {
			_, _ = fmt.Fprintf(w, "\n  Project: %s\n", projectPlan.ProjectName)
			_, _ = fmt.Fprintf(w, "    Remote: %s\n", projectPlan.RemoteDir)

			if projectPlan.PostSyncCommand != "" {
				_, _ = fmt.Fprintf(w, "    Post-sync: %s\n", projectPlan.PostSyncCommand)
			}

			_, _ = fmt.Fprintln(w)

			// Show directory plans.
			if len(projectPlan.Dirs) > 0 {
				_, _ = fmt.Fprintln(w, "    Dirs:")

				for _, directoryPlan := range projectPlan.Dirs {
					if directoryPlan.Exists {
						_, _ = fmt.Fprintf(w, "      = %s/ (exists)\n", directoryPlan.RelativePath)
					} else {
						_, _ = fmt.Fprintf(w, "      + %s/ (create)\n", directoryPlan.RelativePath)
					}
				}

				_, _ = fmt.Fprintln(w)
			}

			if len(projectPlan.Files) == 0 && len(projectPlan.Dirs) == 0 {
				_, _ = fmt.Fprintln(w, "    (no files or dirs)")

				continue
			}

			for _, filePlan := range projectPlan.Files {
				label := ""

				switch filePlan.Action {
				case ActionAdd:
					label = "new, " + humanSize(len(filePlan.LocalData))
				case ActionModify:
					label = "modified"
				case ActionDelete:
					label = "delete"
				case ActionUnchanged:
					label = "unchanged"
				}

				_, _ = fmt.Fprintf(w, "    %s %s (%s)\n", filePlan.Action.Symbol(), filePlan.RelativePath, label)

				if filePlan.Diff != "" {
					for line := range strings.SplitSeq(filePlan.Diff, "\n") {
						if line != "" {
							_, _ = fmt.Fprintf(w, "        %s\n", line)
						}
					}
				}
			}
		}
	}

	hosts, projects, add, mod, del, unch := p.Stats()
	dirCreate, _ := p.DirStats()

	_, _ = fmt.Fprintf(w, "\nSummary: %d host(s), %d project(s) — %d to add, %d to modify, %d to delete, %d unchanged",
		hosts, projects, add, mod, del, unch)

	if dirCreate > 0 {
		_, _ = fmt.Fprintf(w, ", %d dir(s) to create", dirCreate)
	}

	_, _ = fmt.Fprintln(w)
}

// ---------------------------------------------------------------------------
// Build plan
// ---------------------------------------------------------------------------

// BuildPlan connects to each selected host and computes the diff.
func BuildPlan(cfg *config.CmtConfig, hostFilter, projectFilter []string) (*SyncPlan, error) {
	var dependencies PlanDependencies

	return BuildPlanWithDeps(cfg, hostFilter, projectFilter, dependencies)
}

// BuildPlanWithDeps connects to each selected host using injected dependencies.
func BuildPlanWithDeps(cfg *config.CmtConfig, hostFilter, projectFilter []string, deps PlanDependencies) (*SyncPlan, error) {
	clientFactory := deps.ClientFactory
	if clientFactory == nil {
		defaultFactory := new(remote.DefaultClientFactory)
		defaultFactory.Runner = nil
		clientFactory = *defaultFactory
	}

	sshResolver := deps.SSHResolver
	if sshResolver == nil {
		defaultResolver := new(config.DefaultSSHConfigResolver)
		defaultResolver.Runner = nil
		sshResolver = *defaultResolver
	}

	// Discover and filter projects.
	allProjects, err := config.DiscoverProjects(cfg.BasePath)
	if err != nil {
		return nil, err
	}

	projects := config.FilterProjects(allProjects, projectFilter)
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found (filter: %v)", projectFilter)
	}

	hosts := config.FilterHosts(cfg.Hosts, hostFilter)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no hosts matched filter %v", hostFilter)
	}

	plan := new(SyncPlan)
	plan.HostPlans = nil

	for _, host := range hosts {
		// Load host config.
		hostCfg, err := config.LoadHostConfig(cfg.BasePath, host.Name)
		if err != nil && !errors.Is(err, config.ErrHostConfigNotFound) {
			return nil, fmt.Errorf("loading host config for %s: %w", host.Name, err)
		}

		if errors.Is(err, config.ErrHostConfigNotFound) {
			hostCfg = nil
		}

		// Resolve SSH config via ssh -G (always runs; uses -F when
		// host.yml specifies sshConfig, otherwise default ssh config).
		sshConfigPath := ""
		if hostCfg != nil {
			sshConfigPath = hostCfg.SSHConfig
		}

		hostDir := filepath.Join(cfg.BasePath, "hosts", host.Name)

		resolveErr := sshResolver.Resolve(&host, sshConfigPath, hostDir)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolving SSH config for %s: %w", host.Name, resolveErr)
		}

		// Connect via SSH/SFTP.
		client, err := clientFactory.NewClient(host)
		if err != nil {
			return nil, fmt.Errorf("connecting to host %s: %w", host.Name, err)
		}

		hostPlan, err := buildHostPlan(cfg, host, hostCfg, projects, client)
		_ = client.Close()

		if err != nil {
			return nil, err
		}

		plan.HostPlans = append(plan.HostPlans, *hostPlan)
	}

	return plan, nil
}

func buildHostPlan(
	cfg *config.CmtConfig,
	host config.HostEntry,
	hostCfg *config.HostConfig,
	projects []string,
	client remote.RemoteClient,
) (*HostPlan, error) {
	hostPlan := new(HostPlan)
	hostPlan.Host = host
	hostPlan.Projects = nil

	for _, project := range projects {
		resolved := config.ResolveProjectConfig(cfg.Defaults, hostCfg, project)
		if resolved.RemotePath == "" {
			return nil, fmt.Errorf("remotePath is not set for host %q, project %q", host.Name, project)
		}

		remoteDir := path.Join(resolved.RemotePath, project)

		// Build directory plans.
		dirPlans := make([]DirPlan, 0, len(resolved.Dirs))

		for _, d := range resolved.Dirs {
			absDir := path.Join(remoteDir, d)
			_, statErr := client.Stat(absDir)
			dirPlans = append(dirPlans, DirPlan{
				RelativePath: d,
				RemotePath:   absDir,
				Exists:       statErr == nil,
			})
		}

		// Load template variables from host-side .env / env.secrets.yml.
		templateVars, err := LoadTemplateVars(cfg.BasePath, host.Name, project)
		if err != nil {
			return nil, fmt.Errorf("loading template vars for %s/%s: %w", host.Name, project, err)
		}

		// Collect local files.
		localFiles, err := CollectLocalFiles(cfg.BasePath, host.Name, project)
		if err != nil {
			return nil, fmt.Errorf("collecting files for %s/%s: %w", host.Name, project, err)
		}

		// Read remote manifest.
		manifest := readManifest(client, remoteDir)

		// Build file plans.
		filePlans, err := buildFilePlans(localFiles, remoteDir, manifest, client, templateVars)
		if err != nil {
			return nil, fmt.Errorf("building file plan for %s/%s: %w", host.Name, project, err)
		}

		hostPlan.Projects = append(hostPlan.Projects, ProjectPlan{
			ProjectName:     project,
			RemoteDir:       remoteDir,
			PostSyncCommand: resolved.PostSyncCommand,
			Dirs:            dirPlans,
			Files:           filePlans,
		})
	}

	return hostPlan, nil
}

func buildFilePlans(
	localFiles map[string]string,
	remoteDir string,
	manifest *Manifest,
	client remote.RemoteClient,
	templateVars map[string]any,
) ([]FilePlan, error) {
	plans := make([]FilePlan, 0, len(localFiles))

	localSet := make(map[string]bool, len(localFiles))

	for relPath, localPath := range localFiles {
		localSet[relPath] = true

		cleanLocalPath := filepath.Clean(localPath)

		localData, err := os.ReadFile(cleanLocalPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", cleanLocalPath, err)
		}

		// Render file as Go template.
		localData, err = RenderTemplate(localData, templateVars)
		if err != nil {
			return nil, fmt.Errorf("rendering template %s: %w", cleanLocalPath, err)
		}

		remotePath := path.Join(remoteDir, relPath)
		remoteData, readErr := client.ReadFile(remotePath)

		filePlan := new(FilePlan)
		filePlan.RelativePath = relPath
		filePlan.LocalPath = localPath
		filePlan.RemotePath = remotePath
		filePlan.Action = ActionUnchanged
		filePlan.LocalData = localData
		filePlan.RemoteData = nil
		filePlan.Diff = ""

		switch {
		case readErr != nil:
			// Remote file does not exist.
			filePlan.Action = ActionAdd
		case bytes.Equal(localData, remoteData):
			filePlan.Action = ActionUnchanged
			filePlan.RemoteData = remoteData
		default:
			filePlan.Action = ActionModify

			filePlan.RemoteData = remoteData

			if !isBinary(localData) && !isBinary(remoteData) {
				filePlan.Diff = computeDiff(relPath, remoteData, localData)
			}
		}

		plans = append(plans, *filePlan)
	}

	// Files in manifest but not in local → delete.
	if manifest != nil {
		for _, managedFile := range manifest.ManagedFiles {
			if managedFile == manifestFile {
				continue
			}

			if localSet[managedFile] {
				continue
			}

			remotePath := path.Join(remoteDir, managedFile)
			remoteData, _ := client.ReadFile(remotePath)
			deletePlan := new(FilePlan)
			deletePlan.RelativePath = managedFile
			deletePlan.LocalPath = ""
			deletePlan.RemotePath = remotePath
			deletePlan.Action = ActionDelete
			deletePlan.LocalData = nil
			deletePlan.RemoteData = remoteData
			deletePlan.Diff = ""
			plans = append(plans, *deletePlan)
		}
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].RelativePath < plans[j].RelativePath
	})

	return plans, nil
}

// ---------------------------------------------------------------------------
// Local file collection
// ---------------------------------------------------------------------------

// CollectLocalFiles gathers all files that should be synced for one
// (host, project) pair. The returned map is relativePath → absoluteLocalPath.
// Host-level files override project-level files with the same relative path.
func CollectLocalFiles(basePath, hostName, projectName string) (map[string]string, error) {
	files := make(map[string]string)

	projectDir := filepath.Join(basePath, "projects", projectName)
	hostProjectDir := filepath.Join(basePath, "hosts", hostName, projectName)

	// 1. compose.yml from project
	if composePath := filepath.Join(projectDir, "compose.yml"); fileExists(composePath) {
		files["compose.yml"] = composePath
	}

	// 2. files/ from project
	err := walkFiles(filepath.Join(projectDir, "files"), files)
	if err != nil {
		return nil, err
	}

	// 3. compose.override.yml from host project
	if overridePath := filepath.Join(hostProjectDir, "compose.override.yml"); fileExists(overridePath) {
		files["compose.override.yml"] = overridePath
	}

	// 4. .env from host project
	if envPath := filepath.Join(hostProjectDir, ".env"); fileExists(envPath) {
		files[".env"] = envPath
	}

	// 5. files/ from host project (overwrites project-level files)
	err = walkFiles(filepath.Join(hostProjectDir, "files"), files)
	if err != nil {
		return nil, err
	}

	return files, nil
}

// ---------------------------------------------------------------------------
// Manifest helpers
// ---------------------------------------------------------------------------

func readManifest(client remote.RemoteClient, remoteDir string) *Manifest {
	data, err := client.ReadFile(path.Join(remoteDir, manifestFile))
	if err != nil {
		return nil
	}

	var manifest Manifest
	if json.Unmarshal(data, &manifest) != nil {
		return nil
	}

	return &manifest
}

// BuildManifest creates a manifest from the set of local files.
func BuildManifest(localFiles map[string]string) Manifest {
	var manifest Manifest
	for rel := range localFiles {
		manifest.ManagedFiles = append(manifest.ManagedFiles, rel)
	}

	sort.Strings(manifest.ManagedFiles)

	return manifest
}

// ---------------------------------------------------------------------------
// Diff / detection helpers
// ---------------------------------------------------------------------------

func computeDiff(name string, remote, local []byte) string {
	diff := new(difflib.UnifiedDiff)
	diff.A = difflib.SplitLines(string(remote))
	diff.B = difflib.SplitLines(string(local))
	diff.FromFile = name + " (remote)"
	diff.ToFile = name + " (local)"
	diff.FromDate = ""
	diff.ToDate = ""
	diff.Context = diffContextLines
	diff.Eol = ""
	text, _ := difflib.GetUnifiedDiffString(*diff)

	return text
}

func isBinary(data []byte) bool {
	// Check first 8 KB for null bytes.
	check := data
	if len(check) > binaryProbeBytes {
		check = check[:binaryProbeBytes]
	}

	return bytes.ContainsRune(check, 0)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)

	return err == nil && !info.IsDir()
}

func walkFiles(dir string, out map[string]string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("stat %s: %w", dir, err)
	}

	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(dir, func(pathValue string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, _ := filepath.Rel(dir, pathValue)
		out[rel] = pathValue

		return nil
	})
}

const (
	kiloBytes        = 1024
	megaBytes        = 1024 * kiloBytes
	diffContextLines = 3
	binaryProbeBytes = 8192
)

func humanSize(byteCount int) string {
	switch {
	case byteCount >= megaBytes:
		return fmt.Sprintf("%.1f MB", float64(byteCount)/float64(megaBytes))
	case byteCount >= kiloBytes:
		return fmt.Sprintf("%.1f KB", float64(byteCount)/float64(kiloBytes))
	default:
		return fmt.Sprintf("%d B", byteCount)
	}
}
