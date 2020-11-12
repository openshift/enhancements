package enhancements

import "strings"

var (
	titlePrefixes = []string{"wip:", "[wip]", "enhancement:"}
)

func CleanTitle(title string) string {
	for _, prefix := range titlePrefixes {
		if strings.HasPrefix(strings.ToLower(title), prefix) {
			title = strings.TrimLeft(title[len(prefix):], " ")
		}
	}
	return title
}
