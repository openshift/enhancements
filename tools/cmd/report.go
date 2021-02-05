package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/enhancements"
	"github.com/openshift/enhancements/tools/report"
	"github.com/openshift/enhancements/tools/stats"
	"github.com/openshift/enhancements/tools/util"
)

func newReportCommand() *cobra.Command {
	var (
		daysBack, staleMonths int
		devMode               bool
	)

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate the weekly activity report",
		RunE: func(cmd *cobra.Command, args []string) error {

			query := util.NewPullRequestQuery(
				daysBack, staleMonths, orgName, repoName, devMode,
				util.NewGithubClient(configSettings.Github.Token))

			// Define a bucket to include all pull requests as a way
			// to give us a count of the total number we've seen while
			// building the report.
			all := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return true
				},
				Cascade: true,
			}

			// Define a bucket to capture anything we want to ignore
			// in the report, so the other rules aren't even applied.
			ignore := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					group, _, err := enhancements.GetGroup(*prd.Pull.Number)
					if err != nil {
						fmt.Fprintf(os.Stderr, "WARNING: failed to get group of PR %d: %s\n",
							*prd.Pull.Number, err)
						group = "uncategorized"
					}

					// ignore pull requests with only changes in the
					// hack, tools, or this-week directories
					return group == "hack" || group == "tools" || group == "this-week"
				},
			}

			// Define a few buckets for pull requests related to work
			// prioritized for the current or next release.
			prioritizedMerged := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Prioritized && prd.State == "merged" && prd.Pull.ClosedAt.After(query.EarliestDate)
				},
			}
			prioritizedClosed := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Prioritized && prd.State == "closed" && prd.Pull.ClosedAt.After(query.EarliestDate)
				},
			}
			prioritizedNew := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Prioritized && prd.Pull.CreatedAt.After(query.EarliestDate)
				},
			}
			prioritizedActive := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Prioritized && prd.RecentActivityCount > 0
				},
			}

			// Define basic groups for all of the non-prioritized pull
			// requests.
			otherMerged := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.State == "merged" && prd.Pull.ClosedAt.After(query.EarliestDate)
				},
			}
			otherClosed := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.State == "closed" && prd.Pull.ClosedAt.After(query.EarliestDate)
				},
			}
			otherNew := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Pull.CreatedAt.After(query.EarliestDate)
				},
			}
			otherOld := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Pull.UpdatedAt.Before(query.StaleDate) && prd.RecentActivityCount > 0
				},
			}

			// Define some extra buckets for older or lingering items
			revived := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					// Anything in either of these states from
					// this period will fall into an earlier
					// bucket with the rule that includes the date
					// check.
					return prd.State == "closed" || prd.State == "merged"
				},
			}
			stale := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Stale
				},
			}
			otherActive := stats.Bucket{
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
				&ignore,

				&prioritizedMerged,
				&prioritizedClosed,
				&prioritizedNew,
				&prioritizedActive,

				&otherMerged,
				&otherClosed,
				&revived,
				&otherNew,
				&otherOld,

				&stale,
				&otherActive,
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

			fmt.Fprintf(os.Stderr, "Processed %d pull requests\n", len(all.Requests))
			fmt.Fprintf(os.Stderr, "Ignored %d pull requests\n", len(ignore.Requests))

			year, month, day := time.Now().Date()
			fmt.Printf("# This Week in Enhancements - %d-%.2d-%.2d\n", year, month, day)

			// Only print the priority section if there are prioritized pull requests
			if anyRequests(prioritizedMerged, prioritizedClosed, prioritizedNew, prioritizedActive) {
				fmt.Printf("\n## Enhancements for Release Priorities\n")

				report.SortByID(prioritizedMerged.Requests)
				report.ShowPRs("Prioritized Merged", prioritizedMerged.Requests, true)

				report.SortByID(prioritizedNew.Requests)
				report.ShowPRs("Prioritized New", prioritizedNew.Requests, true)

				report.SortByActivityCountDesc(prioritizedActive.Requests)
				report.ShowPRs("Prioritized Active", prioritizedActive.Requests, true)

				report.SortByID(prioritizedClosed.Requests)
				report.ShowPRs("Prioritized Closed", prioritizedClosed.Requests, false)
			}

			fmt.Printf("\n## Other Enhancements\n")

			report.SortByID(otherMerged.Requests)
			report.ShowPRs("Other Merged", otherMerged.Requests, true)

			report.SortByID(otherNew.Requests)
			report.ShowPRs("Other New", otherNew.Requests, true)

			report.SortByActivityCountDesc(otherActive.Requests)
			report.ShowPRs("Other Active", otherActive.Requests, false)

			report.SortByID(otherClosed.Requests)
			report.ShowPRs("Other Closed", otherClosed.Requests, false)

			report.SortByID(revived.Requests)
			report.ShowPRs(
				fmt.Sprintf("Revived (closed more than %d days ago, but with new comments)", daysBack),
				revived.Requests,
				false,
			)

			report.SortByID(otherOld.Requests)
			report.ShowPRs(
				fmt.Sprintf("Old (older than %d months, but discussion in last %d days)",
					staleMonths, daysBack),
				otherOld.Requests,
				false,
			)

			report.SortByID(stale.Requests)
			report.ShowPRs(
				"Other lifecycle/stale",
				stale.Requests,
				false,
			)

			report.SortByID(idle.Requests)
			report.ShowPRs(
				fmt.Sprintf("Idle (no comments for at least %d days)", daysBack),
				idle.Requests,
				false,
			)

			return nil
		},
	}

	cmd.Flags().IntVar(&daysBack, "days-back", 7, "how many days back to query")
	cmd.Flags().IntVar(&staleMonths, "stale-months", 3,
		"how many months before a pull request is considered stale")
	cmd.Flags().BoolVar(&devMode, "dev", false, "dev mode, stop after first page of PRs")

	return cmd
}

func init() {
	rootCmd.AddCommand(newReportCommand())
}

func anyRequests(buckets ...stats.Bucket) bool {
	for _, b := range buckets {
		if len(b.Requests) > 0 {
			return true
		}
	}
	return false
}
