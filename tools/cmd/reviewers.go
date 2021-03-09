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

	"github.com/pkg/errors"
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
		daysBack     int
		devMode      bool
		numReviewers int
	)
	ignoreReviewers := multiStringArg{}

	cmd := &cobra.Command{
		Use:   "reviewers",
		Short: "List reviewers of PRs in a repo",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
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
				return errors.Wrap(err, "failed to retrieve pull request details")
			}

			orderedReviewers := reviewerStats.ReviewersInOrder(ignoreReviewers)
			if numReviewers > 0 {
				orderedReviewers = orderedReviewers[:numReviewers]
			}

			for _, reviewer := range orderedReviewers {
				count := reviewerStats.ReviewCounts[reviewer]
				fmt.Printf("%3d: %s\n", count, reviewer)

				prs := reviewerStats.PRsForReviewer(reviewer)
				sort.Slice(prs, func(i, j int) bool {
					return prs[i].ReviewCount > prs[j].ReviewCount
				})
				for _, prWithCount := range prs {
					pr := prWithCount.PR
					fmt.Printf("\t%3d: %s %q\n", prWithCount.ReviewCount, *pr.URL, *pr.Title)
				}
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&daysBack, "days-back", 31, "how many days back to query, defaults to 31")
	cmd.Flags().BoolVar(&devMode, "dev", false, "dev mode, stop after first page of PRs")
	cmd.Flags().IntVar(&numReviewers, "num", 10, "number of reviewers to show, 0 is all")
	cmd.Flags().Var(&ignoreReviewers, "ignore", "ignore a reviewer, can be repeated")

	return cmd
}

func init() {
	rootCmd.AddCommand(newReviewersCommand())
}
