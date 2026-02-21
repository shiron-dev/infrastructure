package config

const (
	ComposeActionUp     = "up"
	ComposeActionDown   = "down"
	ComposeActionIgnore = "ignore"
)

type CmtConfig struct {
	BasePath         string            `json:"basePath"                   yaml:"basePath"`
	Defaults         *SyncDefaults     `json:"defaults,omitempty"         yaml:"defaults,omitempty"`
	Hosts            []HostEntry       `json:"hosts"                      yaml:"hosts"`
	BeforeApplyHooks *BeforeApplyHooks `json:"beforeApplyHooks,omitempty" yaml:"beforeApplyHooks,omitempty"`
}

type BeforeApplyHooks struct {
	BeforePlan        *HookCommand `json:"beforePlan,omitempty"        yaml:"beforePlan,omitempty"`
	BeforeApplyPrompt *HookCommand `json:"beforeApplyPrompt,omitempty" yaml:"beforeApplyPrompt,omitempty"`
	BeforeApply       *HookCommand `json:"beforeApply,omitempty"       yaml:"beforeApply,omitempty"`
}

type HookCommand struct {
	Command string `json:"command" yaml:"command"`
}

type SyncDefaults struct {
	RemotePath      string `json:"remotePath,omitempty"      yaml:"remotePath,omitempty"`
	PostSyncCommand string `json:"postSyncCommand,omitempty" yaml:"postSyncCommand,omitempty"`
	ComposeAction   string `json:"composeAction,omitempty"   yaml:"composeAction,omitempty"`
}

type HostEntry struct {
	Name       string `json:"name"                 yaml:"name"`
	Host       string `json:"host"                 yaml:"host"`
	Port       int    `json:"port,omitempty"       yaml:"port,omitempty"`
	User       string `json:"user"                 yaml:"user"`
	SSHKeyPath string `json:"sshKeyPath,omitempty" yaml:"sshKeyPath,omitempty"`
	SSHAgent   bool   `json:"sshAgent,omitempty"   yaml:"sshAgent,omitempty"`

	ProxyCommand  string   `json:"-" yaml:"-"`
	IdentityFiles []string `json:"-" yaml:"-"`
	IdentityAgent string   `json:"-" yaml:"-"`
}

type HostConfig struct {
	SSHConfig       string                    `json:"sshConfig,omitempty"       yaml:"sshConfig,omitempty"`
	RemotePath      string                    `json:"remotePath,omitempty"      yaml:"remotePath,omitempty"`
	PostSyncCommand string                    `json:"postSyncCommand,omitempty" yaml:"postSyncCommand,omitempty"`
	ComposeAction   string                    `json:"composeAction,omitempty"   yaml:"composeAction,omitempty"`
	Projects        map[string]*ProjectConfig `json:"projects,omitempty"        yaml:"projects,omitempty"`
}

type ProjectConfig struct {
	RemotePath      string   `json:"remotePath,omitempty"      yaml:"remotePath,omitempty"`
	PostSyncCommand string   `json:"postSyncCommand,omitempty" yaml:"postSyncCommand,omitempty"`
	ComposeAction   string   `json:"composeAction,omitempty"   yaml:"composeAction,omitempty"`
	RemoveOrphans   bool     `json:"removeOrphans,omitempty"   yaml:"removeOrphans,omitempty"`
	Dirs            []string `json:"dirs,omitempty"            yaml:"dirs,omitempty"`
}

type ResolvedProjectConfig struct {
	RemotePath      string
	PostSyncCommand string
	ComposeAction   string
	RemoveOrphans   bool
	Dirs            []string
}

type HookConfigPaths struct {
	ConfigPath string `json:"configPath"`
	BasePath   string `json:"basePath"`
}

type BeforePlanHookPayload struct {
	Hosts      []string        `json:"hosts"`
	WorkingDir string          `json:"workingDir"`
	Paths      HookConfigPaths `json:"paths"`
}

type BeforeApplyPromptHookPayload struct {
	Hosts      []string        `json:"hosts"`
	WorkingDir string          `json:"workingDir"`
	Paths      HookConfigPaths `json:"paths"`
}

type BeforeApplyHookPayload struct {
	Hosts      []string        `json:"hosts"`
	WorkingDir string          `json:"workingDir"`
	Paths      HookConfigPaths `json:"paths"`
}

func ResolveProjectConfig(cmtDefaults *SyncDefaults, hostCfg *HostConfig, projectName string) ResolvedProjectConfig {
	resolved := resolveFromDefaults(cmtDefaults)
	if hostCfg == nil {
		resolved.ComposeAction = normalizeComposeAction(resolved.ComposeAction)

		return resolved
	}

	applyHostOverrides(&resolved, hostCfg)
	applyProjectOverrides(&resolved, hostCfg, projectName)
	resolved.ComposeAction = normalizeComposeAction(resolved.ComposeAction)

	return resolved
}

func resolveFromDefaults(defaults *SyncDefaults) ResolvedProjectConfig {
	if defaults == nil {
		return ResolvedProjectConfig{
			RemotePath:      "",
			PostSyncCommand: "",
			ComposeAction:   "",
			RemoveOrphans:   false,
			Dirs:            nil,
		}
	}

	return ResolvedProjectConfig{
		RemotePath:      defaults.RemotePath,
		PostSyncCommand: defaults.PostSyncCommand,
		ComposeAction:   defaults.ComposeAction,
		RemoveOrphans:   false,
		Dirs:            nil,
	}
}

func applyHostOverrides(resolved *ResolvedProjectConfig, hostCfg *HostConfig) {
	if hostCfg.RemotePath != "" {
		resolved.RemotePath = hostCfg.RemotePath
	}

	if hostCfg.PostSyncCommand != "" {
		resolved.PostSyncCommand = hostCfg.PostSyncCommand
	}

	if hostCfg.ComposeAction != "" {
		resolved.ComposeAction = hostCfg.ComposeAction
	}
}

func applyProjectOverrides(resolved *ResolvedProjectConfig, hostCfg *HostConfig, projectName string) {
	projectConfig, ok := hostCfg.Projects[projectName]
	if !ok || projectConfig == nil {
		return
	}

	if projectConfig.RemotePath != "" {
		resolved.RemotePath = projectConfig.RemotePath
	}

	if projectConfig.PostSyncCommand != "" {
		resolved.PostSyncCommand = projectConfig.PostSyncCommand
	}

	if projectConfig.ComposeAction != "" {
		resolved.ComposeAction = projectConfig.ComposeAction
	}

	resolved.RemoveOrphans = projectConfig.RemoveOrphans

	if len(projectConfig.Dirs) > 0 {
		resolved.Dirs = projectConfig.Dirs
	}
}

func normalizeComposeAction(action string) string {
	if action == "" {
		return ComposeActionUp
	}

	return action
}
