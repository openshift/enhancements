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

func TestValidateMetaData(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Input    MetaData
		Expected []string
	}{
		{
			Scenario: "empty",
			Input:    MetaData{},
			Expected: []string{
				"tracking-link must contain at least one valid URL",
				"authors must have at least one valid github id",
				"reviewers must have at least one valid github id",
				"approvers must have at least one valid github id",
				"api-approvers must have at least one valid github id",
			},
		},
		{
			Scenario: "reviewers-tbd",
			Input: MetaData{
				Reviewers: []string{"TBD"},
			},
			Expected: []string{
				"tracking-link must contain at least one valid URL",
				"authors must have at least one valid github id",
				"reviewers must not have TBD as value",
				"reviewers must have at least one valid github id",
				"approvers must have at least one valid github id",
				"api-approvers must have at least one valid github id",
			},
		},
		{
			Scenario: "approvers-tbd",
			Input: MetaData{
				Approvers: []string{"TBD"},
			},
			Expected: []string{
				"tracking-link must contain at least one valid URL",
				"authors must have at least one valid github id",
				"reviewers must have at least one valid github id",
				"approvers must not have TBD as value",
				"approvers must have at least one valid github id",
				"api-approvers must have at least one valid github id",
			},
		},
		{
			Scenario: "reviewers-empty-string",
			Input: MetaData{
				Reviewers: []string{""},
			},
			Expected: []string{
				"tracking-link must contain at least one valid URL",
				"authors must have at least one valid github id",
				"reviewers must not be an empty string",
				"reviewers must have at least one valid github id",
				"approvers must have at least one valid github id",
				"api-approvers must have at least one valid github id",
			},
		},
		{
			Scenario: "approvers-empty-string",
			Input: MetaData{
				Approvers: []string{""},
			},
			Expected: []string{
				"tracking-link must contain at least one valid URL",
				"authors must have at least one valid github id",
				"reviewers must have at least one valid github id",
				"approvers must not be an empty string",
				"approvers must have at least one valid github id",
				"api-approvers must have at least one valid github id",
			},
		},
		{
			Scenario: "tracking-link-tbd",
			Input: MetaData{
				TrackingLinks: []string{"TBD"},
			},
			Expected: []string{
				"'TBD' is not a valid value for tracking-link",
				"tracking-link must contain at least one valid URL",
				"authors must have at least one valid github id",
				"reviewers must have at least one valid github id",
				"approvers must have at least one valid github id",
				"api-approvers must have at least one valid github id",
			},
		},
		{
			Scenario: "tracking-link-parse",
			Input: MetaData{
				TrackingLinks: []string{"https//blah/blah"},
			},
			Expected: []string{
				"could not parse tracking-link \"https//blah/blah\"",
				"tracking-link must contain at least one valid URL",
				"authors must have at least one valid github id",
				"reviewers must have at least one valid github id",
				"approvers must have at least one valid github id",
				"api-approvers must have at least one valid github id",
			},
		},
		{
			Scenario: "tracking-link-emtpy",
			Input: MetaData{
				TrackingLinks: []string{""},
			},
			Expected: []string{
				"tracking-link must not be empty",
				"tracking-link must contain at least one valid URL",
				"authors must have at least one valid github id",
				"reviewers must have at least one valid github id",
				"approvers must have at least one valid github id",
				"api-approvers must have at least one valid github id",
			},
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := tc.Input.Validate()
			assert.Equal(t, tc.Expected, actual)
		})
	}
}
