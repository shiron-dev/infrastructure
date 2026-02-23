package config

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
)

func SchemaKinds() []string {
	return []string{"cmt", "host", "hook-before-plan", "hook-before-apply-prompt", "hook-before-apply"}
}

func GenerateSchemaJSON(kind string) ([]byte, error) {
	var target any

	switch kind {
	case "cmt":
		targetConfig := new(CmtConfig)
		targetConfig.BasePath = ""
		targetConfig.Defaults = nil
		targetConfig.Hosts = nil
		targetConfig.BeforeApplyHooks = nil
		target = targetConfig
	case "host":
		targetHostConfig := new(HostConfig)
		targetHostConfig.SSHConfig = ""
		targetHostConfig.RemotePath = ""
		targetHostConfig.PostSyncCommand = ""
		targetHostConfig.Projects = nil
		target = targetHostConfig
	case "hook-before-plan":
		target = new(BeforePlanHookPayload)
	case "hook-before-apply-prompt":
		target = new(BeforeApplyPromptHookPayload)
	case "hook-before-apply":
		target = new(BeforeApplyHookPayload)
	default:
		return nil, fmt.Errorf("unknown schema type %q (valid: %v)", kind, SchemaKinds())
	}

	reflector := new(jsonschema.Reflector)
	reflector.Mapper = func(t reflect.Type) *jsonschema.Schema {
		if t == reflect.TypeFor[DirConfig]() {
			return new(DirConfig).JSONSchema()
		}

		return nil
	}

	schema := reflector.Reflect(target)

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling schema: %w", err)
	}

	return data, nil
}
