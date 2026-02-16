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
	"strconv"
	"strings"

	"cmt/internal/config"
	"cmt/internal/remote"

	"github.com/pmezard/go-difflib/difflib"
)

type PlanDependencies struct {
	ClientFactory remote.ClientFactory
	SSHResolver   config.SSHConfigResolver
}

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

type SyncPlan struct {
	HostPlans []HostPlan
}

type HostPlan struct {
	Host     config.HostEntry
	Projects []ProjectPlan
}

type ProjectPlan struct {
	ProjectName     string
	RemoteDir       string
	PostSyncCommand string
	Dirs            []DirPlan
	Files           []FilePlan
}

type DirPlan struct {
	RelativePath string
	RemotePath   string
	Exists       bool
}

type FilePlan struct {
	RelativePath string
	LocalPath    string
	RemotePath   string
	Action       ActionType
	LocalData    []byte
	RemoteData   []byte
	Diff         string
}

type Manifest struct {
	ManagedFiles []string `json:"managedFiles"`
}

const manifestFile = ".cmt-manifest.json"

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

func (p *SyncPlan) HasChanges() bool {
	hostCount, projectCount, add, mod, del, unchangedCount := p.Stats()
	_ = hostCount
	_ = projectCount
	_ = unchangedCount
	dirCreate, _ := p.DirStats()

	return add+mod+del+dirCreate > 0
}

func (p *SyncPlan) Print(writer io.Writer) {
	style := newOutputStyle(writer)

	if len(p.HostPlans) == 0 {
		_, _ = fmt.Fprintln(writer, style.muted("No hosts selected."))

		return
	}

	for _, hostPlan := range p.HostPlans {
		printHostPlan(writer, style, hostPlan)
	}

	hosts, projects, add, mod, del, unch := p.Stats()
	dirCreate, _ := p.DirStats()

	_, _ = fmt.Fprintf(writer, "\n%s %d host(s), %d project(s) — %s to add, %s to modify, %s to delete, %s unchanged",
		style.key("Summary:"),
		hosts,
		projects,
		style.success(strconv.Itoa(add)),
		style.warning(strconv.Itoa(mod)),
		style.danger(strconv.Itoa(del)),
		style.muted(strconv.Itoa(unch)))

	if dirCreate > 0 {
		_, _ = fmt.Fprintf(writer, ", %s dir(s) to create", style.success(strconv.Itoa(dirCreate)))
	}

	_, _ = fmt.Fprintln(writer)
}

func printHostPlan(writer io.Writer, style outputStyle, hostPlan HostPlan) {
	hostLine := fmt.Sprintf(
		"=== Host: %s (%s@%s:%d) ===",
		hostPlan.Host.Name,
		hostPlan.Host.User,
		hostPlan.Host.Host,
		hostPlan.Host.Port,
	)
	_, _ = fmt.Fprintf(writer, "\n%s\n", style.hostHeader(hostLine))

	if len(hostPlan.Projects) == 0 {
		_, _ = fmt.Fprintln(writer, style.muted("  (no projects)"))

		return
	}

	for _, projectPlan := range hostPlan.Projects {
		printProjectPlan(writer, style, projectPlan)
	}
}

func printProjectPlan(writer io.Writer, style outputStyle, projectPlan ProjectPlan) {
	_, _ = fmt.Fprintf(writer, "\n  %s %s\n", style.key("Project:"), style.projectName(projectPlan.ProjectName))
	_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("Remote:"), projectPlan.RemoteDir)

	if projectPlan.PostSyncCommand != "" {
		_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("Post-sync:"), projectPlan.PostSyncCommand)
	}

	_, _ = fmt.Fprintln(writer)
	printProjectDirPlans(writer, style, projectPlan.Dirs)

	if len(projectPlan.Files) == 0 && len(projectPlan.Dirs) == 0 {
		_, _ = fmt.Fprintln(writer, style.muted("    (no files or dirs)"))

		return
	}

	for _, filePlan := range projectPlan.Files {
		_, _ = fmt.Fprintf(
			writer,
			"    %s %s (%s)\n",
			style.actionSymbol(filePlan.Action),
			filePlan.RelativePath,
			filePlanLabel(filePlan),
		)

		printFileDiff(writer, style, filePlan.Diff)
	}
}

