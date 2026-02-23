package config

import (
	"fmt"
	"strconv"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
	"gopkg.in/yaml.v3"
)

const (
	ComposeActionUp      = "up"
	ComposeActionDown    = "down"
	ComposeActionIgnore  = "ignore"
	jsonSchemaTypeString = "string"
)

type DirConfig struct {
	Path       string `json:"path"                 yaml:"path"`
	Permission string `json:"permission,omitempty" yaml:"permission,omitempty"`
	Owner      string `json:"owner,omitempty"      yaml:"owner,omitempty"`
	Group      string `json:"group,omitempty"      yaml:"group,omitempty"`
}

type dirConfigAttrsOnly struct {
	Permission string `yaml:"permission,omitempty"`
	Owner      string `yaml:"owner,omitempty"`
	Group      string `yaml:"group,omitempty"`
}

func (d *DirConfig) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		d.Path = value.Value

		return nil
	case yaml.MappingNode:
		parsed, found, err := parseDirConfigPathKeyForm(value)
		if err != nil {
			return err
		}

		if found {
			*d = parsed

			return nil
		}
	case yaml.DocumentNode, yaml.SequenceNode, yaml.AliasNode:
		return decodeDirConfigPlainNode(d, value)
	default:
		return decodeDirConfigPlainNode(d, value)
	}

	return decodeDirConfigPlainNode(d, value)
}

func decodeDirConfigPlainNode(dst *DirConfig, value *yaml.Node) error {
	type plain DirConfig

	return value.Decode((*plain)(dst))
}

func parseDirConfigPathKeyForm(value *yaml.Node) (DirConfig, bool, error) {
	var (
		cfg       DirConfig
		pathFound bool
		pathValue string
		attrs     dirConfigAttrsOnly
	)

	for i := 0; i+1 < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		key := keyNode.Value

		switch key {
		case "permission":
			attrs.Permission = valNode.Value
		case "owner":
			attrs.Owner = valNode.Value
		case "group":
			attrs.Group = valNode.Value
		default:
			if pathFound {
				return cfg, false, fmt.Errorf("invalid dirs item: multiple path keys found (%q and %q)", pathValue, key)
			}

			pathFound = true
			pathValue = key

			err := mergeDirConfigAttrsFromValue(pathValue, valNode, &attrs)
			if err != nil {
				return cfg, false, err
			}
		}
	}

	if !pathFound {
		return cfg, false, nil
	}

	cfg.Path = pathValue
	cfg.Permission = attrs.Permission
	cfg.Owner = attrs.Owner
	cfg.Group = attrs.Group

	return cfg, true, nil
}

func mergeDirConfigAttrsFromValue(path string, valNode *yaml.Node, attrs *dirConfigAttrsOnly) error {
	switch valNode.Kind {
	case yaml.MappingNode:
		var nested dirConfigAttrsOnly
		err := valNode.Decode(&nested)
		if err != nil {
			return err
		}

		mergeNonEmptyDirConfigAttrs(attrs, nested)
	case yaml.ScalarNode:
		// Allow null/empty attributes:
		//   - <path>:
		if valNode.Tag != "!!null" && valNode.Value != "" {
			return fmt.Errorf("invalid dirs item for path %q: expected mapping or null attributes", path)
		}
	case yaml.DocumentNode, yaml.SequenceNode, yaml.AliasNode:
		return fmt.Errorf("invalid dirs item for path %q: expected mapping attributes", path)
	default:
		return fmt.Errorf("invalid dirs item for path %q: unsupported YAML node kind %d", path, valNode.Kind)
	}

	return nil
}

func mergeNonEmptyDirConfigAttrs(dst *dirConfigAttrsOnly, src dirConfigAttrsOnly) {
	if src.Permission != "" {
		dst.Permission = src.Permission
	}

	if src.Owner != "" {
		dst.Owner = src.Owner
	}

	if src.Group != "" {
		dst.Group = src.Group
	}
}

func (*DirConfig) JSONSchema() *jsonschema.Schema {
	stringSchema := new(jsonschema.Schema)
	stringSchema.Type = jsonSchemaTypeString

	attrsProps := orderedmap.New[string, *jsonschema.Schema]()
	permissionSchema := new(jsonschema.Schema)
	permissionSchema.Type = jsonSchemaTypeString
	attrsProps.Set("permission", permissionSchema)

	ownerSchema := new(jsonschema.Schema)
	ownerSchema.Type = jsonSchemaTypeString
	attrsProps.Set("owner", ownerSchema)

	groupSchema := new(jsonschema.Schema)
	groupSchema.Type = jsonSchemaTypeString
	attrsProps.Set("group", groupSchema)

	attrsObjectSchema := new(jsonschema.Schema)
	attrsObjectSchema.Type = "object"
	attrsObjectSchema.Properties = attrsProps
	attrsObjectSchema.AdditionalProperties = jsonschema.FalseSchema

	nullSchema := new(jsonschema.Schema)
	nullSchema.Type = "null"

	pathValueSchema := new(jsonschema.Schema)
	pathValueSchema.OneOf = []*jsonschema.Schema{attrsObjectSchema, nullSchema}

	pathKeyedObjectSchema := new(jsonschema.Schema)
	pathKeyedObjectSchema.Type = "object"
	pathKeyedObjectSchema.AdditionalProperties = pathValueSchema
	pathPropertyCount := uint64(1)
	pathKeyedObjectSchema.MinProperties = &pathPropertyCount
	pathKeyedObjectSchema.MaxProperties = &pathPropertyCount

	rootSchema := new(jsonschema.Schema)
	rootSchema.OneOf = []*jsonschema.Schema{stringSchema, pathKeyedObjectSchema}

	return rootSchema
}

