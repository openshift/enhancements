package stats

import (
	"fmt"
	"time"

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
	LGTM        bool // lgtm
	Prioritized bool // priority-important/soon or priority/critical-urgent
	Stale       bool // lifecycle/stake
}

// RuleFilter refers to a function that selects pull requests. A
// RuleFilter returns true when the request matches, false when it
// does not.
type RuleFilter func(*PullRequestDetails) bool

// Bucket describes a rule for selecting pull requests to group them
// into a category
type Bucket struct {
	// Rule tells us which pull requests belong in the bucket
	Rule RuleFilter
	// Requests is the set of pull requests in the bucket
	Requests []*PullRequestDetails
	// Cascade tells us whether to keep looking for other buckets. The
	// default, false, means stop when Rule matches. Setting Cascade =
	// true means requests added to the bucket may be added to other
	// buckets.
	Cascade bool
}

// Stats holds the overall stats gathered from the repo
type Stats struct {
	Query        *util.PullRequestQuery
	EarliestDate time.Time
	Buckets      []*Bucket
}

// Populate runs the query and filters requests into the appropriate
// buckets
func (s *Stats) Populate() error {
	return s.Query.IteratePullRequests(s.ProcessOne)
}

// Process extracts the required information from a single PR
func (s *Stats) ProcessOne(pr *github.PullRequest) error {
	// Ignore old closed items
	if !s.EarliestDate.IsZero() && *pr.State == "closed" && pr.UpdatedAt.Before(s.EarliestDate) {
		return nil
	}

	isMerged, err := s.Query.IsMerged(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not determine merged status of %s", *pr.HTMLURL))
	}

	issueComments, err := s.Query.GetIssueComments(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch issue comments on %s", *pr.HTMLURL))
	}

	prComments, err := s.Query.GetPRComments(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch PR comments on %s", *pr.HTMLURL))
	}

	reviews, err := s.Query.GetReviews(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch reviews on %s", *pr.HTMLURL))
	}

	lgtm := false
	prioritized := false
	stale := false
	for _, label := range pr.Labels {
		if *label.Name == "lgtm" {
			lgtm = true
		}
		if *label.Name == "priority/important-soon" || *label.Name == "priority/critical-urgent" {
			prioritized = true
		}
		if *label.Name == "lifecycle/stale" {
			stale = true
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
		Stale:               stale,
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

// add records a given pr in the correct bucket(s)
func (s *Stats) add(details *PullRequestDetails) {
	for _, bucket := range s.Buckets {
		match := bucket.Rule(details)
		if !match {
			continue
		}
		bucket.Requests = append(bucket.Requests, details)
		if !bucket.Cascade {
			break
		}
	}
}