func printProjectDirPlans(writer io.Writer, style outputStyle, dirPlans []DirPlan) {
	if len(dirPlans) == 0 {
		return
	}

	_, _ = fmt.Fprintln(writer, "    "+style.key("Dirs:"))

	for _, directoryPlan := range dirPlans {
		statusText := style.success("(create)")
		action := ActionAdd

		if directoryPlan.Exists {
			statusText = style.muted("(exists)")
			action = ActionUnchanged
		}

		_, _ = fmt.Fprintf(
			writer,
			"      %s %s/ %s\n",
			style.actionSymbol(action),
			directoryPlan.RelativePath,
			statusText,
		)
	}

	_, _ = fmt.Fprintln(writer)
}

func filePlanLabel(filePlan FilePlan) string {
	switch filePlan.Action {
	case ActionAdd:
		return "new, " + humanSize(len(filePlan.LocalData))
	case ActionModify:
		return "modified"
	case ActionDelete:
		return "delete"
	case ActionUnchanged:
		return "unchanged"
	default:
		return "unknown"
	}
}

func printFileDiff(writer io.Writer, style outputStyle, diff string) {
	if diff == "" {
		return
	}

	for line := range strings.SplitSeq(diff, "\n") {
		if line == "" {
			continue
		}

		_, _ = fmt.Fprintf(writer, "        %s\n", style.diffLine(line))
	}
}

func BuildPlan(cfg *config.CmtConfig, hostFilter, projectFilter []string) (*SyncPlan, error) {
	var dependencies PlanDependencies

	return BuildPlanWithDeps(cfg, hostFilter, projectFilter, dependencies)
}

func BuildPlanWithDeps(
	cfg *config.CmtConfig,
	hostFilter, projectFilter []string,
	deps PlanDependencies,
) (*SyncPlan, error) {
	clientFactory, sshResolver := resolvePlanDependencies(deps)

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
		hostPlan, err := buildHostPlanForTarget(cfg, host, projects, clientFactory, sshResolver)
		if err != nil {
			return nil, err
		}

		plan.HostPlans = append(plan.HostPlans, *hostPlan)
	}

	return plan, nil
}

func resolvePlanDependencies(deps PlanDependencies) (remote.ClientFactory, config.SSHConfigResolver) {
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

	return clientFactory, sshResolver
}

func buildHostPlanForTarget(
	cfg *config.CmtConfig,
	host config.HostEntry,
	projects []string,
	clientFactory remote.ClientFactory,
	sshResolver config.SSHConfigResolver,
) (*HostPlan, error) {
	hostCfg, found, err := loadHostConfig(cfg.BasePath, host.Name)
	if err != nil {
		return nil, fmt.Errorf("loading host config for %s: %w", host.Name, err)
	}

	if !found {
		hostCfg = nil
	}

	err = resolveHostSSHConfig(cfg.BasePath, &host, hostCfg, sshResolver)
	if err != nil {
		return nil, fmt.Errorf("resolving SSH config for %s: %w", host.Name, err)
	}

	client, err := clientFactory.NewClient(host)
	if err != nil {
		return nil, fmt.Errorf("connecting to host %s: %w", host.Name, err)
	}

	defer func() {
		_ = client.Close()
	}()

	hostPlan, err := buildHostPlan(cfg, host, hostCfg, projects, client)
	if err != nil {
		return nil, err
	}

	return hostPlan, nil
}

func loadHostConfig(basePath, hostName string) (*config.HostConfig, bool, error) {
	hostCfg, err := config.LoadHostConfig(basePath, hostName)
	if errors.Is(err, config.ErrHostConfigNotFound) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	return hostCfg, true, nil
}

