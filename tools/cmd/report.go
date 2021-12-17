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
		daysBack int
	)

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate the weekly activity report",
		RunE: func(cmd *cobra.Command, args []string) error {

			earliestDate := time.Now().AddDate(0, 0, daysBack*-1)

			query := &util.PullRequestQuery{
				Org:     orgName,
				Repo:    repoName,
				DevMode: devMode,
				Client:  util.NewGithubClient(configSettings.Github.Token),
			}

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
					// ignore pull requests with only changes in the
					// hack, tools, or this-week directories
					return prd.Group == "hack" || prd.Group == "tools" || prd.Group == "this-week"
				},
			}

			// Define a few buckets for pull requests related to work
			// prioritized for the current or next release.
			prioritizedMerged := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Prioritized && prd.State == "merged" && prd.Pull.ClosedAt.After(earliestDate)
				},
			}
			prioritizedClosed := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Prioritized && prd.State == "closed" && prd.Pull.ClosedAt.After(earliestDate)
				},
			}
			prioritizedNew := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Prioritized && prd.Pull.CreatedAt.After(earliestDate)
				},
			}
			prioritizedRevived := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Prioritized && prd.State == "merged" && prd.RecentActivityCount > 0
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
					return prd.State == "merged" && prd.Pull.ClosedAt.After(earliestDate)
				},
			}
			otherClosed := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.State == "closed" && prd.Pull.ClosedAt.After(earliestDate)
				},
			}
			otherNew := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return prd.Pull.CreatedAt.After(earliestDate)
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
				&prioritizedRevived,
				&prioritizedActive,

				&otherMerged,
				&otherClosed,
				&revived,
				&otherNew,

				&stale,
				&otherActive,
				&idle,
			}

			summarizer, err := enhancements.NewSummarizer()
			if err != nil {
				return errors.Wrap(err, "unable to show PR summaries")
			}

			theStats := &stats.Stats{
				Query:        query,
				EarliestDate: earliestDate,
				Buckets:      reportBuckets,
				Summarizer:   summarizer,
			}

			fmt.Fprintf(os.Stderr, "finding pull requests for %s/%s\n", orgName, repoName)
			fmt.Fprintf(os.Stderr, "ignoring items closed before %s\n", earliestDate)

			err = theStats.Populate()
			if err != nil {
				return errors.Wrap(err, "could not generate stats")
			}

			fmt.Fprintf(os.Stderr, "Processed %d pull requests\n", len(all.Requests))
			fmt.Fprintf(os.Stderr, "Ignored %d pull requests\n", len(ignore.Requests))

			year, month, day := time.Now().Date()
			fmt.Printf("# This Week in Enhancements - %d-%.2d-%.2d\n", year, month, day)

			startYear, startMonth, startDay := earliestDate.Date()
			fmt.Printf("\n*Updates since %d-%.2d-%.2d*\n\n", startYear, startMonth, startDay)

			// Only print the priority section if there are prioritized pull requests
			if anyRequests(prioritizedMerged, prioritizedClosed, prioritizedNew, prioritizedRevived, prioritizedActive) {
				fmt.Printf("\n## Enhancements for Release Priorities\n")

				report.SortByID(prioritizedMerged.Requests)
				report.ShowPRs(summarizer, "Prioritized Merged", prioritizedMerged.Requests, true, false)

				report.SortByID(prioritizedNew.Requests)
				report.ShowPRs(summarizer, "Prioritized New", prioritizedNew.Requests, true, true)

				report.SortByID(prioritizedRevived.Requests)
				report.ShowPRs(summarizer,
					"Prioritized Revived (discussion after PR was merged)",
					prioritizedRevived.Requests,
					false,
					false,
				)

				report.SortByActivityCountDesc(prioritizedActive.Requests)
				report.ShowPRs(summarizer, "Prioritized Active", prioritizedActive.Requests, true, true)

				report.SortByID(prioritizedClosed.Requests)
				report.ShowPRs(summarizer, "Prioritized Closed", prioritizedClosed.Requests, false, false)
			}

			fmt.Printf("\n## Other Enhancements\n")

			report.SortByID(otherMerged.Requests)
			report.ShowPRs(summarizer, "Other Merged", otherMerged.Requests, true, false)

			report.SortByID(otherNew.Requests)
			report.ShowPRs(summarizer, "Other New", otherNew.Requests, true, true)

			report.SortByActivityCountDesc(otherActive.Requests)
			report.ShowPRs(summarizer, "Other Active", otherActive.Requests, false, false)

			report.SortByID(otherClosed.Requests)
			report.ShowPRs(summarizer, "Other Closed", otherClosed.Requests, false, false)

			report.SortByID(revived.Requests)
			report.ShowPRs(summarizer,
				fmt.Sprintf("Revived (closed more than %d days ago, but with new comments)", daysBack),
				revived.Requests,
				false,
				false,
			)

			report.SortByID(idle.Requests)
			report.ShowPRs(summarizer,
				fmt.Sprintf("Idle (no comments for at least %d days)", daysBack),
				idle.Requests,
				false,
				false,
			)

			report.SortByID(stale.Requests)
			report.ShowPRs(summarizer,
				"Other lifecycle/stale or lifecycle/rotten",
				stale.Requests,
				false,
				false,
			)

			return nil
		},
	}

	cmd.Flags().IntVar(&daysBack, "days-back", 7, "how many days back to query")

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
