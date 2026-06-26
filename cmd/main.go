package main

import (
	"log/slog"
	"os"

	"tipharez-allmighty/youtube-scraper/internal/cli"
	"tipharez-allmighty/youtube-scraper/internal/config"

	"github.com/alecthomas/kong"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	var cli cli.CLI
	ctx := kong.Parse(&cli)
	err = ctx.Run(cfg)
	ctx.FatalIfErrorf(err)
}
