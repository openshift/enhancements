package util

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

// PullRequestQuery holds the parameters for iterating over pull requests
type PullRequestQuery struct {
	Org     string
	Repo    string
	DevMode bool
	Client  *github.Client
}

const pageSize int = 50

// PRCallback is a type for callbacks for processing pull requests
type PRCallback func(*github.PullRequest) error

// IteratePullRequests queries for all pull requests and invokes the
// callback with each PR individually
func (q *PullRequestQuery) IteratePullRequests(callback PRCallback) error {

	ctx := context.Background()
	opts := &github.PullRequestListOptions{
		State: "all",
		ListOptions: github.ListOptions{
			PerPage: pageSize,
		},
	}

	// Fetch the details of the pull requests in batches. We want some
	// parallelization, but also want to limit the number of
	// simultaneous requests we make to the API to avoid rate
	// limiting.
	for {
		prs, response, err := q.Client.PullRequests.List(ctx, q.Org, q.Repo, opts)
		if err != nil {
			return errors.Wrap(err,
				fmt.Sprintf(
					"could not get pull requests for %s/%s", q.Org, q.Repo))
		}
		for _, pr := range prs {
			err := callback(pr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\ncould not process pull request %s: %s\n",
					*pr.HTMLURL, err)
				continue
			}
			fmt.Fprintf(os.Stderr, ".")
		}

		if q.DevMode {
			fmt.Fprintf(os.Stderr, "shortcutting for dev mode\n")
			break
		}

		if response.NextPage == 0 {
			break
		}
		opts.Page = response.NextPage
	}

	fmt.Fprintf(os.Stderr, "\n")

	return nil
}

func (q *PullRequestQuery) GetIssueComments(pr *github.PullRequest) ([]*github.IssueComment, error) {
	ctx := context.Background()
	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: pageSize,
		},
	}
	results := []*github.IssueComment{}

	for {
		comments, response, err := q.Client.Issues.ListComments(
			ctx, q.Org, q.Repo, *pr.Number, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, comments...)
		if response.NextPage == 0 {
			break
		}
		opts.Page = response.NextPage
	}

	return results, nil
}

func (q *PullRequestQuery) GetPRComments(pr *github.PullRequest) ([]*github.PullRequestComment, error) {
	ctx := context.Background()
	opts := &github.PullRequestListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: pageSize,
		},
	}
	results := []*github.PullRequestComment{}

	for {
		comments, response, err := q.Client.PullRequests.ListComments(
			ctx, q.Org, q.Repo, *pr.Number, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, comments...)
		if response.NextPage == 0 {
			break
		}
		opts.Page = response.NextPage
	}

	return results, nil
}

func (q *PullRequestQuery) GetReviews(pr *github.PullRequest) ([]*github.PullRequestReview, error) {
	ctx := context.Background()
	opts := &github.ListOptions{
		PerPage: pageSize,
	}
	results := []*github.PullRequestReview{}

	for {
		comments, response, err := q.Client.PullRequests.ListReviews(
			ctx, q.Org, q.Repo, *pr.Number, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, comments...)
		if response.NextPage == 0 {
			break
		}
		opts.Page = response.NextPage
	}

	return results, nil
}

func (q *PullRequestQuery) IsMerged(pr *github.PullRequest) (bool, error) {
	ctx := context.Background()
	isMerged, _, err := q.Client.PullRequests.IsMerged(ctx, q.Org, q.Repo, *pr.Number)
	return isMerged, err
}
