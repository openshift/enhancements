package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const longLine string = "The OVN-Kubernetes network type uses [OVN](https://www.ovn.org) to implement node overlay networks for Kubernetes. When OVN-Kubernetes is used as the network type for an Openshift cluster, OVN ACLs are the used to implement Kubernetes  network policies ('NetworkPolicy' resources).  ACL's can either allow or deny traffic by matching on packets with specific rules. Built into the OVN ACL feature is"

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
		{
			Scenario: "Long line",
			Input:    longLine,
			Expected: `  > The OVN-Kubernetes network type uses [OVN](https://www.ovn.org) to implement node overlay networks for Kubernetes. When OVN-Kubernetes is used as the network type for an Openshift cluster, OVN ACLs are the used to implement Kubernetes  network policies ('NetworkPolicy' resources).  ACL's can either allow or deny traffic by matching on packets with specific rules. Built into the OVN ACL feature
  > is`,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := formatDescription(tc.Input, descriptionIndent)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}

func TestFormatLabels(t *testing.T) {

	for _, tc := range []struct {
		Scenario string
		Input    []string
		Expected string
	}{
		{
			Scenario: "None",
			Input:    []string{},
			Expected: "",
		},
		{
			Scenario: "One",
			Input:    []string{"label1"},
			Expected: descriptionIndent + "`label1`",
		},
		{
			Scenario: "Two",
			Input:    []string{"label1", "label2"},
			Expected: descriptionIndent + "`label1, label2`",
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := formatLabels(tc.Input, descriptionIndent)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}

func TestFindSplit(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Input    string
		Start    int
		Expected int
	}{
		{
			Scenario: "no spaces",
			Input:    "this-string-has-no-spaces",
			Expected: 0,
		},
		{
			Scenario: "one space",
			Input:    "one space",
			Expected: 3,
		},
		{
			Scenario: "multiple spaces",
			Input:    "this string has no spaces",
			Expected: 18,
		},
		{
			Scenario: "past end",
			Input:    "short",
			Start:    100,
			Expected: 0,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			start := tc.Start
			if start == 0 {
				start = len(tc.Input) - 1
			}
			actual := findSplit(tc.Input, start)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}

func TestWrapLine(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Input    string
		Len      int
		Expected []string
	}{
		{
			Scenario: "no spaces",
			Input:    "this-string-has-no-spaces",
			Len:      5,
			Expected: []string{"this-string-has-no-spaces"},
		},
		{
			Scenario: "one space",
			Input:    "one space",
			Len:      5,
			Expected: []string{"one", "space"},
		},
		{
			Scenario: "long line",
			Input:    longLine,
			Len:      maxLineLength - 4,
			Expected: []string{"The OVN-Kubernetes network type uses [OVN](https://www.ovn.org) to implement node overlay networks for Kubernetes. When OVN-Kubernetes is used as the network type for an Openshift cluster, OVN ACLs are the used to implement Kubernetes  network policies ('NetworkPolicy' resources).  ACL's can either allow or deny traffic by matching on packets with specific rules. Built into the OVN ACL feature", "is"},
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := wrapLine(tc.Input, tc.Len)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}
