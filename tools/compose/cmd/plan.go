package cmd

import (
	"os"

	"cmt/internal/config"
	"cmt/internal/syncer"

	"github.com/spf13/cobra"
)

func newPlanCmd(configPath *string) *cobra.Command {
	var hostFilter []string

	var projectFilter []string

	dependencies := syncer.PlanDependencies{
		ProgressWriter: os.Stdout,
	}

	planCommand := new(cobra.Command)
	planCommand.Use = "plan"
	planCommand.Short = "Show what would be synced without making changes"
	planCommand.RunE = func(_ *cobra.Command, _ []string) error {
		cfg, err := config.LoadCmtConfig(*configPath)
		if err != nil {
			return err
		}

		plan, err := syncer.BuildPlanWithDeps(cfg, hostFilter, projectFilter, dependencies)
		if err != nil {
			return err
		}

		plan.Print(os.Stdout)

		return nil
	}

	planCommand.Flags().StringSliceVar(&hostFilter, "host", nil, "filter by host name (repeatable)")
	planCommand.Flags().StringSliceVar(&projectFilter, "project", nil, "filter by project name (repeatable)")

	return planCommand
}
