package cmd

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/enhancements"
	"github.com/openshift/enhancements/tools/report"
	"github.com/openshift/enhancements/tools/stats"
	"github.com/openshift/enhancements/tools/util"
)

func newShowPRCommand() *cobra.Command {
	var daysBack int

	cmd := &cobra.Command{
		Use:       "show-pr",
		Short:     "Dump details for a pull request",
		ValidArgs: []string{"pull-request-id"},
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("please specify one valid pull request ID")
			}
			if _, err := strconv.Atoi(args[0]); err != nil {
				return errors.Wrap(err,
					fmt.Sprintf("pull request ID %q must be an integer", args[0]))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			prID, err := strconv.Atoi(args[0])
			if err != nil {
				return errors.Wrap(err,
					fmt.Sprintf("failed to interpret pull request ID %q as a number", args[0]))
			}
			group, isEnhancement, err := enhancements.GetGroup(prID)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to determine group for PR %d", prID))
			}

			ghClient := util.NewGithubClient(configSettings.Github.Token)
			ctx := context.Background()
			pr, _, err := ghClient.PullRequests.Get(ctx, orgName, repoName, prID)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to fetch pull request %d", prID))
			}

			earliestDate := time.Now().AddDate(0, 0, daysBack*-1)

			query := &util.PullRequestQuery{
				Org:     orgName,
				Repo:    repoName,
				DevMode: false,
				Client:  ghClient,
			}

			// Set up a Stats object so we can get the details for the
			// pull request.
			//
			// TODO: This is a bit clunky. Can we improve it without
			// forcing the low level report code to know all about
			// everything?
			all := stats.Bucket{
				Rule: func(prd *stats.PullRequestDetails) bool {
					return true
				},
			}
			reportBuckets := []*stats.Bucket{
				&all,
			}
			theStats := &stats.Stats{
				Query:        query,
				EarliestDate: earliestDate,
				Buckets:      reportBuckets,
			}
			if err := theStats.ProcessOne(pr); err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to fetch details for PR %d", prID))
			}

			report.ShowPRs(
				fmt.Sprintf("Pull Request %d", prID),
				all.Requests,
				true,
			)

			prd := all.Requests[0]

			var sinceUpdated float64
			var sinceClosed float64

			if prd.Pull.UpdatedAt != nil && !prd.Pull.UpdatedAt.IsZero() {
				sinceUpdated = time.Since(*prd.Pull.UpdatedAt).Hours() / 24
			}
			if prd.Pull.ClosedAt != nil && !prd.Pull.ClosedAt.IsZero() {
				sinceClosed = time.Since(*prd.Pull.ClosedAt).Hours() / 24
			}

			fmt.Printf("Last updated:   %s (%.02f days)\n", prd.Pull.UpdatedAt, sinceUpdated)
			fmt.Printf("Closed:         %s (%.02f days)\n", prd.Pull.ClosedAt, sinceClosed)
			fmt.Printf("Group:          %s\n", group)
			fmt.Printf("Enhancement:    %v\n", isEnhancement)
			fmt.Printf("State:          %q\n", prd.State)
			fmt.Printf("LGTM:           %v\n", prd.LGTM)
			fmt.Printf("Prioritized:    %v\n", prd.Prioritized)
			fmt.Printf("Stale:          %v\n", prd.Stale)
			fmt.Printf("Reviews:        %3d / %3d\n", prd.RecentReviewCount, len(prd.Reviews))
			fmt.Printf("PR Comments:    %3d / %3d\n", prd.RecentPRCommentCount, len(prd.PullRequestComments))
			fmt.Printf("Issue comments: %3d / %3d\n", prd.RecentIssueCommentCount, len(prd.IssueComments))

			return nil
		},
	}
	cmd.Flags().IntVar(&daysBack, "days-back", 7, "how many days back to query")

	return cmd
}

func init() {
	rootCmd.AddCommand(newShowPRCommand())
}
