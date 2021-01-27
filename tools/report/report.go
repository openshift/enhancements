package report

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/openshift/enhancements/tools/enhancements"
	"github.com/openshift/enhancements/tools/stats"
)

func formatDescription(text string, indent string) string {
	paras := strings.SplitN(strings.ReplaceAll(text, "\r", ""), "\n\n", -1)

	unwrappedParas := []string{}

	for _, p := range paras {
		unwrapped := strings.ReplaceAll(p, "\n", " ")
		quoted := fmt.Sprintf("%s> %s", indent, unwrapped)
		unwrappedParas = append(unwrappedParas, quoted)
	}

	joinOn := fmt.Sprintf("\n%s>\n", indent)

	return strings.Join(unwrappedParas, joinOn)
}

const descriptionIndent = "  "

func showPRs(name string, prds []*stats.PullRequestDetails, withDescription bool) {
	fmt.Printf("\n### %s Enhancements\n", name)
	if len(prds) == 0 {
		fmt.Printf("\nThere were 0 %s pull requests.\n\n", name)
		return
	}

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

func filterPRDs(prds []*stats.PullRequestDetails, prioritized bool) []*stats.PullRequestDetails {
	results := []*stats.PullRequestDetails{}

	for _, prd := range prds {
		if prd.Prioritized != prioritized {
			continue
		}

		group, _, err := enhancements.GetGroup(*prd.Pull.Number)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: failed to get group of PR %d: %s\n",
				*prd.Pull.Number, err)
			group = "uncategorized"
		}

		// ignore pull requests with only changes in the tools
		// directory or this-week posts
		if group == "tools" || group == "this-week" {
			continue
		}

		results = append(results, prd)
	}
	return results
}

func sortByID(prds []*stats.PullRequestDetails) {
	sort.Slice(prds, func(i, j int) bool {
		return *prds[i].Pull.Number < *prds[j].Pull.Number
	})
}

func sortByActivityCountDesc(prds []*stats.PullRequestDetails) {
	sort.Slice(prds, func(i, j int) bool {
		return prds[i].RecentActivityCount > prds[j].RecentActivityCount
	})
}

func ShowReport(theStats *stats.Stats, daysBack, staleMonths int, full bool) {
	// NOTE: for manual testing
	// n := 260
	// url := "fake://url"
	// theStats.Merged = append(
	// 	theStats.Merged,
	// 	&stats.PullRequestDetails{
	// 		Pull: &github.PullRequest{
	// 			Number:  &n,
	// 			HTMLURL: &url,
	// 			Title:   &url,
	// 		},
	// 	},
	// )

	fmt.Fprintf(os.Stderr, "Processed %d pull requests\n", len(theStats.All))

	sortByID(theStats.Merged)
	sortByID(theStats.Closed)
	sortByID(theStats.New)
	sortByActivityCountDesc(theStats.Active)

	if full {
		sortByID(theStats.Old)
		sortByID(theStats.Idle)
		sortByID(theStats.Stale)
		sortByID(theStats.Revived)
	}

	year, month, day := time.Now().Date()
	fmt.Printf("# This Week in Enhancements - %d-%.2d-%.2d\n", year, month, day)

	fmt.Printf("\n## Enhancements for Release Priorities\n")

	showPRs("Prioritized Merged", filterPRDs(theStats.Merged, true), true)
	showPRs("Prioritized Closed", filterPRDs(theStats.Closed, true), false)
	showPRs("Prioritized New", filterPRDs(theStats.New, true), true)
	showPRs("Prioritized Active", filterPRDs(theStats.Active, true), false)

	if full {
		showPRs(fmt.Sprintf("Prioritized Revived (closed more than %d days ago, but with new comments)", daysBack),
			filterPRDs(theStats.Revived, true), false)
		showPRs(fmt.Sprintf("Prioritized Idle (no comments for at least %d days)", daysBack),
			filterPRDs(theStats.Idle, true), false)
		showPRs(fmt.Sprintf("Prioritized Old (older than %d months, but discussion in last %d days)",
			staleMonths, daysBack), filterPRDs(theStats.Old, true), false)
		showPRs(fmt.Sprintf("Prioritized Stale (older than %d months, not discussed in last %d days)",
			staleMonths, daysBack), filterPRDs(theStats.Stale, true), false)
	}

	fmt.Printf("\n## Other Enhancements\n")

	showPRs("Other Merged", filterPRDs(theStats.Merged, false), true)
	showPRs("Other Closed", filterPRDs(theStats.Closed, false), false)
	showPRs("Other New", filterPRDs(theStats.New, false), true)
	showPRs("Other Active", filterPRDs(theStats.Active, false), false)

	if full {
		showPRs(fmt.Sprintf("Other Revived (closed more than %d days ago, but with new comments)", daysBack),
			filterPRDs(theStats.Revived, false), false)
		showPRs(fmt.Sprintf("Other Idle (no comments for at least %d days)", daysBack),
			filterPRDs(theStats.Idle, false), false)
		showPRs(fmt.Sprintf("Other Old (older than %d months, but discussion in last %d days)",
			staleMonths, daysBack), filterPRDs(theStats.Old, false), false)
		showPRs(fmt.Sprintf("Other Stale (older than %d months, not discussed in last %d days)",
			staleMonths, daysBack), filterPRDs(theStats.Stale, false), false)
	}
}
