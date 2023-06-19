/*
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
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/reviewers"
	"github.com/openshift/enhancements/tools/util"
)

type multiStringArg []string

func (m *multiStringArg) String() string {
	return ""
}

func (m *multiStringArg) Type() string {
	return "github_id"
}

func (m *multiStringArg) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func newReviewersCommand() *cobra.Command {
	var (
		daysBack      int
		numReviewers  int
		summaryMode   bool
		minNumReviews int
	)
	ignoreReviewers := multiStringArg{}

	cmd := &cobra.Command{
		Use:   "reviewers",
		Short: "List reviewers of PRs in a repo",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			initConfig()

			earliestDate := time.Now().AddDate(0, 0, daysBack*-1)

			query := &util.PullRequestQuery{
				Org:     orgName,
				Repo:    repoName,
				DevMode: devMode,
				Client:  util.NewGithubClient(configSettings.Github.Token),
			}

			reviewerStats := &reviewers.Stats{
				Query:        query,
				EarliestDate: earliestDate,
			}

			err := query.IteratePullRequests(reviewerStats.ProcessOne)
			if err != nil {
				return fmt.Errorf("failed to retrieve pull request details: %w", err)
			}

			// The command line ignore options override the config file.
			var toIgnore []string
			toIgnore = configSettings.Reviewers.Ignore
			if len(ignoreReviewers) > 0 {
				toIgnore = ignoreReviewers
			}

			orderedReviewers := reviewerStats.ReviewersInOrder(toIgnore)
			if numReviewers > 0 {
				orderedReviewers = orderedReviewers[:numReviewers]
			}

			for _, reviewer := range orderedReviewers {
				count := reviewerStats.ReviewCounts[reviewer]
				if count < minNumReviews {
					continue
				}
				prs := reviewerStats.PRsForReviewer(reviewer)

				fmt.Printf("%d/%d: %s\n", count, len(prs), reviewer)

				sort.Slice(prs, func(i, j int) bool {
					return prs[i].ReviewCount > prs[j].ReviewCount
				})
				if summaryMode != true {
					for _, prWithCount := range prs {
						pr := prWithCount.PR
						fmt.Printf("\t%3d: %s [%s] %q\n", prWithCount.ReviewCount,
							*pr.HTMLURL, *pr.User.Login, *pr.Title)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&daysBack, "days-back", 31, "how many days back to query, defaults to 31")
	cmd.Flags().IntVar(&numReviewers, "num", 10, "number of reviewers to show, 0 is all")
	cmd.Flags().Var(&ignoreReviewers, "ignore", "ignore a reviewer, can be repeated")
	cmd.Flags().BoolVar(&summaryMode, "summary", false, "show only summary (reviewer identity and totals)")
	cmd.Flags().IntVar(&minNumReviews, "min-reviews", 0, "show only reviewers with at least a minimum number of reviews")

	return cmd
}

func init() {
	rootCmd.AddCommand(newReviewersCommand())
}
