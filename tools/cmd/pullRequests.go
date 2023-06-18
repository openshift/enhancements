/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

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
	"encoding/csv"
	"fmt"
	"os"
	"time"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/stats"
	"github.com/openshift/enhancements/tools/util"
)

const dateFmt = "2006-01-02"

func newPullRequestsCommand() *cobra.Command {

	var (
		includeUpdates bool
		daysBack       int
	)

	// pullRequestsCmd represents the pullRequests command
	var pullRequestsCmd = &cobra.Command{
		Use:   "pull-requests",
		Short: "List pull requests and some characteristics in CSV format",
		Long:  `Produce a CSV list of pull requests suitable for import into a spreadsheet.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			initConfig()

			earliestDate := time.Now().AddDate(0, 0, daysBack*-1)

			query := &util.PullRequestQuery{
				Org:     orgName,
				Repo:    repoName,
				DevMode: devMode,
				Client:  util.NewGithubClient(configSettings.Github.Token),
			}

			all := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {

					// FIXME: Add an option to include all PRs
					if prd.State != "merged" {
						return false
					}

					if !includeUpdates && !prd.IsNew {
						return false
					}

					// FIXME: add an option to include non-enhancements
					if !prd.IsEnhancement {
						return false
					}

					return true
				},
				Cascade: false,
			}
			theStats := &stats.Stats{
				Query:        query,
				EarliestDate: earliestDate,
				Buckets:      []*stats.Bucket{&all},
			}
			err := theStats.Populate()
			if err != nil {
				return errors.Wrap(err, "could not generate stats")
			}

			out := csv.NewWriter(os.Stdout)
			err = out.Write([]string{
				"ID",
				"Title",
				"State",
				"Author",
				"URL",
				"New",
				"Created",
				"Closed",
				"Days to Merge",
			})
			if err != nil {
				return errors.Wrap(err, "could not write field headers")
			}

			for _, prd := range all.Requests {

				var (
					createdAt, closedAt, isNew string
					daysToMerge                int = -1
				)

				if prd.Pull.CreatedAt != nil {
					createdAt = prd.Pull.CreatedAt.Format(dateFmt)
				}
				if prd.Pull.ClosedAt != nil {
					closedAt = prd.Pull.ClosedAt.Format(dateFmt)
				}
				if prd.State == "merged" && prd.Pull.CreatedAt != nil && prd.Pull.ClosedAt != nil {
					daysToMerge = int(prd.Pull.ClosedAt.Sub(*prd.Pull.CreatedAt).Hours() / 24)
				}

				user := getName(prd.Pull.User)

				isNew = "false"
				if prd.IsNew {
					isNew = "true"
				}

				err = out.Write([]string{
					fmt.Sprintf("%d", *prd.Pull.Number),
					*prd.Pull.Title,
					prd.State,
					user,
					*prd.Pull.HTMLURL,
					isNew,
					createdAt,
					closedAt,
					fmt.Sprintf("%d", daysToMerge),
				})
				if err != nil {
					return errors.Wrap(err, "could not write record")
				}
			}

			out.Flush()

			return nil
		},
	}

	pullRequestsCmd.Flags().BoolVarP(&includeUpdates, "include-updates", "U", false, "include updates to existing enhancements")
	pullRequestsCmd.Flags().StringP("output", "o", "", "output file to create (defaults to stdout)")
	pullRequestsCmd.Flags().IntVar(&daysBack, "days-back", 90, "how many days back to query")

	return pullRequestsCmd
}

func getName(user *github.User) string {
	if user == nil {
		return "unnamed"
	}
	if user.Name != nil {
		return *user.Name
	}
	if user.Login != nil {
		return *user.Login
	}
	return "unnamed"
}

func init() {
	rootCmd.AddCommand(newPullRequestsCommand())
}
