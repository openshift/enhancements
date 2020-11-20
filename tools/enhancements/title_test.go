package enhancements

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanTitle(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Input    string
		Expected string
	}{
		{
			Scenario: "empty-string",
			Input:    "",
			Expected: "",
		},
		{
			Scenario: "no-changes",
			Input:    "this is a perfectly nice title",
			Expected: "this is a perfectly nice title",
		},
		{
			Scenario: "enhancement-prefix",
			Input:    "enhancement: remove the prefix",
			Expected: "remove the prefix",
		},
		{
			Scenario: "enhancement-prefix-caps",
			Input:    "Enhancement: Remove the prefix",
			Expected: "Remove the prefix",
		},
		{
			Scenario: "WIP:",
			Input:    "WIP: Remove the prefix",
			Expected: "Remove the prefix",
		},
		{
			Scenario: "[wip]",
			Input:    "[WIP] Remove the prefix",
			Expected: "Remove the prefix",
		},
		{
			Scenario: "combo",
			Input:    "[WIP] enhancement: Remove the prefix",
			Expected: "Remove the prefix",
		},
		{
			Scenario: "combo-no-whitespace",
			Input:    "[WIP]enhancement:Remove the prefix",
			Expected: "Remove the prefix",
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := CleanTitle(tc.Input)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}
