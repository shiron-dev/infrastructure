package syncer

import (
	"bytes"
	"encoding/json"
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
	case ActionAdd:
		return "add"
	case ActionModify:
		return "modify"
	case ActionDelete:
		return "delete"
	default:
		return "unchanged"
	}
}

func (a ActionType) Symbol() string {
	switch a {
	case ActionAdd:
		return "+"
	case ActionModify:
		return "~"
	case ActionDelete:
		return "-"
	default:
		return "="
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
func (p *SyncPlan) Stats() (hosts, projects, add, modify, delete, unchanged int) {
	hosts = len(p.HostPlans)

	for _, hp := range p.HostPlans {
		projects += len(hp.Projects)

		for _, pp := range hp.Projects {
			for _, fp := range pp.Files {
				switch fp.Action {
				case ActionAdd:
					add++
				case ActionModify:
					modify++
				case ActionDelete:
					delete++
				case ActionUnchanged:
					unchanged++
				}
			}
		}
	}

	return
}

// DirStats returns counts of directories to create and already existing.
func (p *SyncPlan) DirStats() (toCreate, existing int) {
	for _, hp := range p.HostPlans {
		for _, pp := range hp.Projects {
			for _, dp := range pp.Dirs {
				if dp.Exists {
					existing++
				} else {
					toCreate++
				}
			}
		}
	}

	return
}

// HasChanges returns true if the plan contains any add/modify/delete actions
// or directories to create.
func (p *SyncPlan) HasChanges() bool {
	_, _, add, mod, del, _ := p.Stats()
	dirCreate, _ := p.DirStats()

	return add+mod+del+dirCreate > 0
}

// ---------------------------------------------------------------------------
// Plan display
// ---------------------------------------------------------------------------

// Print writes a human-readable plan to w.
func (p *SyncPlan) Print(w io.Writer) {
	if len(p.HostPlans) == 0 {
		fmt.Fprintln(w, "No hosts selected.")

		return
	}

	for _, hp := range p.HostPlans {
		fmt.Fprintf(w, "\n=== Host: %s (%s@%s:%d) ===\n",
			hp.Host.Name, hp.Host.User, hp.Host.Host, hp.Host.Port)

		if len(hp.Projects) == 0 {
			fmt.Fprintln(w, "  (no projects)")

			continue
		}

		for _, pp := range hp.Projects {
			fmt.Fprintf(w, "\n  Project: %s\n", pp.ProjectName)
			fmt.Fprintf(w, "    Remote: %s\n", pp.RemoteDir)

			if pp.PostSyncCommand != "" {
				fmt.Fprintf(w, "    Post-sync: %s\n", pp.PostSyncCommand)
			}

			fmt.Fprintln(w)

			// Show directory plans.
			if len(pp.Dirs) > 0 {
				fmt.Fprintln(w, "    Dirs:")

				for _, dp := range pp.Dirs {
					if dp.Exists {
						fmt.Fprintf(w, "      = %s/ (exists)\n", dp.RelativePath)
					} else {
						fmt.Fprintf(w, "      + %s/ (create)\n", dp.RelativePath)
					}
				}

				fmt.Fprintln(w)
			}

			if len(pp.Files) == 0 && len(pp.Dirs) == 0 {
				fmt.Fprintln(w, "    (no files or dirs)")

				continue
			}

			for _, fp := range pp.Files {
				label := ""

				switch fp.Action {
				case ActionAdd:
					label = "new, " + humanSize(len(fp.LocalData))
				case ActionModify:
					label = "modified"
				case ActionDelete:
					label = "delete"
				case ActionUnchanged:
					label = "unchanged"
				}

				fmt.Fprintf(w, "    %s %s (%s)\n", fp.Action.Symbol(), fp.RelativePath, label)

				if fp.Diff != "" {
					for line := range strings.SplitSeq(fp.Diff, "\n") {
						if line != "" {
							fmt.Fprintf(w, "        %s\n", line)
						}
					}
				}
			}
		}
	}

	hosts, projects, add, mod, del, unch := p.Stats()
	dirCreate, _ := p.DirStats()

	fmt.Fprintf(w, "\nSummary: %d host(s), %d project(s) — %d to add, %d to modify, %d to delete, %d unchanged",
		hosts, projects, add, mod, del, unch)

	if dirCreate > 0 {
		fmt.Fprintf(w, ", %d dir(s) to create", dirCreate)
	}

	fmt.Fprintln(w)
}

// ---------------------------------------------------------------------------
// Build plan
// ---------------------------------------------------------------------------

// BuildPlan connects to each selected host and computes the diff.
func BuildPlan(cfg *config.CmtConfig, hostFilter, projectFilter []string) (*SyncPlan, error) {
	return BuildPlanWithDeps(cfg, hostFilter, projectFilter, PlanDependencies{})
}

// BuildPlanWithDeps connects to each selected host using injected dependencies.
func BuildPlanWithDeps(cfg *config.CmtConfig, hostFilter, projectFilter []string, deps PlanDependencies) (*SyncPlan, error) {
	clientFactory := deps.ClientFactory
	if clientFactory == nil {
		clientFactory = remote.DefaultClientFactory{}
	}

	sshResolver := deps.SSHResolver
	if sshResolver == nil {
		sshResolver = config.DefaultSSHConfigResolver{}
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

	plan := &SyncPlan{}

	for _, host := range hosts {
		// Load host config.
		hostCfg, err := config.LoadHostConfig(cfg.BasePath, host.Name)
		if err != nil {
			return nil, fmt.Errorf("loading host config for %s: %w", host.Name, err)
		}

		// Resolve SSH config via ssh -G (always runs; uses -F when
		// host.yml specifies sshConfig, otherwise default ssh config).
		sshConfigPath := ""
		if hostCfg != nil {
			sshConfigPath = hostCfg.SSHConfig
		}

		hostDir := filepath.Join(cfg.BasePath, "hosts", host.Name)
		if err := sshResolver.Resolve(&host, sshConfigPath, hostDir); err != nil {
			return nil, fmt.Errorf("resolving SSH config for %s: %w", host.Name, err)
		}

		// Connect via SSH/SFTP.
		client, err := clientFactory.NewClient(host)
		if err != nil {
			return nil, fmt.Errorf("connecting to host %s: %w", host.Name, err)
		}

		hostPlan, err := buildHostPlan(cfg, host, hostCfg, projects, client)
		client.Close()

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
	hp := &HostPlan{Host: host}

	for _, project := range projects {
		resolved := config.ResolveProjectConfig(cfg.Defaults, hostCfg, project)
		if resolved.RemotePath == "" {
			return nil, fmt.Errorf("remotePath is not set for host %q, project %q", host.Name, project)
		}

		remoteDir := path.Join(resolved.RemotePath, project)

		// Build directory plans.
		var dirPlans []DirPlan

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

		hp.Projects = append(hp.Projects, ProjectPlan{
			ProjectName:     project,
			RemoteDir:       remoteDir,
			PostSyncCommand: resolved.PostSyncCommand,
			Dirs:            dirPlans,
			Files:           filePlans,
		})
	}

	return hp, nil
}

func buildFilePlans(
	localFiles map[string]string,
	remoteDir string,
	manifest *Manifest,
	client remote.RemoteClient,
	templateVars map[string]any,
) ([]FilePlan, error) {
	var plans []FilePlan

	localSet := make(map[string]bool, len(localFiles))

	for relPath, localPath := range localFiles {
		localSet[relPath] = true

		localData, err := os.ReadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", localPath, err)
		}

		// Render file as Go template.
		localData, err = RenderTemplate(localData, templateVars)
		if err != nil {
			return nil, fmt.Errorf("rendering template %s: %w", localPath, err)
		}

		remotePath := path.Join(remoteDir, relPath)
		remoteData, readErr := client.ReadFile(remotePath)

		fp := FilePlan{
			RelativePath: relPath,
			LocalPath:    localPath,
			RemotePath:   remotePath,
			LocalData:    localData,
		}

		if readErr != nil {
			// Remote file does not exist.
			fp.Action = ActionAdd
		} else if bytes.Equal(localData, remoteData) {
			fp.Action = ActionUnchanged
			fp.RemoteData = remoteData
		} else {
			fp.Action = ActionModify

			fp.RemoteData = remoteData

			if !isBinary(localData) && !isBinary(remoteData) {
				fp.Diff = computeDiff(relPath, remoteData, localData)
			}
		}

		plans = append(plans, fp)
	}

	// Files in manifest but not in local → delete.
	if manifest != nil {
		for _, mf := range manifest.ManagedFiles {
			if mf == manifestFile {
				continue
			}

			if localSet[mf] {
				continue
			}

			remotePath := path.Join(remoteDir, mf)
			remoteData, _ := client.ReadFile(remotePath)
			plans = append(plans, FilePlan{
				RelativePath: mf,
				RemotePath:   remotePath,
				Action:       ActionDelete,
				RemoteData:   remoteData,
			})
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
	if p := filepath.Join(projectDir, "compose.yml"); fileExists(p) {
		files["compose.yml"] = p
	}

	// 2. files/ from project
	err := walkFiles(filepath.Join(projectDir, "files"), files)
	if err != nil {
		return nil, err
	}

	// 3. compose.override.yml from host project
	if p := filepath.Join(hostProjectDir, "compose.override.yml"); fileExists(p) {
		files["compose.override.yml"] = p
	}

	// 4. .env from host project
	if p := filepath.Join(hostProjectDir, ".env"); fileExists(p) {
		files[".env"] = p
	}

	// 5. files/ from host project (overwrites project-level files)
	err := walkFiles(filepath.Join(hostProjectDir, "files"), files)
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

	var m Manifest
	if json.Unmarshal(data, &m) != nil {
		return nil
	}

	return &m
}

// BuildManifest creates a manifest from the set of local files.
func BuildManifest(localFiles map[string]string) Manifest {
	var m Manifest
	for rel := range localFiles {
		m.ManagedFiles = append(m.ManagedFiles, rel)
	}

	sort.Strings(m.ManagedFiles)

	return m
}

// ---------------------------------------------------------------------------
// Diff / detection helpers
// ---------------------------------------------------------------------------

func computeDiff(name string, remote, local []byte) string {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(remote)),
		B:        difflib.SplitLines(string(local)),
		FromFile: name + " (remote)",
		ToFile:   name + " (local)",
		Context:  3,
	}
	text, _ := difflib.GetUnifiedDiffString(diff)

	return text
}

func isBinary(data []byte) bool {
	// Check first 8 KB for null bytes.
	check := data
	if len(check) > 8192 {
		check = check[:8192]
	}

	return bytes.ContainsRune(check, 0)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)

	return err == nil && !info.IsDir()
}

func walkFiles(dir string, out map[string]string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil // directory does not exist; not an error
	}

	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, _ := filepath.Rel(dir, p)
		out[rel] = p

		return nil
	})
}

func humanSize(n int) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)

	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
