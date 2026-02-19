package config

const (
	ComposeActionUp   = "up"
	ComposeActionDown = "down"
)

type CmtConfig struct {
	BasePath         string            `json:"basePath"                   yaml:"basePath"`
	Defaults         *SyncDefaults     `json:"defaults,omitempty"         yaml:"defaults,omitempty"`
	Hosts            []HostEntry       `json:"hosts"                      yaml:"hosts"`
	BeforeApplyHooks *BeforeApplyHooks `json:"beforeApplyHooks,omitempty" yaml:"beforeApplyHooks,omitempty"`
}

type BeforeApplyHooks struct {
	BeforePrompt *HookCommand `json:"beforePrompt,omitempty" yaml:"beforePrompt,omitempty"`
	AfterPrompt  *HookCommand `json:"afterPrompt,omitempty"  yaml:"afterPrompt,omitempty"`
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
	Dirs            []string `json:"dirs,omitempty"            yaml:"dirs,omitempty"`
}

type ResolvedProjectConfig struct {
	RemotePath      string
	PostSyncCommand string
	ComposeAction   string
	Dirs            []string
}

type HookConfigPaths struct {
	ConfigPath string `json:"configPath"`
	BasePath   string `json:"basePath"`
}

type BeforePromptHookPayload struct {
	Hosts      []string        `json:"hosts"`
	WorkingDir string          `json:"workingDir"`
	Paths      HookConfigPaths `json:"paths"`
}

type AfterPromptHookPayload struct {
	Hosts      []string        `json:"hosts"`
	WorkingDir string          `json:"workingDir"`
	Paths      HookConfigPaths `json:"paths"`
}

func ResolveProjectConfig(cmtDefaults *SyncDefaults, hostCfg *HostConfig, projectName string) ResolvedProjectConfig {
	var resolved ResolvedProjectConfig

	if cmtDefaults != nil {
		resolved.RemotePath = cmtDefaults.RemotePath
		resolved.PostSyncCommand = cmtDefaults.PostSyncCommand
		resolved.ComposeAction = cmtDefaults.ComposeAction
	}

	if hostCfg == nil {
		resolved.ComposeAction = normalizeComposeAction(resolved.ComposeAction)

		return resolved
	}

	if hostCfg.RemotePath != "" {
		resolved.RemotePath = hostCfg.RemotePath
	}

	if hostCfg.PostSyncCommand != "" {
		resolved.PostSyncCommand = hostCfg.PostSyncCommand
	}

	if hostCfg.ComposeAction != "" {
		resolved.ComposeAction = hostCfg.ComposeAction
	}

	if projectConfig, ok := hostCfg.Projects[projectName]; ok && projectConfig != nil {
		if projectConfig.RemotePath != "" {
			resolved.RemotePath = projectConfig.RemotePath
		}

		if projectConfig.PostSyncCommand != "" {
			resolved.PostSyncCommand = projectConfig.PostSyncCommand
		}

		if projectConfig.ComposeAction != "" {
			resolved.ComposeAction = projectConfig.ComposeAction
		}

		if len(projectConfig.Dirs) > 0 {
			resolved.Dirs = projectConfig.Dirs
		}
	}

	resolved.ComposeAction = normalizeComposeAction(resolved.ComposeAction)

	return resolved
}

func normalizeComposeAction(action string) string {
	if action == "" {
		return ComposeActionUp
	}

	return action
}
