// Package config.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	YoutubeAPIKey  string `env:"YOUTUBE_API_KEY,required"`
	YoutubeBaseURL string `env:"YOUTUBE_BASE_URL" envDefault:"https://www.googleapis.com/youtube/v3/"`
	StateFile      string `env:"STATE_FILE" envDefault:"storage.db"`
	NumWorkers     int    `env:"NUM_WORKERS" envDefault:"5"`
	BufferSize     int    `env:"BUFFER_SIZE" envDefault:"10"`
	MaxRetries     int    `env:"MAX_RETRIES" envDefault:"3"`
	LimitRows      int    `env:"LIMIT_ROWS" envDefault:"5000"`
}

func Load() (*Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to find user config dir: %w", err)
	}
	envPath := filepath.Join(configDir, "yt-scraper", ".env")
	if err := godotenv.Load(envPath); err != nil {
		return nil, fmt.Errorf("failed to load config file from %s: %w", envPath, err)
	}
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse .env config: %w", err)
	}
	return cfg, nil
}
