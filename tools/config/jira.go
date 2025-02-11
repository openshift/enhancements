package config

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

type JiraSettings struct {
	Login  string `yaml:"login"`
	Server string `yaml:"server"`
}

func LoadJiraConfigFromFile(filename string) (*JiraSettings, error) {

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	result := JiraSettings{}
	err = yaml.Unmarshal(content, &result)
	if err != nil {
		return nil, err
	}
	if result.Login == "" {
		return nil, fmt.Errorf("No Jira login found in %s", filename)
	}
	return &result, nil
}
