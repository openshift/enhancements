package util

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v3"
)

var candidateFilenames = []string{"OWNERS", "../OWNERS"}

type Owners struct {
	Approvers []string `yaml:"approvers"`
	filename  string
}

func readFromFile(filename string) (*Owners, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	owners := &Owners{
		filename: filename,
	}
	err = yaml.Unmarshal(content, owners)
	if err != nil {
		return nil, err
	}
	return owners, nil
}

func ReadOwners() (*Owners, error) {
	for _, filename := range candidateFilenames {
		if _, err := os.Stat(filename); err == nil {
			return readFromFile(filename)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("Did not find OWNERS file at %v", candidateFilenames)
}
