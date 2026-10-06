package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

const usage = `usage:
  admin create  -email EMAIL -name NAME   (password from ADMIN_PASSWORD or stdin)
  admin promote -email EMAIL`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}

	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	email := fs.String("email", "", "account email")
	name := fs.String("name", "", "display name (create only)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	*email = strings.ToLower(strings.TrimSpace(*email))
	if *email == "" {
		return errors.New(usage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := users.NewRepository()
	now := time.Now()

	switch args[0] {
	case "create":
		if strings.TrimSpace(*name) == "" {
			return errors.New("-name is required")
		}
		password, err := readPassword()
		if err != nil {
			return err
		}
		if n := len([]rune(password)); n < 8 || n > 128 {
			return errors.New("password must be between 8 and 128 characters")
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			u, err := repo.Create(ctx, tx, strings.TrimSpace(*name), *email, hash)
			if err != nil {
				return err
			}
			if err := repo.SetRole(ctx, tx, u.ID, users.RoleAdmin, now); err != nil {
				return err
			}
			if err := repo.MarkEmailVerified(ctx, tx, u.ID, now); err != nil {
				return err
			}
			fmt.Println("created admin", u.Email)
			return nil
		})

	case "promote":
		u, err := repo.ByEmail(ctx, pool, *email)
		if err != nil {
			return err
		}
		if err := repo.SetRole(ctx, pool, u.ID, users.RoleAdmin, now); err != nil {
			return err
		}
		fmt.Println("promoted", u.Email, "to ADMIN")
		return nil
	}
	return errors.New(usage)
}

func readPassword() (string, error) {
	if p := os.Getenv("ADMIN_PASSWORD"); p != "" {
		return p, nil
	}
	fmt.Fprint(os.Stderr, "password: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
