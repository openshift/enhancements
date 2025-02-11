/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/openshift/enhancements/tools/util"
	"github.com/spf13/cobra"
)

// pruneOwnersCmd represents the pruneOwners command
var pruneOwnersCmd = &cobra.Command{
	Use:   "prune-owners",
	Short: "Edit the OWNERS file to remove anyone no longer a member of the org",
	RunE: func(cmd *cobra.Command, args []string) error {
		initConfig()

		owners, err := util.ReadOwners()
		if err != nil {
			return err
		}
		fmt.Printf("Checking %d names...\n", len(owners.Approvers))

		keepMembers := []string{}
		client := util.NewGithubClient(configSettings.Github.Token)
		for _, name := range owners.Approvers {
			user, _, err := client.Users.Get(context.Background(), name)
			if err != nil {
				return err
			}
			isMember, _, err := client.Organizations.IsMember(context.Background(), "openshift", name)
			if err != nil {
				return err
			}
			if isMember {
				keepMembers = append(keepMembers, name)
			} else {
				fmt.Printf("%s (%s) is not an organization member\n", *user.Name, name)
			}
		}

		fmt.Printf("Keeping %d names\n", len(keepMembers))
		owners.Approvers = keepMembers
		return owners.Write()
	},
}

func init() {
	rootCmd.AddCommand(pruneOwnersCmd)
}
