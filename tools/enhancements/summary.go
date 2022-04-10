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

// findOpenShiftRemote looks through the remotes for the current git
// repository to find one with "openshift/enhancements" in the URL
func findOpenShiftRemote() (string, error) {
	rawRemotes, err := exec.Command("git", "remote").Output()
	if err != nil {
		return "", errors.Wrap(err, "fetching remotes failed")
	}

	remotes := strings.Split(string(rawRemotes), "\n")
	for _, r := range remotes {
		rawURL, err := exec.Command("git", "remote", "get-url", r).Output()
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("fetching url for remote %q failed", r))
		}
		if strings.Contains(string(rawURL), "openshift/enhancements") {
			return r, nil
		}
	}

	return "", fmt.Errorf("unable to find remote for repo openshift/enhancements")
}

func ensureFetchSettings() error {
	upstream, err := findOpenShiftRemote()
	if err != nil {
		return errors.Wrap(err, "unable to configure fetch settings")
	}

	configName := fmt.Sprintf("remote.%s.fetch", upstream)
	out, err := exec.Command("git", "config", "--get-all", "--local",
		configName).Output()
	if err != nil {
		return errors.Wrap(err, "fetching config failed")
	}

	desiredRefSetting := fmt.Sprintf("+refs/pull/*/head:refs/remotes/%s/pr/*", upstream)

	if strings.Contains(string(out), desiredRefSetting) {
		return nil
	}

	fmt.Fprintf(os.Stderr, "adding %s to %s\n", desiredRefSetting, configName)
	_, err = exec.Command("git", "config", "--add", "--local",
		configName, desiredRefSetting).Output()
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

type Summarizer struct {
	upstreamRemote string
}

func NewSummarizer() (*Summarizer, error) {
	upstream, err := findOpenShiftRemote()
	if err != nil {
		return nil, errors.Wrap(err, "could not construct PR summarizer")
	}
	s := Summarizer{
		upstreamRemote: upstream,
	}
	return &s, nil
}

func (s *Summarizer) prRef(pr int) string {
	return fmt.Sprintf("%s/pr/%d", s.upstreamRemote, pr)
}

type ModifiedFile struct {
	Name string
	Mode string
}

// GetModifiedFiles tries to determine which files have changed in a
// pull request.
func (s *Summarizer) GetModifiedFiles(pr int) (files []ModifiedFile, err error) {
	ref := s.prRef(pr)

	// Find the list of files added or modified in the PR by starting
	// from the oldest ancestor commit common with the origin/master
	// branch and diffing the PR branch. Equivalent to:
	//
	// oldest_ancestor=$(git rev-list $(git rev-list --first-parent ^"$PR_BRANCH" "origin/master" | tail -n1)^^!)
	// git diff --name-status ${oldest_ancestor}..${PR_BRANCH}

	firstParentOut, err := exec.Command("git", "rev-list", "--first-parent", "^"+ref, "origin/master").Output()
	if err != nil {
		exitError := err.(*exec.ExitError)
		return nil, errors.Wrap(err, fmt.Sprintf("could not get files changed in %s: %s",
			ref, exitError.Stderr))
	}
	firstParentLines := strings.Split(string(firstParentOut), "\n")
	firstParent := firstParentLines[len(firstParentLines)-1]
	if firstParent == "" {
		if len(firstParentLines) > 1 {
			firstParent = firstParentLines[len(firstParentLines)-2]
		}
	}

	modifiedFiles := []ModifiedFile{}

	if firstParent != "" {
		oldestAncestorOut, err := exec.Command("git", "rev-list", firstParent+"^^!").Output()
		if err != nil {
			exitError := err.(*exec.ExitError)
			return nil, errors.Wrap(err, fmt.Sprintf("could not get files changed in %s: %s",
				ref, exitError.Stderr))
		}
		oldestAncestor := strings.TrimSpace(string(oldestAncestorOut))

		out, err := exec.Command("git", "diff", "--name-status", oldestAncestor+".."+ref).Output()
		if err != nil {
			exitError := err.(*exec.ExitError)
			return nil, errors.Wrap(err, fmt.Sprintf("could not get files changed in %s: %s",
				ref, exitError.Stderr))
		}
		for _, line := range strings.Split(string(out), "\n") {
			if line == "" {
				continue
			}
			// The diff output shows the operation (mode) and filename:
			// A       name-of-new-file
			// C       name-of-changed-file
			mode := line[0:1]
			trimmed := strings.TrimSpace(line[1:])
			if trimmed != "" {
				modifiedFiles = append(modifiedFiles, ModifiedFile{Name: trimmed, Mode: mode})
			}
		}
	} else {
		// Resort to using `git show` to find the changed files
		out, err := exec.Command("git", "show", "--pretty=", "--name-only", ref).Output()
		if err != nil {
			exitError := err.(*exec.ExitError)
			return nil, errors.Wrap(err, fmt.Sprintf("could not get files changed in %s: %s",
				ref, exitError.Stderr))
		}
		for _, name := range strings.Split(string(out), "\n") {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				modifiedFiles = append(modifiedFiles, ModifiedFile{Name: trimmed, Mode: "?"})
			}
		}
	}

	return modifiedFiles, nil
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
func DeriveGroup(files []ModifiedFile) (filename string, isEnhancement bool) {
	// First look for an actual enhancement document...
	// FIXME: What if we find more than one?
	for _, f := range files {
		if strings.HasPrefix(f.Name, "enhancements/") {
			parts := strings.Split(f.Name, "/")
			if len(parts) == 3 {
				return parts[1], true
			}
			return "general", true
		}
	}
	// Now look for some known housekeeping files...
	for _, f := range files {
		if strings.HasPrefix(f.Name, "OWNERS") {
			return "housekeeping", false
		}
		if strings.HasPrefix(f.Name, ".markdownlint-cli2.yaml") {
			return "tools", false
		}
		if strings.HasPrefix(f.Name, "hack/") {
			return "tools", false
		}
	}
	// If there was no enhancement, take the root directory of the
	// first file that has a directory.
	for _, f := range files {
		if strings.Contains(f.Name, "/") {
			parts := strings.Split(f.Name, "/")
			return parts[0], false
		}
	}
	// If there was no directory, assume a "general" change like
	return "general", false
}

// getEnhancementFilenames looks for modified enhancement files in the
// pull request.
func (s *Summarizer) getEnhancementFilenames(pr int) ([]string, error) {
	files, err := s.GetModifiedFiles(pr)
	if err != nil {
		return nil, errors.Wrap(err, "could not determine the list of modified files")
	}
	enhancementFiles := []string{}
	for _, f := range files {
		if !strings.HasPrefix(f.Name, "enhancements/") {
			continue
		}
		if !strings.HasSuffix(f.Name, ".md") {
			continue
		}
		enhancementFiles = append(enhancementFiles, f.Name)
	}
	return enhancementFiles, nil
}

// GetEnhancementFilename returns the primary enhancement file in the PR.
func (s *Summarizer) GetEnhancementFilename(pr int) (string, error) {
	enhancementFiles, err := s.getEnhancementFilenames(pr)
	if err != nil {
		return "", errors.Wrap(err, "could not determine the list of enhancement files")
	}
	if len(enhancementFiles) != 1 {
		return "", fmt.Errorf("expected 1 modified file, found %v", enhancementFiles)
	}
	return enhancementFiles[0], nil
}

// GetSummary reads the files being changed in the pull request to
// find the summary block.
func (s *Summarizer) GetSummary(pr int) (summary string, err error) {
	enhancementFile, err := s.GetEnhancementFilename(pr)
	if err != nil {
		return "", errors.Wrap(err, "could not determine the enhancement file name")
	}
	summary = fmt.Sprintf("(no '## Summary' section found in %s)", enhancementFile)
	fileRef := fmt.Sprintf("%s:%s", s.prRef(pr), enhancementFile)
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
