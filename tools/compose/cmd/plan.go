package cmd

import (
	"os"

	"cmt/internal/config"
	"cmt/internal/syncer"

	"github.com/spf13/cobra"
)

var (
	planHostFilter    []string
	planProjectFilter []string
	planDeps          syncer.PlanDependencies
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show what would be synced without making changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCmtConfig(cfgFile)
		if err != nil {
			return err
		}

		plan, err := syncer.BuildPlanWithDeps(cfg, planHostFilter, planProjectFilter, planDeps)
		if err != nil {
			return err
		}

		plan.Print(os.Stdout)

		return nil
	},
}

func init() {
	planCmd.Flags().StringSliceVar(&planHostFilter, "host", nil, "filter by host name (repeatable)")
	planCmd.Flags().StringSliceVar(&planProjectFilter, "project", nil, "filter by project name (repeatable)")
	rootCmd.AddCommand(planCmd)
}
