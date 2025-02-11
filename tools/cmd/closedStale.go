package cmd

/*
Copyright © 2023 Doug Hellmann <dhellmann@redhat.com>

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

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/google/go-github/v32/github"
	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/enhancements"
	"github.com/openshift/enhancements/tools/util"
)

const jiraTicketURLBase string = "https://issues.redhat.com/browse/"
const noJiraMessage string = "It does not appear to be linked to a valid Jira ticket."
const messageTemplate string = "(automated message) This pull request is closed with lifecycle/rotten. %s Should the PR be reopened, updated, and merged? If not, removing the lifecycle/rotten label will tell this bot to ignore it in the future."

var (
	jiraIDPattern  = regexp.MustCompile("[a-zA-Z0-9]+-[0-9]+")
	jiraURLPattern = regexp.MustCompile(jiraTicketURLBase + "[a-zA-Z0-9]+-[0-9]+")
)

func getJiraID(summarizer *enhancements.Summarizer, issue *github.Issue) (string, error) {
	isEnhancement, err := summarizer.IsEnhancement(*issue.Number)
	if err != nil {
		return "", fmt.Errorf("failed to determine if %d is an enhancement: %w",
			*issue.Number, err)
	}
	if !isEnhancement {
		// not an enhancement
		return "", nil
	}

	// It is an enhancement, see if we can get a tracking URL
	metaData, err := summarizer.GetMetaData(*issue.Number)
	if err != nil {
		// Could not parse the metadata, probably someone has an unquoted @.
		// Try to get the tracking link via regex.
		enhancementFilename, err := summarizer.GetEnhancementFilename(*issue.Number)
		if err != nil {
			return "", fmt.Errorf("failed to determine name of enhancement file: %w", err)
		}
		contentBytes, err := summarizer.GetFileContents(*issue.Number, enhancementFilename)
		if err != nil {
			return "", fmt.Errorf("failed to get contents of %q from %d: %w", enhancementFilename, *issue.Number, err)
		}
		results := jiraURLPattern.FindAllString(string(contentBytes), -1)
		if len(results) > 0 {
			return strings.TrimPrefix(results[0], jiraTicketURLBase), nil
		}
		return "", fmt.Errorf("failed to get metadata for %d: %w", *issue.Number, err)
	}
	// If we have a tracking link from jira, that's definitely what we want.
	for _, link := range metaData.TrackingLinks {
		if strings.HasPrefix(link, jiraTicketURLBase) {
			return strings.TrimPrefix(link, jiraTicketURLBase), nil
		}
	}

	// If there is no tracking link, an ID in the title is most likely
	// to be right.
	if issue.Title != nil {
		results := jiraIDPattern.FindAllString(*issue.Title, -1)
		if len(results) != 0 {
			return results[0], nil
		}
	}

	// Last resort, look at the body of the PR
	if issue.Body != nil {
		results := jiraIDPattern.FindAllString(*issue.Body, -1)
		if len(results) != 0 {
			return results[0], nil
		}
	}
	return "", nil
}

func getApproverNames(summarizer *enhancements.Summarizer, pr int) ([]string, error) {
	metaData, err := summarizer.GetMetaData(pr)
	if err != nil {
		return nil, fmt.Errorf("failed to load approvers from metadata: %w", err)
	}
	names := []string{}
	for _, name := range metaData.Approvers {
		nameParts := strings.Split(name, " ")
		candidateName := strings.Trim(nameParts[0], "@,")
		names = append(names, candidateName)
	}
	return names, nil
}

func newClosedStaleCommand() *cobra.Command {
	var (
		daysBack int
		dryRun   bool
		verbose  bool
	)

	cmd := &cobra.Command{
		Use:   "closed-stale",
		Short: "Process PRs that were closed as stale",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			initConfig()
			initJiraConfig()

			earliestDate := time.Now().AddDate(0, 0, daysBack*-1)

			ctx := context.Background()
			ghClient := util.NewGithubClient(configSettings.Github.Token)
			summarizer, err := enhancements.NewSummarizer()
			if err != nil {
				return fmt.Errorf("failed to initialize summarizer: %w", err)
			}
			jiraTransport := jira.BearerAuthTransport{
				//Username: jiraSettings.Login,
				Token: jiraToken,
			}
			jiraClient, err := jira.NewClient(jiraTransport.Client(), jiraSettings.Server)
			if err != nil {
				return fmt.Errorf("failed to create jira client: %w", err)
			}

			processedCount := 0
			totalCount := 0
			queryString := "repo:openshift/enhancements is:pr is:closed is:unmerged label:lifecycle/rotten"
			opts := &github.SearchOptions{
				Sort:  "updated",
				Order: "desc",
				ListOptions: github.ListOptions{
					PerPage: 50,
				},
			}

			for {
				results, response, err := ghClient.Search.Issues(ctx, queryString, opts)
				if err != nil {
					return err
				}

				for _, issue := range results.Issues {
					totalCount++

					showSummary := func(message string) {
						fmt.Printf("\n%s: %s\n%s\n", *issue.HTMLURL, *issue.Title, message)
						approvers, err := getApproverNames(summarizer, *issue.Number)
						if err != nil {
							fmt.Printf("\t%s\n", err)
						} else {
							fmt.Printf("\tApprovers: %v\n", approvers)
						}
					}

					if issue.UpdatedAt.Before(earliestDate) {
						if verbose {
							showSummary(fmt.Sprintf("Closed on %s, before %s",
								issue.ClosedAt.Format("2006-01-02"),
								earliestDate.Format("2006-01-02")))
						}
						continue
					}

					// Convenience function for dealing with a PR that needs a message added.
					handlePR := func(message string) {
						processedCount++

						if dryRun {
							showSummary(message)
							return
						}

						fullMessage := fmt.Sprintf(messageTemplate, message)
						showSummary(fullMessage)

						comment := &github.IssueComment{
							Body: &fullMessage,
						}
						_, _, err := ghClient.Issues.CreateComment(ctx, "openshift", "enhancements",
							*issue.Number, comment)
						if err != nil {
							fmt.Printf("Error adding comment: %s\n", err)
						}
					}

					jiraID, err := getJiraID(summarizer, issue) // error logged in conditional
					if jiraID == "" {
						handlePR(noJiraMessage)
						// log the message from determining the ID
						// here so it is formatted nicely with the
						// handler function output
						if err != nil {
							fmt.Printf("Could not get jira ID: %s\n", err)
						}
						continue
					}

					// Decide what to do based on the jira ticket status

					jiraState := "unknown"
					ticket, _, err := jiraClient.Issue.GetWithContext(ctx, jiraID, nil)
					if err != nil {
						handlePR(noJiraMessage)
						fmt.Printf("\tFailed to load jira ticket %s: %s\n", jiraID, err)
						continue
					}

					status := ticket.Fields.Status.Name

					jiraState = status
					if status == "Closed" {
						resolution := ticket.Fields.Resolution.Name
						jiraState = fmt.Sprintf("Closed, %s", resolution)
					}

					message := fmt.Sprintf("The associated Jira ticket, %s, has status %q.", jiraID, jiraState)
					handlePR(message)
				}

				if response.NextPage == 0 {
					break
				}
				opts.Page = response.NextPage
				if devMode {
					break
				}
			}

			fmt.Printf("\nHandled %d of %d PRs\n", processedCount, totalCount)
			return nil
		},
	}

	cmd.Flags().IntVar(&daysBack, "closed-in", 90, "ignore closed PRs older than this many days")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report without commenting")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "give more detail")
	return cmd
}

func init() {
	rootCmd.AddCommand(newClosedStaleCommand())

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// closedStaleCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// closedStaleCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
