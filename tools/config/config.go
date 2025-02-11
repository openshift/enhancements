package config

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

// GithubSettings includes the details needed to connect to the GitHub
// API
type GithubSettings struct {
	Token string `yaml:"token"`
}

// ReviewerSettings includes the details needed to run the reviewer command.
type ReviewerSettings struct {
	Ignore []string `yaml:"ignore"`
}

// Settings includes all of the application settinggs
type Settings struct {
	Github    GithubSettings   `yaml:"github"`
	Reviewers ReviewerSettings `yaml:"reviewers"`
}

// LoadFromFile reads the named file and returns the Settings
// contained within.
func LoadFromFile(filename string) (*Settings, error) {

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	result := Settings{}
	err = yaml.Unmarshal(content, &result)
	if err != nil {
		return nil, err
	}

	if result.Github.Token == "" {
		return nil, fmt.Errorf("No github.token found in %s", filename)
	}

	return &result, nil
}

// GetTemplate returns the body of an empty configuration file to use
// as a template for initializing the file.
func GetTemplate() string {
	template, _ := yaml.Marshal(Settings{})
	return string(template)
}
