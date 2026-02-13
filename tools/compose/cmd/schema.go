package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"cmt/internal/config"

	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"
)

func newSchemaCmd() *cobra.Command {
	schemaCommand := new(cobra.Command)
	schemaCommand.Use = "schema [cmt|host]"
	schemaCommand.Short = "Generate JSON Schema for cmt config or host.yml"
	schemaCommand.Args = cobra.ExactArgs(1)
	schemaCommand.ValidArgs = []string{"cmt", "host"}
	schemaCommand.RunE = func(_ *cobra.Command, args []string) error {
		data, err := generateSchemaJSON(args[0])
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(append(data, '\n'))

		return err
	}

	return schemaCommand
}

// generateSchemaJSON returns the JSON Schema bytes for the given schema type.
func generateSchemaJSON(kind string) ([]byte, error) {
	var target any

	switch kind {
	case "cmt":
		targetConfig := new(config.CmtConfig)
		targetConfig.BasePath = ""
		targetConfig.Defaults = nil
		targetConfig.Hosts = nil
		target = targetConfig
	case "host":
		targetHostConfig := new(config.HostConfig)
		targetHostConfig.SSHConfig = ""
		targetHostConfig.RemotePath = ""
		targetHostConfig.PostSyncCommand = ""
		targetHostConfig.Projects = nil
		target = targetHostConfig
	default:
		return nil, fmt.Errorf("unknown schema type %q (use \"cmt\" or \"host\")", kind)
	}

	r := new(jsonschema.Reflector)
	schema := r.Reflect(target)

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling schema: %w", err)
	}

	return data, nil
}
