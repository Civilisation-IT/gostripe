package cmd

import (
	"net/url"
	"os"

	"gostripe/conf"

	"github.com/gobuffalo/pop/v5"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var migrateCmd = cobra.Command{
	Use:  "migrate",
	Long: "Migrate the database",
	Run: func(cmd *cobra.Command, args []string) {
		execWithConfig(cmd, migrate)
	},
}

func migrate(config *conf.GlobalConfiguration) {
	if config.DB.Driver == "" && config.DB.URL != "" {
		u, err := url.Parse(config.DB.URL)
		if err != nil {
			logrus.Fatalf("%+v", errors.Wrap(err, "parsing db connection url"))
		}
		config.DB.Driver = u.Scheme
	}
	pop.Debug = true

	deets := &pop.ConnectionDetails{
		Dialect: config.DB.Driver,
		URL:     config.DB.URL,
	}
	if config.DB.Namespace != "" {
		deets.Options = map[string]string{
			"Namespace": config.DB.Namespace + "_",
		}
	}

	db, err := pop.NewConnection(deets)
	if err != nil {
		logrus.Fatalf("%+v", errors.Wrap(err, "opening db connection"))
	}
	defer db.Close()

	if err := db.Open(); err != nil {
		logrus.Fatalf("%+v", errors.Wrap(err, "checking database connection"))
	}

	logrus.Infof("Reading migrations from %s", config.DB.MigrationsPath)
	mig, err := pop.NewFileMigrator(config.DB.MigrationsPath, db)
	if err != nil {
		logrus.Fatalf("%+v", errors.Wrap(err, "creating db migrator"))
	}
	logrus.Infof("before status")
	err = mig.Status(os.Stdout)
	if err != nil {
		logrus.Fatalf("%+v", errors.Wrap(err, "migration status"))
	}
	// turn off schema dump
	mig.SchemaPath = ""

	err = mig.Up()
	if err != nil {
		logrus.Fatalf("%+v", errors.Wrap(err, "running db migrations"))
	}

	logrus.Infof("after status")
	err = mig.Status(os.Stdout)
	if err != nil {
		logrus.Fatalf("%+v", errors.Wrap(err, "migration status"))
	}
}
