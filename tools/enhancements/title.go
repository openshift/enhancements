package enhancements

import "strings"

func CleanTitle(title string) string {
	// Sometimes we have a superfluous "enhancement:" prefix in
	// the PR title
	if strings.HasPrefix(strings.ToLower(title), "enhancement:") {
		title = strings.TrimLeft(title[12:], " ")
	}
	return title
}
