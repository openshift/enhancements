/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sort"

	"github.com/openshift/enhancements/tools/util"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type Owners struct {
	Approvers []string `yaml:"approvers"`
}

// ownerNamesCmd represents the ownerNames command
var ownerNamesCmd = &cobra.Command{
	Use:   "owner-names",
	Short: "Show the names for accounts in the owners list",
	Long:  `Look up real names for owners in the owners file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initConfig()

		content, err := ioutil.ReadFile("../OWNERS")
		if err != nil {
			return err
		}
		owners := Owners{}
		err = yaml.Unmarshal(content, &owners)
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

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// ownerNamesCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// ownerNamesCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
