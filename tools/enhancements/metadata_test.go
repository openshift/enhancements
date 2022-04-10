package enhancements

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMetaData(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Input    []byte
		Expected *MetaData
	}{
		{
			Scenario: "empty",
			Input:    []byte(""),
			Expected: &MetaData{},
		},
		{
			Scenario: "template",
			Input: []byte(`
---
title: neat-enhancement-idea
authors:
  - "@janedoe"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring. For example, "- @networkguru, for networking aspects"
  - TBD
  - "@alicedoe"
approvers:
  - TBD
  - "@oscardoe"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - TBD
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---
`),
			Expected: &MetaData{
				Title:         "neat-enhancement-idea",
				Authors:       []string{"@janedoe"},
				Reviewers:     []string{"TBD", "@alicedoe"},
				Approvers:     []string{"TBD", "@oscardoe"},
				APIApprovers:  []string{"TBD"},
				TrackingLinks: []string{"TBD"},
			},
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual, _ := NewMetaData(tc.Input)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}
