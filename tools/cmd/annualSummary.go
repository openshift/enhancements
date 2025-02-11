/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/enhancements"
	"github.com/openshift/enhancements/tools/report"
	"github.com/openshift/enhancements/tools/stats"
	"github.com/openshift/enhancements/tools/util"
)

// which year? (default to last year)
// FIXME: Add some logic to figure out if we're running this
// late in December and adjust earliestDate and latestDate
// accordingly.
var year = time.Now().Year() - 1

// annualSummaryCmd represents the annualSummary command
var annualSummaryCmd = &cobra.Command{
	Use:   "annual-summary",
	Short: "Summarize the enhancements work over the previous year",
	// Long: ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		initConfig()

		earliestDate := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
		latestDate := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC)

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

		// Define basic groups for all of the non-prioritized pull
		// requests.
		otherMerged := stats.Bucket{
			Rule: func(prd *stats.PullRequestDetails) bool {
				return prd.State == "merged" && prd.Pull.ClosedAt.After(earliestDate)
			},
		}
		otherOpen := stats.Bucket{
			Rule: func(prd *stats.PullRequestDetails) bool {
				return prd.State != "merged" && prd.State != "closed" && prd.Pull.CreatedAt.After(earliestDate)
			},
		}

		// Anything that falls through the other lists will be ignored.
		remainder := stats.Bucket{
			Rule: func(prd *stats.PullRequestDetails) bool {
				return true
			},
		}

		reportBuckets := []*stats.Bucket{
			&all,
			&ignore,
			&otherMerged,
			&otherOpen,
			&remainder,
		}

		summarizer, err := enhancements.NewSummarizer()
		if err != nil {
			return fmt.Errorf("unable to show PR summaries: %w", err)
		}

		theStats := &stats.Stats{
			Query:        query,
			EarliestDate: earliestDate,
			LatestDate:   latestDate,
			Buckets:      reportBuckets,
			Summarizer:   summarizer,
		}

		fmt.Fprintf(os.Stderr, "finding pull requests for %s/%s\n", orgName, repoName)
		fmt.Fprintf(os.Stderr, "including items between %s and %s\n", earliestDate, latestDate)

		err = theStats.Populate()
		if err != nil {
			return fmt.Errorf("could not generate stats: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Processed %d pull requests\n", len(all.Requests))
		totalIgnored := len(ignore.Requests) + len(remainder.Requests)
		fmt.Fprintf(os.Stderr, "Ignored %d pull requests\n", totalIgnored)

		fmt.Printf("# The Year in Enhancements - %d\n", earliestDate.Year())

		fmt.Printf("\n## Enhancements\n")

		report.SortByID(otherMerged.Requests)
		report.ShowPRs(summarizer, "Merged", otherMerged.Requests, false, false)

		report.SortByID(otherOpen.Requests)
		report.ShowPRs(summarizer, "Open", otherOpen.Requests, false, false)

		return nil
	},
}

func init() {
	annualSummaryCmd.Flags().IntVar(&year, "year", year, "which year")

	rootCmd.AddCommand(annualSummaryCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// annualSummaryCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// annualSummaryCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
