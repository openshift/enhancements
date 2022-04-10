package enhancements

import (
	"fmt"
	"net/url"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type MetaData struct {
	Title         string   `yaml:"title"`
	Authors       []string `yaml:"authors"`
	Reviewers     []string `yaml:"reviewers"`
	Approvers     []string `yaml:"approvers"`
	APIApprovers  []string `yaml:"api-approvers"`
	TrackingLinks []string `yaml:"tracking-link"`
}

// NewMetaData returns the metadata block from the top of the text
// passed in.
func NewMetaData(content []byte) (*MetaData, error) {
	result := MetaData{}

	err := yaml.Unmarshal(content, &result)
	if err != nil {
		return nil, errors.Wrap(err, "could not extract meta data from header")
	}

	return &result, nil
}

// GetMetaData returns the metadata from the top of the primary
// enhancement file for a pull request.
func (s *Summarizer) GetMetaData(pr int) (*MetaData, error) {
	enhancementFile, err := s.GetEnhancementFilename(pr)
	if err != nil {
		return nil, errors.Wrap(err, "could not determine the enhancement file name")
	}
	fileRef := fmt.Sprintf("%s:%s", s.prRef(pr), enhancementFile)
	content, err := getFileContents(fileRef)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not get content of %s", fileRef))
	}
	return NewMetaData(content)
}

// Validate returns a list of issues found with the metadata
func (m *MetaData) Validate() []string {
	results := []string{}

	reportError := func(msg string) {
		results = append(results, msg)
	}

	// Must have one valid tracking link and no TBD
	foundLink := false
	for _, trackingLink := range m.TrackingLinks {
		if trackingLink == "TBD" {
			reportError("'TBD' is not a valid value for tracking-link")
			continue
		}
		if trackingLink == "" {
			reportError("tracking-link must not be empty")
			continue
		}
		if u, err := url.Parse(trackingLink); err != nil {
			reportError(fmt.Sprintf("could not parse tracking-link %q: %s",
				trackingLink, err,
			))
			continue
		} else {
			if u.Scheme == "" {
				reportError(fmt.Sprintf("could not parse tracking-link %q",
					trackingLink,
				))
				continue
			}
		}
		foundLink = true
	}
	if !foundLink {
		reportError("tracking-link must contain at least one valid URL")
	}

	// No TBD in required people fields
	for _, field := range []struct {
		Name string
		Data []string
	}{
		{
			Name: "authors",
			Data: m.Authors,
		},
		{
			Name: "reviewers",
			Data: m.Reviewers,
		},
		{
			Name: "approvers",
			Data: m.Approvers,
		},
		{
			Name: "api-approvers",
			Data: m.APIApprovers,
		},
	} {
		valid := 0
		for _, value := range field.Data {
			if value == "TBD" {
				reportError(fmt.Sprintf("%s must not have TBD as value", field.Name))
				continue
			}
			if value == "" {
				reportError(fmt.Sprintf("%s must not be an empty string", field.Name))
				continue
			}
			valid++
		}
		if valid < 1 {
			reportError(fmt.Sprintf("%s must have at least one valid github id", field.Name))
		}
	}

	return results
}
