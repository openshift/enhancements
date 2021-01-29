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

			all := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return true
				},
				Cascade: true,
			}
			merged := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.State == "merged" && prd.Pull.ClosedAt.After(query.EarliestDate)
				},
			}
			closed := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.State == "closed" && prd.Pull.ClosedAt.After(query.EarliestDate)
				},
			}
			revived := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					// Anything in either of these states from
					// this period will fall into an earlier
					// bucket with the rule that includes the date
					// check.
					return prd.State == "closed" || prd.State == "merged"
				},
			}
			newPRs := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Pull.CreatedAt.After(query.EarliestDate)
				},
			}
			oldPRs := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Pull.UpdatedAt.Before(query.StaleDate) && prd.RecentActivityCount > 0
				},
			}
			stale := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Pull.UpdatedAt.Before(query.StaleDate)
				},
			}
			active := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.RecentActivityCount > 0
				},
			}
			idle := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return true
				},
			}

			reportBuckets := []*stats.Bucket{
				&all,
				&merged,
				&closed,
				&revived,
				&newPRs,
				&oldPRs,
				&stale,
				&active,
				&idle,
			}

			theStats := &stats.Stats{
				Query:   query,
				Buckets: reportBuckets,
			}

			err := theStats.Populate()
			if err != nil {
				return errors.Wrap(err, "could not generate stats")
			}

			// FIXME: temporary hack to make the report printing work
			theStats.All = all.Requests
			theStats.Merged = merged.Requests
			theStats.Revived = revived.Requests
			theStats.New = newPRs.Requests
			theStats.Old = oldPRs.Requests
			theStats.Stale = stale.Requests
			theStats.Active = active.Requests
			theStats.Idle = idle.Requests

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
