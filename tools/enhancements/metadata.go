package enhancements

import (
	"fmt"

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
