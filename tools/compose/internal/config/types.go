package config

// ---------------------------------------------------------------------------
// cmt config  (--config flag)
// ---------------------------------------------------------------------------

// CmtConfig is the top-level configuration for the Compose Manage Tool.
type CmtConfig struct {
	BasePath string        `json:"basePath"           jsonschema:"required,description=Path to the compose root directory"                  yaml:"basePath"`
	Defaults *SyncDefaults `json:"defaults,omitempty" jsonschema:"description=Default settings applied when host.yml does not specify them" yaml:"defaults,omitempty"`
	Hosts    []HostEntry   `json:"hosts"              jsonschema:"required,minItems=1,description=List of target hosts"                     yaml:"hosts"`
}

// SyncDefaults holds the lowest-priority defaults for sync settings.
type SyncDefaults struct {
	RemotePath      string `json:"remotePath,omitempty"      jsonschema:"description=Base remote directory for project files"              yaml:"remotePath,omitempty"`
	PostSyncCommand string `json:"postSyncCommand,omitempty" jsonschema:"description=Command executed in the project directory after sync" yaml:"postSyncCommand,omitempty"`
}

// HostEntry defines SSH connection parameters for a target host.
type HostEntry struct {
	Name       string `json:"name"                 jsonschema:"required,description=Host identifier (must match directory name under hosts/)" yaml:"name"`
	Host       string `json:"host"                 jsonschema:"required,description=SSH hostname or IP address"                               yaml:"host"`
	Port       int    `json:"port,omitempty"       jsonschema:"description=SSH port (default 22)"                                             yaml:"port,omitempty"`
	User       string `json:"user"                 jsonschema:"required,description=SSH username"                                             yaml:"user"`
	SSHKeyPath string `json:"sshKeyPath,omitempty" jsonschema:"description=Path to SSH private key (or public key for agent identification)"  yaml:"sshKeyPath,omitempty"`
	SSHAgent   bool   `json:"sshAgent,omitempty"   jsonschema:"description=Use SSH agent for authentication"                                  yaml:"sshAgent,omitempty"`

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
	SSHConfig       string                    `json:"sshConfig,omitempty"       jsonschema:"description=Path to SSH config file for ssh -G resolution. Relative paths are resolved against the host directory." yaml:"sshConfig,omitempty"`
	RemotePath      string                    `json:"remotePath,omitempty"      jsonschema:"description=Default remote base path for this host"                                                                 yaml:"remotePath,omitempty"`
	PostSyncCommand string                    `json:"postSyncCommand,omitempty" jsonschema:"description=Default post-sync command for this host"                                                                yaml:"postSyncCommand,omitempty"`
	Projects        map[string]*ProjectConfig `json:"projects,omitempty"        jsonschema:"description=Per-project overrides for this host"                                                                    yaml:"projects,omitempty"`
}

// ProjectConfig provides project-specific overrides within a host config.
type ProjectConfig struct {
	RemotePath      string   `json:"remotePath,omitempty"      jsonschema:"description=Remote base path override for this project"                                                yaml:"remotePath,omitempty"`
	PostSyncCommand string   `json:"postSyncCommand,omitempty" jsonschema:"description=Post-sync command override for this project"                                               yaml:"postSyncCommand,omitempty"`
	Dirs            []string `json:"dirs,omitempty"            jsonschema:"description=Directories to pre-create in the remote project directory (e.g. for Docker volume mounts)" yaml:"dirs,omitempty"`
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
