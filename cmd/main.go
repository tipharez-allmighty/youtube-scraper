package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"tipharez-allmighty/youtube-scraper/internal/cli"
	"tipharez-allmighty/youtube-scraper/internal/config"

	"github.com/alecthomas/kong"
)

func main() {
	loggerLevel := new(slog.LevelVar)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: loggerLevel}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var cli cli.CLI
	kongCtx := kong.Parse(&cli)
	switch cli.Verbose {
	case 1:
		loggerLevel.Set(slog.LevelInfo)
	case 2:
		loggerLevel.Set(slog.LevelDebug)
	default:
		loggerLevel.Set(slog.LevelWarn)
	}
	kongCtx.BindTo(ctx, (*context.Context)(nil))
	err = kongCtx.Run(cfg)
	kongCtx.FatalIfErrorf(err)
}
