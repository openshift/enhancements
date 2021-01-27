package stats

import (
	"fmt"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"

	"github.com/openshift/enhancements/tools/util"
)

// PullRequestDetails includes the PullRequest and some supplementary
// data
type PullRequestDetails struct {
	Pull *github.PullRequest

	// These are groups of comments, submited with a review action
	Reviews           []*github.PullRequestReview
	RecentReviewCount int

	// These are "review comments", associated with a diff
	PullRequestComments  []*github.PullRequestComment
	RecentPRCommentCount int

	// PRs are also issues, so these are the standard comments
	IssueComments           []*github.IssueComment
	RecentIssueCommentCount int

	RecentActivityCount int
	AllActivityCount    int

	State       string
	LGTM        bool
	Prioritized bool
}

// New creates a new Stats implementation
func New(query *util.PullRequestQuery) *Stats {
	return &Stats{PullRequestQuery: query}
}

// Stats holds the overall stats gathered from the repo
type Stats struct {
	*util.PullRequestQuery

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

// Process extracts the required information from a single PR
func (s *Stats) Process(pr *github.PullRequest) error {
	// Ignore old closed items
	if *pr.State == "closed" && pr.UpdatedAt.Before(s.EarliestDate) {
		return nil
	}

	isMerged, err := s.IsMerged(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not determine merged status of %s", *pr.HTMLURL))
	}

	issueComments, err := s.GetIssueComments(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch issue comments on %s", *pr.HTMLURL))
	}

	prComments, err := s.GetPRComments(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch PR comments on %s", *pr.HTMLURL))
	}

	reviews, err := s.GetReviews(pr)
	if err != nil {
		return errors.Wrap(err,
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
		Pull:                pr,
		State:               *pr.State,
		IssueComments:       issueComments,
		PullRequestComments: prComments,
		Reviews:             reviews,
		LGTM:                lgtm,
		Prioritized:         prioritized,
	}
	if isMerged {
		details.State = "merged"
	}
	for _, r := range reviews {
		if r.SubmittedAt.After(s.EarliestDate) {
			details.RecentReviewCount++
		}
	}
	for _, c := range issueComments {
		if c.CreatedAt.After(s.EarliestDate) {
			details.RecentIssueCommentCount++
		}
	}
	for _, c := range prComments {
		if c.CreatedAt.After(s.EarliestDate) {
			details.RecentPRCommentCount++
		}
	}
	details.RecentActivityCount = details.RecentIssueCommentCount + details.RecentPRCommentCount + details.RecentReviewCount
	details.AllActivityCount = len(details.IssueComments) + len(details.PullRequestComments) + len(details.Reviews)
	s.add(details)
	return nil
}

func (s *Stats) add(details *PullRequestDetails) {
	s.All = append(s.All, details)

	if details.State == "merged" || details.State == "closed" {
		if details.Pull.ClosedAt.After(s.EarliestDate) {
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

	if details.Pull.CreatedAt.After(s.EarliestDate) {
		s.New = append(s.New, details)
		return
	}

	if details.Pull.UpdatedAt.Before(s.StaleDate) && details.RecentActivityCount > 0 {
		s.Old = append(s.Old, details)
		return
	}

	if details.Pull.UpdatedAt.Before(s.StaleDate) {
		s.Stale = append(s.Stale, details)
		return
	}

	if details.RecentActivityCount > 0 {
		s.Active = append(s.Active, details)
		return
	}

	s.Idle = append(s.Idle, details)
}