func ValidateDirConfigs(dirs []DirConfig) error {
	for i, dirConfig := range dirs {
		if dirConfig.Path == "" {
			return fmt.Errorf("dirs[%d]: path is required", i)
		}

		if dirConfig.Permission == "" {
			continue
		}

		_, err := strconv.ParseUint(dirConfig.Permission, 8, 32)
		if err != nil {
			return fmt.Errorf("dirs[%d]: invalid permission %q (expected octal like \"0755\"): %w", i, dirConfig.Permission, err)
		}
	}

	return nil
}

func DefaultTemplateVarSources() []string {
	return []string{"*.yml", "*.yaml"}
}

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
	RemotePath         string   `json:"remotePath,omitempty"         yaml:"remotePath,omitempty"`
	PostSyncCommand    string   `json:"postSyncCommand,omitempty"    yaml:"postSyncCommand,omitempty"`
	ComposeAction      string   `json:"composeAction,omitempty"      yaml:"composeAction,omitempty"`
	TemplateVarSources []string `json:"templateVarSources,omitempty" yaml:"templateVarSources,omitempty"`
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
	SSHConfig          string                    `json:"sshConfig,omitempty"          yaml:"sshConfig,omitempty"`
	RemotePath         string                    `json:"remotePath,omitempty"         yaml:"remotePath,omitempty"`
	PostSyncCommand    string                    `json:"postSyncCommand,omitempty"    yaml:"postSyncCommand,omitempty"`
	ComposeAction      string                    `json:"composeAction,omitempty"      yaml:"composeAction,omitempty"`
	TemplateVarSources []string                  `json:"templateVarSources,omitempty" yaml:"templateVarSources,omitempty"`
	Projects           map[string]*ProjectConfig `json:"projects,omitempty"           yaml:"projects,omitempty"`
}

type ProjectConfig struct {
	RemotePath         string      `json:"remotePath,omitempty"         yaml:"remotePath,omitempty"`
	PostSyncCommand    string      `json:"postSyncCommand,omitempty"    yaml:"postSyncCommand,omitempty"`
	ComposeAction      string      `json:"composeAction,omitempty"      yaml:"composeAction,omitempty"`
	RemoveOrphans      bool        `json:"removeOrphans,omitempty"      yaml:"removeOrphans,omitempty"`
	Dirs               []DirConfig `json:"dirs,omitempty"               yaml:"dirs,omitempty"`
	TemplateVarSources []string    `json:"templateVarSources,omitempty" yaml:"templateVarSources,omitempty"`
}

type ResolvedProjectConfig struct {
	RemotePath         string
	PostSyncCommand    string
	ComposeAction      string
	RemoveOrphans      bool
	Dirs               []DirConfig
	TemplateVarSources []string
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
		resolved.TemplateVarSources = normalizeTemplateVarSources(resolved.TemplateVarSources)

		return resolved
	}

	applyHostOverrides(&resolved, hostCfg)
	applyProjectOverrides(&resolved, hostCfg, projectName)
	resolved.ComposeAction = normalizeComposeAction(resolved.ComposeAction)
	resolved.TemplateVarSources = normalizeTemplateVarSources(resolved.TemplateVarSources)

	return resolved
}

func normalizeTemplateVarSources(sources []string) []string {
	if len(sources) == 0 {
		return DefaultTemplateVarSources()
	}

	return sources
}

func resolveFromDefaults(defaults *SyncDefaults) ResolvedProjectConfig {
	if defaults == nil {
		return ResolvedProjectConfig{
			RemotePath:         "",
			PostSyncCommand:    "",
			ComposeAction:      "",
			RemoveOrphans:      false,
			Dirs:               nil,
			TemplateVarSources: nil,
		}
	}

	return ResolvedProjectConfig{
		RemotePath:         defaults.RemotePath,
		PostSyncCommand:    defaults.PostSyncCommand,
		ComposeAction:      defaults.ComposeAction,
		RemoveOrphans:      false,
		Dirs:               nil,
		TemplateVarSources: defaults.TemplateVarSources,
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

	if len(hostCfg.TemplateVarSources) > 0 {
		resolved.TemplateVarSources = hostCfg.TemplateVarSources
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

	if len(projectConfig.TemplateVarSources) > 0 {
		resolved.TemplateVarSources = projectConfig.TemplateVarSources
	}
}

func normalizeComposeAction(action string) string {
	if action == "" {
		return ComposeActionUp
	}

	return action
}
