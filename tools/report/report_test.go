package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatDescription(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Input    string
		Expected string
	}{
		{
			Scenario: "One line",
			Input:    "One line",
			Expected: "  > One line",
		},
		{
			Scenario: "Two lines one para",
			Input: `First line
Second line
`,
			Expected: `  > First line
  > Second line`,
		},
		{
			Scenario: "Multiple para",
			Input: `First line
Second line

Third line
Fourth line
`,
			Expected: `  > First line
  > Second line
  > 
  > Third line
  > Fourth line`,
		},
		{
			Scenario: "Bullet list",
			Input: `First line
Second line

- list a
- list b

Third line
Fourth line
`,
			Expected: `  > First line
  > Second line
  > 
  > - list a
  > - list b
  > 
  > Third line
  > Fourth line`,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := formatDescription(tc.Input, descriptionIndent)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}
