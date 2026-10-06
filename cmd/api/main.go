package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/admin"
	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/categories"
	"github.com/Ferousco-dev/sati/backend/internal/email"
	"github.com/Ferousco-dev/sati/backend/internal/files"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/internal/products"
	"github.com/Ferousco-dev/sati/backend/internal/server"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.AppEnv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	mailer := email.NewLogMailer(log, !cfg.IsProduction())
	authSvc := auth.NewService(pool, mailer, log, cfg.SessionSecret, auth.DefaultSettings())

	storage, err := files.NewLocalStorage(cfg.UploadDir, cfg.AssetBaseURL)
	if cfg.StorageDriver != "local" {
		err = fmt.Errorf("storage driver %q is not implemented yet", cfg.StorageDriver)
	}
	if err != nil {
		return err
	}

	categorySvc := categories.NewService(categories.NewRepository(pool))
	productSvc := products.NewService(pool, storage, log)
	handlers := server.Handlers{
		Health:     health.New(pool),
		Auth:       auth.NewHandler(authSvc, log, cfg.IsProduction()),
		Categories: categories.NewHandler(categorySvc, log),
		Products:   products.NewHandler(productSvc, log),
		Admin:      admin.NewHandler(admin.NewService(pool, authSvc), log),
		LocalFiles: storage.Handler(),
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(cfg, log, handlers),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("api listening", "port", cfg.Port, "env", cfg.AppEnv)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}
