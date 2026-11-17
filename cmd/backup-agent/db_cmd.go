package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
	"github.com/plusclouds/ubuntu-agent/internal/backup/dbdump"
)

// newDBCmd is `stash db`: list and dump databases with exactly the drivers
// and local connection settings a job_type "database" job uses, but without
// NATS, the platform or an upload. It exists to check a machine's database
// setup (client tools, credentials, permissions) before a job is scheduled.
func newDBCmd() *cobra.Command {
	var engine, connection string

	cmd := &cobra.Command{
		Use:   "db",
		Short: "List or dump local databases (checks the setup used by database jobs)",
		// A failure here is a database problem (bad password, missing
		// tool), not a usage mistake — don't bury it under the help text.
		SilenceUsage: true,
	}
	cmd.PersistentFlags().StringVar(&engine, "engine", "", "Database engine: postgres, mysql or mssql")
	cmd.PersistentFlags().StringVar(&connection, "connection", "",
		`Connection name from the config's "databases" section (default: "default")`)
	_ = cmd.MarkPersistentFlagRequired("engine")

	driver := func() (dbdump.Driver, error) {
		cfg, err := backupconfig.Load(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}
		return dbdump.New(engine, cfg.DBConn(engine, connection))
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the databases that can be backed up",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			drv, err := driver()
			if err != nil {
				return err
			}
			names, err := drv.ListDatabases(ctx)
			if err != nil {
				return err
			}
			for _, name := range names {
				fmt.Println(name)
			}
			return nil
		},
	})

	var all bool
	var out string
	dump := &cobra.Command{
		Use:   "dump [database...]",
		Short: "Dump databases into a directory (no upload)",
		RunE: func(_ *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			drv, err := driver()
			if err != nil {
				return err
			}
			names, err := dbdump.Select(ctx, drv, args, all)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(out, 0o700); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}
			for _, name := range names {
				path, err := drv.Dump(ctx, name, out)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				size := int64(-1)
				if info, err := os.Stat(path); err == nil {
					size = info.Size()
				}
				fmt.Printf("%s\t%s\t%d bytes\n", name, path, size)
			}
			return nil
		},
	}
	dump.Flags().BoolVar(&all, "all", false, "Dump every user database")
	dump.Flags().StringVar(&out, "out", "", "Directory to write the dump files into")
	_ = dump.MarkFlagRequired("out")
	cmd.AddCommand(dump)

	return cmd
}
