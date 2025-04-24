package cmd

import (
	"context"
	"fmt"

	"gostripe/api"
	"gostripe/conf"
	"gostripe/storage"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var serveCmd = cobra.Command{
	Use:  "serve",
	Long: "Start the GoStripe API server",
	Run: func(cmd *cobra.Command, args []string) {
		execWithConfig(cmd, serve)
	},
}

func serve(config *conf.GlobalConfiguration) {
	db, err := storage.Dial(config)
	if err != nil {
		logrus.Fatalf("Error opening database: %+v", err)
	}
	defer db.Close()

	ctx := context.Background()
	api := api.NewAPIWithVersion(ctx, config, db, Version)

	l := fmt.Sprintf("%v:%v", config.API.Host, config.API.Port)
	logrus.Infof("GoStripe API started on: %s", l)
	api.ListenAndServe(l)
}
