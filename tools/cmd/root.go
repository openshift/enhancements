package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/config"
	"github.com/openshift/enhancements/tools/enhancements"
)

var (
	configFilename    string
	orgName, repoName string
	devMode           bool

	configSettings *config.Settings

	rootCmd = &cobra.Command{
		Use:   "enhancements",
		Short: "Tools for analyzing the openshift/enhancements repo",
	}
)

func init() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		handleError(fmt.Sprintf("Could not get default for config file name: %v", err))
	}

	defaultConfigFilename := filepath.Join(configDir, "ocp-enhancements", "config.yml")

	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVar(&devMode, "dev", false, "dev mode, stop after first page of PRs")
	rootCmd.PersistentFlags().StringVar(&configFilename, "config", defaultConfigFilename, "config file")
	rootCmd.PersistentFlags().StringVar(&orgName, "org", "openshift", "github organization")
	rootCmd.PersistentFlags().StringVar(&repoName, "repo", "enhancements", "github repository")
}

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func handleError(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	os.Exit(1)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		handleError(err.Error())
	}
}

func initConfig() {
	if configFilename == "" {
		handleError(fmt.Sprintf("Please specify the --config file name"))
	}

	if !fileExists(configFilename) {
		template := config.GetTemplate()
		handleError(fmt.Sprintf("Please create %s containing\n\n%s\n",
			configFilename,
			string(template),
		))
	}

	settings, err := config.LoadFromFile(configFilename)
	if err != nil {
		handleError(fmt.Sprintf("Could not load config file %s: %v", configFilename, err))
	}
	configSettings = settings

	// quota and then fail for something that happens locally.
	if err := enhancements.UpdateGitRepo(); err != nil {
		handleError(fmt.Sprintf("Could not update local git repository: %v", err))
	}
}
