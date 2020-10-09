package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"

	"github.com/openshift/enhancements/tools/config"
	"github.com/openshift/enhancements/tools/enhancements"
	"github.com/openshift/enhancements/tools/stats"
)

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func handleError(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	os.Exit(1)
}

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
	fmt.Printf("\n## %s Enhancements\n", name)
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

		// Sometimes we have a superfluous "enhancement:" prefix in
		// the PR title
		title := *prd.Pull.Title
		if strings.HasPrefix(strings.ToLower(title), "enhancement:") {
			title = strings.TrimLeft(title[12:], " ")
		}

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

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		handleError(fmt.Sprintf("Could not get default for config file name: %v", err))
	}
	configFilenameDefault := filepath.Join(configDir, "ocp-enhancements", "config.yml")

	configFilename := flag.String("config", configFilenameDefault,
		"the configuration file name")

	daysBack := flag.Int("days-back", 7, "how many days back to query")
	staleMonths := flag.Int("stale-months", 3,
		"how many months before a pull request is considered stale")
	org := flag.String("org", "openshift", "github organization")
	repo := flag.String("repo", "enhancements", "github repository")
	devMode := flag.Bool("dev", false, "dev mode, stop after first page of PRs")
	full := flag.Bool("full", false, "full report, not just summary")

	flag.Parse()

	if *configFilename == "" {
		handleError(fmt.Sprintf("Please specify the -config file name"))
	}

	if !fileExists(*configFilename) {
		template := config.GetTemplate()
		handleError(fmt.Sprintf("Please create %s containing\n\n%s\n",
			*configFilename,
			string(template),
		))
	}

	settings, err := config.LoadFromFile(*configFilename)
	if err != nil {
		handleError(fmt.Sprintf("Could not load config file %s: %v", *configFilename, err))
	}

	// Set up git before talking to github so we don't use up our API
	// quota and then fail for something that happens locally.
	if err := enhancements.UpdateGitRepo(); err != nil {
		handleError(fmt.Sprintf("Could not update local git repository: %v", err))
	}

	// todo: add flags for days back and stale months
	theStats, err := stats.New(*daysBack, *staleMonths, *org, *repo, *devMode, func() *github.Client {
		ctx := context.Background()
		tokenSource := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: settings.Github.Token},
		)
		oauthClient := oauth2.NewClient(ctx, tokenSource)
		return github.NewClient(oauthClient)
	})
	if err != nil {
		handleError(fmt.Sprintf("Could not create stats: %v", err))
	}

	err = theStats.IteratePullRequests()
	if err != nil {
		handleError(fmt.Sprintf("could not process pull requests: %v", err))
	}

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
	sortByID(theStats.Old)
	sortByID(theStats.Idle)
	sortByID(theStats.Stale)
	sortByID(theStats.Revived)

	year, month, day := time.Now().Date()
	fmt.Printf("# This Week in Enhancements - %d-%d-%d\n", year, month, day)

	showPRs("Merged", theStats.Merged, true)
	showPRs("Closed", theStats.Closed, false)
	showPRs("New", theStats.New, true)
	showPRs("Active", theStats.Active, false)

	if *full {
		showPRs(fmt.Sprintf("Revived (closed more than %d days ago, but with new comments)",
			*daysBack), theStats.Revived, false)
		showPRs(fmt.Sprintf("Idle (no comments for at least %d days)", *daysBack), theStats.Idle, false)
		showPRs(fmt.Sprintf("Old (older than %d months, but discussion in last %d days)",
			*staleMonths, *daysBack), theStats.Old, false)
		showPRs(fmt.Sprintf("Stale (older than %d months, not discussed in last %d days)",
			*staleMonths, *daysBack), theStats.Stale, false)
	}
}
