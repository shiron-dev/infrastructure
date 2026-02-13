package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"cmt/internal/config"

	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:       "schema [cmt|host]",
	Short:     "Generate JSON Schema for cmt config or host.yml",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"cmt", "host"},
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := generateSchemaJSON(args[0])
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(data, '\n'))

		return err
	},
}

// generateSchemaJSON returns the JSON Schema bytes for the given schema type.
func generateSchemaJSON(kind string) ([]byte, error) {
	var target any

	switch kind {
	case "cmt":
		target = &config.CmtConfig{}
	case "host":
		target = &config.HostConfig{}
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

func init() {
	rootCmd.AddCommand(schemaCmd)
}
