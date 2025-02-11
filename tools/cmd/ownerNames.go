/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/openshift/enhancements/tools/util"
	"github.com/spf13/cobra"
)

// ownerNamesCmd represents the ownerNames command
var ownerNamesCmd = &cobra.Command{
	Use:   "owner-names",
	Short: "Show the names for accounts in the owners list",
	Long:  `Look up real names for owners in the owners file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initConfig()

		owners, err := util.ReadOwners()
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "looking up names...")
		client := util.NewGithubClient(configSettings.Github.Token)
		fullNames := []string{}
		for _, name := range owners.Approvers {
			user, _, err := client.Users.Get(context.Background(), name)
			if err != nil {
				return err
			}
			isMember, _, err := client.Organizations.IsMember(context.Background(), "openshift", name)
			if err != nil {
				return err
			}
			memberStatus := ""
			if !isMember {
				memberStatus = "NOT MEMBER"
			}
			fullNames = append(fullNames,
				fmt.Sprintf("%s (%s) %s", *user.Name, name, memberStatus),
			)
		}

		sort.Strings(fullNames)
		for _, name := range fullNames {
			fmt.Printf("%s\n", name)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(ownerNamesCmd)
}
