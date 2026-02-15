package cmd

import (
	"cmt/internal/config"
	"os"

	"github.com/spf13/cobra"
)

func newSchemaCmd() *cobra.Command {
	schemaCommand := new(cobra.Command)
	schemaCommand.Use = "schema [cmt|host]"
	schemaCommand.Short = "Generate JSON Schema for cmt config or host.yml"
	schemaCommand.Args = cobra.ExactArgs(1)
	schemaCommand.ValidArgs = []string{"cmt", "host"}
	schemaCommand.RunE = func(_ *cobra.Command, args []string) error {
		data, err := config.GenerateSchemaJSON(args[0])
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(append(data, '\n'))

		return err
	}

	return schemaCommand
}
