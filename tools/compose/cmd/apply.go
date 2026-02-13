package cmd

import (
	"os"

	"cmt/internal/config"
	"cmt/internal/syncer"

	"github.com/spf13/cobra"
)

var (
	applyHostFilter    []string
	applyProjectFilter []string
	autoApprove        bool
	applyDeps          syncer.ApplyDependencies
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Sync files to remote hosts (with confirmation unless --auto-approve)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCmtConfig(cfgFile)
		if err != nil {
			return err
		}

		plan, err := syncer.BuildPlanWithDeps(cfg, applyHostFilter, applyProjectFilter, syncer.PlanDependencies{
			ClientFactory: applyDeps.ClientFactory,
		})
		if err != nil {
			return err
		}

		return syncer.ApplyWithDeps(cfg, plan, autoApprove, os.Stdout, applyDeps)
	},
}

func init() {
	applyCmd.Flags().StringSliceVar(&applyHostFilter, "host", nil, "filter by host name (repeatable)")
	applyCmd.Flags().StringSliceVar(&applyProjectFilter, "project", nil, "filter by project name (repeatable)")
	applyCmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "skip confirmation prompt")
	rootCmd.AddCommand(applyCmd)
}
