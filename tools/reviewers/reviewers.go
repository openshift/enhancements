package reviewers

import (
	"fmt"
	"sort"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"

	"github.com/openshift/enhancements/tools/util"
)

type Stats struct {
	Query        *util.PullRequestQuery
	ReviewCounts map[string]int32
	byReviewer   map[string]map[int]*github.PullRequest
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

func (s *Stats) PRsForReviewer(name string) []*github.PullRequest {
	prMap := s.byReviewer[name]
	if prMap == nil {
		return nil
	}
	prs := []*github.PullRequest{}
	for _, v := range prMap {
		prs = append(prs, v)
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
	if s.byReviewer == nil {
		s.byReviewer = make(map[string]map[int]*github.PullRequest)
	}

	savePR := func(name string) {
		if s.byReviewer[name] == nil {
			s.byReviewer[name] = make(map[int]*github.PullRequest)
		}
		s.byReviewer[name][*pr.Number] = pr
	}

	issueComments, err := s.Query.GetIssueComments(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch issue comments on %s", *pr.HTMLURL))
	}
	for _, comment := range issueComments {
		name := getName(comment.User)
		s.ReviewCounts[name] += 1
		savePR(name)
	}

	prComments, err := s.Query.GetPRComments(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch PR comments on %s", *pr.HTMLURL))
	}
	for _, comment := range prComments {
		name := getName(comment.User)
		s.ReviewCounts[name] += 1
		savePR(name)
	}

	reviews, err := s.Query.GetReviews(pr)
	if err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("could not fetch reviews on %s", *pr.HTMLURL))
	}
	for _, review := range reviews {
		name := getName(review.User)
		s.ReviewCounts[name] += 1
		savePR(name)
	}

	return nil
}
