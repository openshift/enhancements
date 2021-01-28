package cmd

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/report"
	"github.com/openshift/enhancements/tools/stats"
	"github.com/openshift/enhancements/tools/util"
)

func newReportCommand() *cobra.Command {
	var (
		daysBack, staleMonths int
		devMode, full         bool
	)

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate the weekly activity report",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := util.NewPullRequestQuery(
				daysBack, staleMonths, orgName, repoName, devMode,
				util.NewGithubClient(configSettings.Github.Token))

			theStats, err := stats.Generate(query)
			if err != nil {
				return errors.Wrap(err, "could not generate stats")
			}

			report.ShowReport(theStats, daysBack, staleMonths, full)

			return nil
		},
	}

	cmd.Flags().IntVar(&daysBack, "days-back", 7, "how many days back to query")
	cmd.Flags().IntVar(&staleMonths, "stale-months", 3,
		"how many months before a pull request is considered stale")
	cmd.Flags().BoolVar(&devMode, "dev", false, "dev mode, stop after first page of PRs")
	cmd.Flags().BoolVar(&full, "full", false, "full report, not just summary")

	return cmd
}

func init() {
	rootCmd.AddCommand(newReportCommand())
}
