package config

// ---------------------------------------------------------------------------
// cmt config  (--config flag)
// ---------------------------------------------------------------------------

// CmtConfig is the top-level configuration for the Compose Manage Tool.
type CmtConfig struct {
	BasePath string        `yaml:"basePath" json:"basePath" jsonschema:"required,description=Path to the compose root directory"`
	Defaults *SyncDefaults `yaml:"defaults,omitempty" json:"defaults,omitempty" jsonschema:"description=Default settings applied when host.yml does not specify them"`
	Hosts    []HostEntry   `yaml:"hosts" json:"hosts" jsonschema:"required,minItems=1,description=List of target hosts"`
}

// SyncDefaults holds the lowest-priority defaults for sync settings.
type SyncDefaults struct {
	RemotePath      string `yaml:"remotePath,omitempty" json:"remotePath,omitempty" jsonschema:"description=Base remote directory for project files"`
	PostSyncCommand string `yaml:"postSyncCommand,omitempty" json:"postSyncCommand,omitempty" jsonschema:"description=Command executed in the project directory after sync"`
}

// HostEntry defines SSH connection parameters for a target host.
type HostEntry struct {
	Name       string `yaml:"name" json:"name" jsonschema:"required,description=Host identifier (must match directory name under hosts/)"`
	Host       string `yaml:"host" json:"host" jsonschema:"required,description=SSH hostname or IP address"`
	Port       int    `yaml:"port,omitempty" json:"port,omitempty" jsonschema:"description=SSH port (default 22)"`
	User       string `yaml:"user" json:"user" jsonschema:"required,description=SSH username"`
	SSHKeyPath string `yaml:"sshKeyPath,omitempty" json:"sshKeyPath,omitempty" jsonschema:"description=Path to SSH private key (or public key for agent identification)"`
	SSHAgent   bool   `yaml:"sshAgent,omitempty" json:"sshAgent,omitempty" jsonschema:"description=Use SSH agent for authentication"`

	// Fields populated by SSH config resolution via ssh -G (not from YAML).
	ProxyCommand  string   `yaml:"-" json:"-"`
	IdentityFiles []string `yaml:"-" json:"-"`
	IdentityAgent string   `yaml:"-" json:"-"`
}

// ---------------------------------------------------------------------------
// host.yml  (per-host file at hosts/<hostname>/host.yml)
// ---------------------------------------------------------------------------

// HostConfig is stored per-host in host.yml under hosts/<hostname>/.
type HostConfig struct {
	SSHConfig       string                    `yaml:"sshConfig,omitempty" json:"sshConfig,omitempty" jsonschema:"description=Path to SSH config file for ssh -G resolution. Relative paths are resolved against the host directory."`
	RemotePath      string                    `yaml:"remotePath,omitempty" json:"remotePath,omitempty" jsonschema:"description=Default remote base path for this host"`
	PostSyncCommand string                    `yaml:"postSyncCommand,omitempty" json:"postSyncCommand,omitempty" jsonschema:"description=Default post-sync command for this host"`
	Projects        map[string]*ProjectConfig `yaml:"projects,omitempty" json:"projects,omitempty" jsonschema:"description=Per-project overrides for this host"`
}

// ProjectConfig provides project-specific overrides within a host config.
type ProjectConfig struct {
	RemotePath      string   `yaml:"remotePath,omitempty" json:"remotePath,omitempty" jsonschema:"description=Remote base path override for this project"`
	PostSyncCommand string   `yaml:"postSyncCommand,omitempty" json:"postSyncCommand,omitempty" jsonschema:"description=Post-sync command override for this project"`
	Dirs            []string `yaml:"dirs,omitempty" json:"dirs,omitempty" jsonschema:"description=Directories to pre-create in the remote project directory (e.g. for Docker volume mounts)"`
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
	var r ResolvedProjectConfig

	// Layer 1: cmt config defaults
	if cmtDefaults != nil {
		r.RemotePath = cmtDefaults.RemotePath
		r.PostSyncCommand = cmtDefaults.PostSyncCommand
	}

	if hostCfg == nil {
		return r
	}

	// Layer 2: host.yml host-level defaults
	if hostCfg.RemotePath != "" {
		r.RemotePath = hostCfg.RemotePath
	}
	if hostCfg.PostSyncCommand != "" {
		r.PostSyncCommand = hostCfg.PostSyncCommand
	}

	// Layer 3: host.yml project-level overrides
	if pc, ok := hostCfg.Projects[projectName]; ok && pc != nil {
		if pc.RemotePath != "" {
			r.RemotePath = pc.RemotePath
		}
		if pc.PostSyncCommand != "" {
			r.PostSyncCommand = pc.PostSyncCommand
		}
		if len(pc.Dirs) > 0 {
			r.Dirs = pc.Dirs
		}
	}

	return r
}
