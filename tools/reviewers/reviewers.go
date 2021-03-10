package reviewers

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"

	"github.com/openshift/enhancements/tools/util"
)

type Stats struct {
	Query            *util.PullRequestQuery
	EarliestDate     time.Time
	ReviewCounts     map[string]int32
	allPRs           map[int]*github.PullRequest
	ReviewCountsByPR map[string]map[int]int
}

func strInSlice(input string, slice []string) bool {
	for _, s := range slice {
		if input == s {
			return true
		}
	}
	return false
}

func (s *Stats) ReviewersInOrder(ignore []string) []string {
	type kv struct {
		Key   string
		Value int32
	}

	var sorted []kv
	for k, v := range s.ReviewCounts {
		if strInSlice(k, ignore) {
			continue
		}
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	result := make([]string, len(sorted))
	for i, j := range sorted {
		result[i] = j.Key
	}
	return result
}

type PRWithCount struct {
	PR          *github.PullRequest
	ReviewCount int
}

func (s *Stats) PRsForReviewer(name string) []PRWithCount {
	prMap := s.ReviewCountsByPR[name]
	if prMap == nil {
		return nil
	}
	prs := []PRWithCount{}
	for prNum, count := range prMap {
		prs = append(prs, PRWithCount{PR: s.allPRs[prNum], ReviewCount: count})
	}
	return prs
}

func getName(user *github.User) string {
	if user.Name != nil {
		return *user.Name
	}
	if user.Login != nil {
		return *user.Login
	}
	return "unnamed"
}

func (s *Stats) ProcessOne(pr *github.PullRequest) error {

	if s.ReviewCounts == nil {
		s.ReviewCounts = make(map[string]int32)
	}
	if s.ReviewCountsByPR == nil {
		s.ReviewCountsByPR = make(map[string]map[int]int)
	}
	if s.allPRs == nil {
		s.allPRs = make(map[int]*github.PullRequest)
	}

	if pr.UpdatedAt.Before(s.EarliestDate) {
		return nil
	}

	s.allPRs[*pr.Number] = pr

	incrementPR := func(name string) {
		if s.ReviewCountsByPR[name] == nil {
			s.ReviewCountsByPR[name] = make(map[int]int)
		}
		s.ReviewCountsByPR[name][*pr.Number]++
	}

	issueComments, err := s.Query.GetIssueComments(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch issue comments on %s", *pr.HTMLURL))
	}
	for _, c := range issueComments {
		if c.CreatedAt.IsZero() || c.CreatedAt.Before(s.EarliestDate) {
			continue
		}
		name := getName(c.User)
		s.ReviewCounts[name]++
		incrementPR(name)
	}

	prComments, err := s.Query.GetPRComments(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch PR comments on %s", *pr.HTMLURL))
	}
	for _, c := range prComments {
		if c.CreatedAt.IsZero() || c.CreatedAt.Before(s.EarliestDate) {
			continue
		}
		name := getName(c.User)
		s.ReviewCounts[name]++
		incrementPR(name)
	}

	reviews, err := s.Query.GetReviews(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch reviews on %s", *pr.HTMLURL))
	}
	for _, r := range reviews {
		if r.SubmittedAt == nil || r.SubmittedAt.IsZero() || r.SubmittedAt.Before(s.EarliestDate) {
			continue
		}
		name := getName(r.User)
		s.ReviewCounts[name]++
		incrementPR(name)
	}

	return nil
}
