package cmd

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/enhancements"
)

func newShowPRCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "show-pr",
		Short:     "Dump details for a pull request",
		ValidArgs: []string{"pull-request-id"},
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("please specify one valid pull request ID")
			}
			if _, err := strconv.Atoi(args[0]); err != nil {
				return errors.Wrap(err,
					fmt.Sprintf("pull request ID %q must be an integer", args[0]))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			prID, err := strconv.Atoi(args[0])
			if err != nil {
				return errors.Wrap(err,
					fmt.Sprintf("could not interpret pull request ID %q as a number", args[0]))
			}
			group, isEnhancement, err := enhancements.GetGroup(prID)
			if err != nil {
				return errors.Wrap(err, "Could not determine group for PR")
			}

			fmt.Printf("Group: %s\n", group)
			fmt.Printf("Enhancement: %v\n", isEnhancement)

			return nil
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(newShowPRCommand())
}
