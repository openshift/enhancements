package enhancements

import (
	"fmt"
	"os"
	"os/exec"
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

func stringSliceContains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// GetModifiedFiles tries to determine which files have changed in a
// pull request.
func GetModifiedFiles(pr int) (filenames []string, err error) {
	ref := prRef(pr)

	firstParentOut, err := exec.Command("git", "rev-list", "--first-parent", "^"+ref, "origin/master").Output()
	if err != nil {
		exitError := err.(*exec.ExitError)
		return nil, errors.Wrap(err, fmt.Sprintf("could not get files changed in %s: %s",
			ref, exitError.Stderr))
	}
	firstParentLines := strings.Split(string(firstParentOut), "\n")
	firstParent := firstParentLines[len(firstParentLines)-1]
	if firstParent == "" {
		firstParent = firstParentLines[len(firstParentLines)-2]
	}

	oldestAncestorOut, err := exec.Command("git", "rev-list", firstParent+"^^!").Output()
	if err != nil {
		exitError := err.(*exec.ExitError)
		return nil, errors.Wrap(err, fmt.Sprintf("could not get files changed in %s: %s",
			ref, exitError.Stderr))
	}
	oldestAncestor := strings.TrimSpace(string(oldestAncestorOut))

	out, err := exec.Command("git", "log", "--oneline", "--pretty=", "--name-only",
		oldestAncestor+".."+ref).Output()
	if err != nil {
		exitError := err.(*exec.ExitError)
		return nil, errors.Wrap(err, fmt.Sprintf("could not get files changed in %s: %s",
			ref, exitError.Stderr))
	}
	for _, name := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" && !stringSliceContains(filenames, trimmed) {
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

// DeriveGroup returns the grouping of an enhancement, based
// on the filename. Documents are normally named
// "enhancements/group/title.md" or "enhancements/title.md"
func DeriveGroup(filenames []string) (filename string, isEnhancement bool) {
	// First look for an actual enhancement document...
	// FIXME: What if we find more than one?
	for _, name := range filenames {
		if strings.HasPrefix(name, "enhancements/") {
			parts := strings.Split(name, "/")
			if len(parts) == 3 {
				return parts[1], true
			}
			return "general", true
		}
	}
	// Now look for some known housekeeping files...
	for _, name := range filenames {
		if strings.HasPrefix(name, "OWNERS") {
			return "housekeeping", false
		}
		if strings.HasPrefix(name, ".markdownlint-cli2.yaml") {
			return "tools", false
		}
		if strings.HasPrefix(name, "hack/") {
			return "tools", false
		}
	}
	// If there was no enhancement, take the root directory of the
	// first file that has a directory.
	for _, name := range filenames {
		if strings.Contains(name, "/") {
			parts := strings.Split(name, "/")
			return parts[0], false
		}
	}
	// If there was no directory, assume a "general" change like
	return "general", false
}

// GetSummary reads the files being changed in the pull request to
// find the summary block.
func GetSummary(pr int) (summary string, err error) {
	filenames, err := GetModifiedFiles(pr)
	if err != nil {
		return "", errors.Wrap(err, "could not determine the list of modified files")
	}
	enhancementFiles := []string{}
	for _, name := range filenames {
		if !strings.HasPrefix(name, "enhancements/") {
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		enhancementFiles = append(enhancementFiles, name)
	}
	if len(enhancementFiles) != 1 {
		return "", fmt.Errorf("expected 1 modified file, found %v", enhancementFiles)
	}
	summary = fmt.Sprintf("(no '## Summary' section found in %s)", enhancementFiles[0])
	fileRef := fmt.Sprintf("%s:%s", prRef(pr), enhancementFiles[0])
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
