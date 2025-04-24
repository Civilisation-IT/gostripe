package cmd

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"gostripe/conf"
)

var configFile = ""

var rootCmd = cobra.Command{
	Use: "gostripe",
	Run: func(cmd *cobra.Command, args []string) {
		execWithConfig(cmd, serve)
	},
}

// RootCommand will setup and return the root command
func RootCommand() *cobra.Command {
	rootCmd.AddCommand(&serveCmd, &migrateCmd, &versionCmd)
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "the config file to use")

	return &rootCmd
}

// execWithConfig runs a function with the config
func execWithConfig(cmd *cobra.Command, fn func(config *conf.GlobalConfiguration)) {
	config, err := conf.LoadGlobal(configFile)
	if err != nil {
		logrus.Fatalf("Failed to load configuration: %+v", err)
	}
	fn(config)
}
