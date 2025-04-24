package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the current version of the GoStripe API
var Version = "dev"

var versionCmd = cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(Version)
	},
}
