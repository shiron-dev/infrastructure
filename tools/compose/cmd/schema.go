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
		var target any
		switch args[0] {
		case "cmt":
			target = &config.CmtConfig{}
		case "host":
			target = &config.HostConfig{}
		default:
			return fmt.Errorf("unknown schema type %q (use \"cmt\" or \"host\")", args[0])
		}

		r := new(jsonschema.Reflector)
		schema := r.Reflect(target)

		data, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling schema: %w", err)
		}
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}