func resolveHostSSHConfig(
	basePath string,
	host *config.HostEntry,
	hostCfg *config.HostConfig,
	sshResolver config.SSHConfigResolver,
) error {
	sshConfigPath := ""
	if hostCfg != nil {
		sshConfigPath = hostCfg.SSHConfig
	}

	hostDir := filepath.Join(basePath, "hosts", host.Name)

	return sshResolver.Resolve(host, sshConfigPath, hostDir)
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

		templateVars, err := LoadTemplateVars(cfg.BasePath, host.Name, project)
		if err != nil {
			return nil, fmt.Errorf("loading template vars for %s/%s: %w", host.Name, project, err)
		}

		localFiles, err := CollectLocalFiles(cfg.BasePath, host.Name, project)
		if err != nil {
			return nil, fmt.Errorf("collecting files for %s/%s: %w", host.Name, project, err)
		}

		manifest := readManifest(client, remoteDir)

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

		filePlan, err := buildLocalFilePlan(relPath, localPath, remoteDir, client, templateVars)
		if err != nil {
			return nil, err
		}

		plans = append(plans, filePlan)
	}

	plans = append(plans, buildDeleteFilePlans(manifest, localSet, remoteDir, client)...)

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].RelativePath < plans[j].RelativePath
	})

	return plans, nil
}

func buildLocalFilePlan(
	relPath string,
	localPath string,
	remoteDir string,
	client remote.RemoteClient,
	templateVars map[string]any,
) (FilePlan, error) {
	cleanLocalPath := filepath.Clean(localPath)

	localData, err := os.ReadFile(cleanLocalPath)
	if err != nil {
		return FilePlan{}, fmt.Errorf("reading %s: %w", cleanLocalPath, err)
	}

	localData, err = RenderTemplate(localData, templateVars)
	if err != nil {
		return FilePlan{}, fmt.Errorf("rendering template %s: %w", cleanLocalPath, err)
	}

	remotePath := path.Join(remoteDir, relPath)
	remoteData, readErr := client.ReadFile(remotePath)

	filePlan := FilePlan{
		RelativePath: relPath,
		LocalPath:    localPath,
		RemotePath:   remotePath,
		Action:       ActionUnchanged,
		LocalData:    localData,
		RemoteData:   nil,
		Diff:         "",
	}

	if readErr == nil {
		filePlan.RemoteData = remoteData
		if bytes.Equal(localData, remoteData) {
			return filePlan, nil
		}

		filePlan.Action = ActionModify
		if !isBinary(localData) && !isBinary(remoteData) {
			filePlan.Diff = computeDiff(relPath, remoteData, localData)
		}

		return filePlan, nil
	}

	filePlan.Action = ActionAdd

	return filePlan, nil
}

func buildDeleteFilePlans(
	manifest *Manifest,
	localSet map[string]bool,
	remoteDir string,
	client remote.RemoteClient,
) []FilePlan {
	if manifest == nil {
		return nil
	}

	deletePlans := make([]FilePlan, 0)

	for _, managedFile := range manifest.ManagedFiles {
		if managedFile == manifestFile || localSet[managedFile] {
			continue
		}

		remotePath := path.Join(remoteDir, managedFile)
		remoteData, _ := client.ReadFile(remotePath)
		deletePlans = append(deletePlans, FilePlan{
			RelativePath: managedFile,
			LocalPath:    "",
			RemotePath:   remotePath,
			Action:       ActionDelete,
			LocalData:    nil,
			RemoteData:   remoteData,
			Diff:         "",
		})
	}

	return deletePlans
}

func CollectLocalFiles(basePath, hostName, projectName string) (map[string]string, error) {
	files := make(map[string]string)

	projectDir := filepath.Join(basePath, "projects", projectName)
	hostProjectDir := filepath.Join(basePath, "hosts", hostName, projectName)

	if composePath := filepath.Join(projectDir, "compose.yml"); fileExists(composePath) {
		files["compose.yml"] = composePath
	}

	err := walkFiles(filepath.Join(projectDir, "files"), files)
	if err != nil {
		return nil, err
	}

	if overridePath := filepath.Join(hostProjectDir, "compose.override.yml"); fileExists(overridePath) {
		files["compose.override.yml"] = overridePath
	}

	if envPath := filepath.Join(hostProjectDir, ".env"); fileExists(envPath) {
		files[".env"] = envPath
	}

	err = walkFiles(filepath.Join(hostProjectDir, "files"), files)
	if err != nil {
		return nil, err
	}

	return files, nil
}

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

func BuildManifest(localFiles map[string]string) Manifest {
	var manifest Manifest
	for rel := range localFiles {
		manifest.ManagedFiles = append(manifest.ManagedFiles, rel)
	}

	sort.Strings(manifest.ManagedFiles)

	return manifest
}

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
