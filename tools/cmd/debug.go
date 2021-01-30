package cmd

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/openshift/enhancements/tools/enhancements"
)

func newDebugCommand() *cobra.Command {
	var prID int

	cmd := &cobra.Command{
		Use:   "debug",
		Short: "Dump debug details for a pull request",
		RunE: func(cmd *cobra.Command, args []string) error {
			if prID <= 0 {
				return fmt.Errorf("Please specify a valid --pr ID")
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

	cmd.Flags().IntVar(&prID, "pr", 0, "which pull request")

	return cmd
}

func init() {
	rootCmd.AddCommand(newDebugCommand())
}
