/*
Copyright © 2022 Doug Hellmann <dhellmann@redhat.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/openshift/enhancements/tools/enhancements"

	"github.com/spf13/cobra"
)

// metadataLintCmd represents the metadataLint command
var metadataLintCmd = &cobra.Command{
	Use:       "metadata-lint",
	Short:     "Check the metadata of the enhancement files given as arguments",
	ValidArgs: []string{"enhancement-filename"},
	Run: func(cmd *cobra.Command, args []string) {
		errorCount := 0
		for _, filename := range args {
			reportError := func(msg string) {
				fmt.Fprintf(os.Stderr, "%s: %s\n", filename, msg)
				errorCount++
			}
			content, err := ioutil.ReadFile(filename)
			if err != nil {
				reportError(fmt.Sprintf("%s", err))
				continue
			}
			metadata, err := enhancements.NewMetaData(content)
			if err != nil {
				reportError(fmt.Sprintf("%s", err))
				continue
			}

			// Doc title needs to match filename, minus the ".md" extension
			fileBase := filepath.Base(filename)
			fileBase = fileBase[0 : len(fileBase)-3]
			if fileBase != metadata.Title {
				reportError(fmt.Sprintf("the title %s and the file base name %s must match",
					metadata.Title, fileBase,
				))
			}

			for _, msg := range metadata.Validate() {
				reportError(msg)
			}
		}

		if errorCount > 0 {
			fmt.Fprintf(os.Stderr, "%d errors", errorCount)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(metadataLintCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// metadataLintCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// metadataLintCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
