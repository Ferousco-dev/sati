package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Ferousco-dev/sati/backend/db"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate <up|down|status|version>")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	return goose.RunContext(context.Background(), args[0], conn, "migrations")
}
