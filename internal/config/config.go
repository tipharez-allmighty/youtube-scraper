// Package config.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	YoutubeAPIKey string `env:"YOUTUBE_API_KEY"`
	StateFile     string `env:"STATE_FILE" envDefault:"storage.db"`
	NumWorkers    int    `env:"NUM_WORKERS" envDefault:"5"`
	BufferSize    int    `env:"BUFFER_SIZE" envDefault:"10"`
}

func Load() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load .env file")
	}
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse .env file content into config struct")
	}
	return cfg, nil
}
