package logger

import (
	"log/slog"
	"os"
)

func New(appEnv string) *slog.Logger {
	if appEnv == "production" {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
