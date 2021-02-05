package report

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/openshift/enhancements/tools/enhancements"
	"github.com/openshift/enhancements/tools/stats"
)

// Indent the summary and prefix it each line to make it format as a
// block quote.
func formatDescription(text string, indent string) string {
	withoutCarriageReturns := strings.ReplaceAll(text, "\r", "")
	withoutWhitespace := strings.TrimSpace(withoutCarriageReturns)
	lines := strings.SplitN(withoutWhitespace, "\n", -1)

	indentedLines := []string{}
	prefix := fmt.Sprintf("%s> ", indent)

	for _, line := range lines {
		indentedLines = append(indentedLines, strings.TrimRight(prefix+line, " "))
	}

	return strings.Join(indentedLines, "\n")
}

const descriptionIndent = "  "

// ShowPRs prints a section of the report by formatting the
// PullRequestDetails as a list.
func ShowPRs(name string, prds []*stats.PullRequestDetails, withDescription bool) {
	if len(prds) == 0 {
		return
	}
	fmt.Printf("\n### %s Enhancements\n", name)

	fmt.Printf("\n*&lt;PR ID&gt;: (activity this week / total activity) summary*\n")

	if len(prds) == 1 {
		fmt.Printf("\nThere was 1 %s pull request:\n\n", name)
	} else {
		fmt.Printf("\nThere were %d %s pull requests:\n\n", len(prds), name)
	}
	for _, prd := range prds {
		author := ""
		if prd.Pull.User != nil {
			for _, option := range []*string{prd.Pull.User.Name, prd.Pull.User.Login} {
				if option != nil {
					author = *option
					break
				}
			}
		}

		group, isEnhancement, err := enhancements.GetGroup(*prd.Pull.Number)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: failed to get group of PR %d: %s\n",
				*prd.Pull.Number, err)
			group = "uncategorized"
		}

		groupPrefix := fmt.Sprintf("%s: ", group)
		if strings.HasPrefix(*prd.Pull.Title, groupPrefix) {
			// avoid redundant group prefix
			groupPrefix = ""
		}

		title := enhancements.CleanTitle(*prd.Pull.Title)

		fmt.Printf("- [%d](%s): (%d/%d) %s%s (%s)\n",
			*prd.Pull.Number,
			*prd.Pull.HTMLURL,
			prd.RecentActivityCount,
			prd.AllActivityCount,
			groupPrefix,
			title,
			author,
		)
		if withDescription {
			var summary string
			if isEnhancement {
				summary, err = enhancements.GetSummary(*prd.Pull.Number)
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: failed to get summary of PR %d: %s\n",
						*prd.Pull.Number, err)
				}
			} else {
				if prd.Pull.Body != nil {
					summary = *prd.Pull.Body
				}
			}
			if summary != "" {
				fmt.Printf("\n%s\n\n", formatDescription(summary, descriptionIndent))
			}
		}
	}
}

// SortByID reorders the slice of PullRequestDetails by the pull
// request number in ascending order.
func SortByID(prds []*stats.PullRequestDetails) {
	sort.Slice(prds, func(i, j int) bool {
		return *prds[i].Pull.Number < *prds[j].Pull.Number
	})
}

// SortByActivityCountDesc reorders the slice of PullRequestDetails by
// the recent activity count in descending order.
func SortByActivityCountDesc(prds []*stats.PullRequestDetails) {
	sort.Slice(prds, func(i, j int) bool {
		return prds[i].RecentActivityCount > prds[j].RecentActivityCount
	})
}
