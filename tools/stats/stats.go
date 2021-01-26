package stats

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

// GithubClientSource describes a function that can create a
// github.Client without any other inputs.
type GithubClientSource func() *github.Client

// PullRequestDetails includes the PullRequest and some supplementary
// data
type PullRequestDetails struct {
	Pull                *github.PullRequest
	Reviews             []*github.PullRequestReview
	RecentReviewCount   int
	Comments            []*github.PullRequestComment
	RecentCommentCount  int
	RecentActivityCount int
	AllActivityCount    int
	State               string
	LGTM                bool
	Prioritized         bool
}

// New creates a new Stats implementation
func New(daysBack int, staleMonths int, orgName, repoName string, devMode bool, clientSource GithubClientSource) (*Stats, error) {
	result := &Stats{
		org:          orgName,
		repo:         repoName,
		earliestDate: time.Now().AddDate(0, 0, daysBack*-1),
		staleDate:    time.Now().AddDate(0, staleMonths*-1, 0),
		devMode:      devMode,
		clientSource: clientSource,
	}
	return result, nil
}

// Stats holds the overall stats gathered from the repo
type Stats struct {
	org          string
	repo         string
	earliestDate time.Time
	staleDate    time.Time
	devMode      bool
	clientSource GithubClientSource

	All     []*PullRequestDetails
	New     []*PullRequestDetails
	Merged  []*PullRequestDetails
	Closed  []*PullRequestDetails
	Stale   []*PullRequestDetails
	Idle    []*PullRequestDetails
	Active  []*PullRequestDetails
	Old     []*PullRequestDetails
	Revived []*PullRequestDetails
}

const pageSize int = 50

// PRCallback is a type for callbacks for processing pull requests
type PRCallback func(*github.PullRequest) error

// IteratePullRequests queries for all pull requests and invokes the
// callback with each PR individually
func (s *Stats) IteratePullRequests() error {
	fmt.Printf("finding pull requests for %s/%s\n", s.org, s.repo)
	fmt.Printf("ignoring items closed before %s\n", s.earliestDate)

	client := s.clientSource()
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
		prs, response, err := client.PullRequests.List(ctx, s.org, s.repo, opts)
		if err != nil {
			return errors.Wrap(err,
				fmt.Sprintf(
					"could not get pull requests for %s/%s", s.org, s.repo))
		}
		for _, pr := range prs {
			details, err := s.makeDetails(pr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\ncould not process pull request %s: %s\n",
					*pr.HTMLURL, err)
				continue
			}
			s.add(details)
			fmt.Fprintf(os.Stderr, ".")
		}

		if s.devMode {
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

func (s *Stats) getComments(pr *github.PullRequest) ([]*github.PullRequestComment, error) {
	c := s.clientSource()
	ctx := context.Background()
	opts := &github.PullRequestListCommentsOptions{
		Since: s.earliestDate,
		ListOptions: github.ListOptions{
			PerPage: pageSize,
		},
	}
	results := []*github.PullRequestComment{}

	for {
		comments, response, err := c.PullRequests.ListComments(
			ctx, s.org, s.repo, *pr.Number, opts)
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

func (s *Stats) getReviews(pr *github.PullRequest) ([]*github.PullRequestReview, error) {
	c := s.clientSource()
	ctx := context.Background()
	opts := &github.ListOptions{
		PerPage: pageSize,
	}
	results := []*github.PullRequestReview{}

	for {
		comments, response, err := c.PullRequests.ListReviews(
			ctx, s.org, s.repo, *pr.Number, opts)
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

func (s *Stats) makeDetails(pr *github.PullRequest) (*PullRequestDetails, error) {
	// Ignore old closed items
	if *pr.State == "closed" && pr.UpdatedAt.Before(s.earliestDate) {
		return nil, nil
	}

	c := s.clientSource()
	ctx := context.Background()
	isMerged, _, err := c.PullRequests.IsMerged(ctx, s.org, s.repo, *pr.Number)
	if err != nil {
		return nil, errors.Wrap(err,
			fmt.Sprintf("could not determine merged status of %s", *pr.HTMLURL))
	}

	comments, err := s.getComments(pr)
	if err != nil {
		return nil, errors.Wrap(err,
			fmt.Sprintf("could not fetch comments on %s", *pr.HTMLURL))
	}

	reviews, err := s.getReviews(pr)
	if err != nil {
		return nil, errors.Wrap(err,
			fmt.Sprintf("could not fetch reviews on %s", *pr.HTMLURL))
	}

	lgtm := false
	prioritized := false
	for _, label := range pr.Labels {
		if *label.Name == "lgtm" {
			lgtm = true
		}
		if *label.Name == "priority/important-soon" || *label.Name == "priority/critical-urgent" {
			prioritized = true
		}
	}

	details := &PullRequestDetails{
		Pull:        pr,
		State:       *pr.State,
		Comments:    comments,
		Reviews:     reviews,
		LGTM:        lgtm,
		Prioritized: prioritized,
	}
	if isMerged {
		details.State = "merged"
	}
	for _, r := range reviews {
		if r.SubmittedAt.After(s.earliestDate) {
			details.RecentReviewCount++
		}
	}
	for _, c := range comments {
		if c.CreatedAt.After(s.earliestDate) {
			details.RecentCommentCount++
		}
	}
	details.RecentActivityCount = details.RecentCommentCount + details.RecentReviewCount
	details.AllActivityCount = len(details.Comments) + len(details.Reviews)
	return details, nil
}

func (s *Stats) add(details *PullRequestDetails) {
	if details == nil {
		return
	}

	s.All = append(s.All, details)

	if details.State == "merged" || details.State == "closed" {
		if details.Pull.ClosedAt.After(s.earliestDate) {
			// The PR closed this period.
			if details.State == "merged" {
				s.Merged = append(s.Merged, details)
				return
			}
			if details.State == "closed" {
				s.Closed = append(s.Closed, details)
				return
			}
		}
		// The PR has had commentary this period but was closed
		// earlier.
		s.Revived = append(s.Revived, details)
		return
	}

	if details.Pull.CreatedAt.After(s.earliestDate) {
		s.New = append(s.New, details)
		return
	}

	if details.Pull.UpdatedAt.Before(s.staleDate) && details.RecentActivityCount > 0 {
		s.Old = append(s.Old, details)
		return
	}

	if details.Pull.UpdatedAt.Before(s.staleDate) {
		s.Stale = append(s.Stale, details)
		return
	}

	if details.RecentActivityCount > 0 {
		s.Active = append(s.Active, details)
		return
	}

	s.Idle = append(s.Idle, details)
}
