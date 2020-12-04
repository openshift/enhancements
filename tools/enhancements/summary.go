package enhancements

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// UpdateGitRepo refreshes the local repository so all of the pull
// requests are available.
func UpdateGitRepo() error {
	if err := ensureFetchSettings(); err != nil {
		return errors.Wrap(err, "could not configure fetch settings")
	}
	fmt.Fprintf(os.Stderr, "updating local git repository...")
	_, err := exec.Command("git", "remote", "update").Output()
	if err != nil {
		return errors.Wrap(err, "fetching config failed")
	}
	fmt.Fprintf(os.Stderr, "\n")
	return nil
}

const desiredRefSetting = "+refs/pull/*/head:refs/remotes/origin/pr/*"

func prRef(pr int) string {
	return fmt.Sprintf("origin/pr/%d", pr)
}

func ensureFetchSettings() error {
	out, err := exec.Command("git", "config", "--get-all", "--local",
		"remote.origin.fetch").Output()
	if err != nil {
		return errors.Wrap(err, "fetching config failed")
	}

	if strings.Contains(string(out), desiredRefSetting) {
		return nil
	}

	fmt.Fprintf(os.Stderr, "adding %s to remote.origin.fetch\n", desiredRefSetting)
	_, err = exec.Command("git", "config", "--add", "--local",
		"remote.origin.fetch", desiredRefSetting).Output()
	if err != nil {
		return errors.Wrap(err, "failed to update git config")
	}
	return nil
}

// getModifiedFiles tries to determine which files have changed in a
// pull request.
func getModifiedFiles(pr int) (filenames []string, err error) {
	ref := prRef(pr)
	out, err := exec.Command("git", "show", "--name-only", ref, "--format=").Output()
	if err != nil {
		exitError := err.(*exec.ExitError)
		return nil, errors.Wrap(err, fmt.Sprintf("could not get files changed in %s: %s",
			ref, exitError.Stderr))
	}
	for _, name := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			// Ignore anything that doesn't look like a markdown file.
			if filepath.Ext(trimmed) != ".md" {
				continue
			}
			filenames = append(filenames, trimmed)
		}
	}
	return filenames, nil
}

func getFileContents(ref string) ([]byte, error) {
	content, err := exec.Command("git", "show", ref).Output()
	if err != nil {
		exitError := err.(*exec.ExitError)
		return nil, errors.Wrap(err, fmt.Sprintf("git show failed: %s", exitError.Stderr))
	}
	return content, nil
}

// extractSummary looks for a block of text starting after the line
// "## Summary" and ending before the next header line starting with
// "##"
func extractSummary(body string) string {
	var b strings.Builder
	inSummary := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## Summary") {
			inSummary = true
			continue
		}
		if !inSummary {
			continue
		}
		if strings.HasPrefix(line, "##") {
			break
		}
		fmt.Fprintf(&b, "%s\n", line)
	}
	return b.String()
}

// GetGroup returns the grouping of the enhancement, based
// on the filename. Documents are normally named
// "enhancements/group/title.md" or "enhancements/title.md"
func GetGroup(pr int) (filename string, isEnhancement bool, err error) {
	filenames, err := getModifiedFiles(pr)
	if err != nil {
		return "", false, errors.Wrap(err, "could not determine the list of modified files")
	}
	// First look for an actual enhancement document...
	// FIXME: What if we find more than one?
	for _, name := range filenames {
		if strings.HasPrefix(name, "enhancements/") {
			parts := strings.Split(name, "/")
			if len(parts) == 3 {
				return parts[1], true, nil
			}
			return "general", true, nil
		}
	}
	// If there was no enhancement, take the root directory of the
	// first file that has a directory.
	for _, name := range filenames {
		if strings.Contains(name, "/") {
			parts := strings.Split(name, "/")
			return parts[0], false, nil
		}
	}
	// If there was no directory, assume a "general" change like
	// OWNERS file.
	return "general", false, nil
}

// GetSummary reads the files being changed in the pull request to
// find the summary block.
func GetSummary(pr int) (summary string, err error) {
	filenames, err := getModifiedFiles(pr)
	if err != nil {
		return "", errors.Wrap(err, "could not determine the list of modified files")
	}
	if len(filenames) != 1 {
		return "", fmt.Errorf("expected 1 modified file, found %v", filenames)
	}
	summary = fmt.Sprintf("(no '## Summary' section found in %s)", filenames[0])
	fileRef := fmt.Sprintf("%s:%s", prRef(pr), filenames[0])
	content, err := getFileContents(fileRef)
	if err != nil {
		return summary, errors.Wrap(err, fmt.Sprintf("could not get content of %s", fileRef))
	}
	candidate := strings.TrimSpace(extractSummary(string(content)))
	if candidate != "" {
		summary = candidate
	}
	return summary, nil
}
