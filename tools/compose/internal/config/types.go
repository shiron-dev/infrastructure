package config

// ---------------------------------------------------------------------------
// cmt config  (--config flag)
// ---------------------------------------------------------------------------

// CmtConfig is the top-level configuration for the Compose Manage Tool.
type CmtConfig struct {
	BasePath string        `json:"basePath"           yaml:"basePath"`
	Defaults *SyncDefaults `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Hosts    []HostEntry   `json:"hosts"              yaml:"hosts"`
}

// SyncDefaults holds the lowest-priority defaults for sync settings.
type SyncDefaults struct {
	RemotePath      string `json:"remotePath,omitempty"      yaml:"remotePath,omitempty"`
	PostSyncCommand string `json:"postSyncCommand,omitempty" yaml:"postSyncCommand,omitempty"`
}

// HostEntry defines SSH connection parameters for a target host.
type HostEntry struct {
	Name       string `json:"name"                 yaml:"name"`
	Host       string `json:"host"                 yaml:"host"`
	Port       int    `json:"port,omitempty"       yaml:"port,omitempty"`
	User       string `json:"user"                 yaml:"user"`
	SSHKeyPath string `json:"sshKeyPath,omitempty" yaml:"sshKeyPath,omitempty"`
	SSHAgent   bool   `json:"sshAgent,omitempty"   yaml:"sshAgent,omitempty"`

	// Fields populated by SSH config resolution via ssh -G (not from YAML).
	ProxyCommand  string   `json:"-" yaml:"-"`
	IdentityFiles []string `json:"-" yaml:"-"`
	IdentityAgent string   `json:"-" yaml:"-"`
}

// ---------------------------------------------------------------------------
// host.yml  (per-host file at hosts/<hostname>/host.yml)
// ---------------------------------------------------------------------------

// HostConfig is stored per-host in host.yml under hosts/<hostname>/.
type HostConfig struct {
	SSHConfig       string                    `json:"sshConfig,omitempty"       yaml:"sshConfig,omitempty"`
	RemotePath      string                    `json:"remotePath,omitempty"      yaml:"remotePath,omitempty"`
	PostSyncCommand string                    `json:"postSyncCommand,omitempty" yaml:"postSyncCommand,omitempty"`
	Projects        map[string]*ProjectConfig `json:"projects,omitempty"        yaml:"projects,omitempty"`
}

// ProjectConfig provides project-specific overrides within a host config.
type ProjectConfig struct {
	RemotePath      string   `json:"remotePath,omitempty"      yaml:"remotePath,omitempty"`
	PostSyncCommand string   `json:"postSyncCommand,omitempty" yaml:"postSyncCommand,omitempty"`
	Dirs            []string `json:"dirs,omitempty"            yaml:"dirs,omitempty"`
}

// ---------------------------------------------------------------------------
// Resolved (merged) configuration
// ---------------------------------------------------------------------------

// ResolvedProjectConfig holds the final merged settings for one host+project pair.
type ResolvedProjectConfig struct {
	RemotePath      string
	PostSyncCommand string
	Dirs            []string
}

// ResolveProjectConfig merges defaults from three layers:
//
//	cmt config defaults  →  host.yml host defaults  →  host.yml project overrides
func ResolveProjectConfig(cmtDefaults *SyncDefaults, hostCfg *HostConfig, projectName string) ResolvedProjectConfig {
	var resolved ResolvedProjectConfig

	// Layer 1: cmt config defaults
	if cmtDefaults != nil {
		resolved.RemotePath = cmtDefaults.RemotePath
		resolved.PostSyncCommand = cmtDefaults.PostSyncCommand
	}

	if hostCfg == nil {
		return resolved
	}

	// Layer 2: host.yml host-level defaults
	if hostCfg.RemotePath != "" {
		resolved.RemotePath = hostCfg.RemotePath
	}

	if hostCfg.PostSyncCommand != "" {
		resolved.PostSyncCommand = hostCfg.PostSyncCommand
	}

	// Layer 3: host.yml project-level overrides
	if projectConfig, ok := hostCfg.Projects[projectName]; ok && projectConfig != nil {
		if projectConfig.RemotePath != "" {
			resolved.RemotePath = projectConfig.RemotePath
		}

		if projectConfig.PostSyncCommand != "" {
			resolved.PostSyncCommand = projectConfig.PostSyncCommand
		}

		if len(projectConfig.Dirs) > 0 {
			resolved.Dirs = projectConfig.Dirs
		}
	}

	return resolved
}
