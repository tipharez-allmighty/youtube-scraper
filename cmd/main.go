package main

import (
	"encoding/json"
	"fmt"
	"os"

	"tipharez-allmighty/youtube-scraper/youtube"

	"github.com/go-playground/validator/v10"
)

func main() {
	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "youtube api key is not set")
		os.Exit(1)
	}
	var input youtube.InputSchema
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintln(os.Stderr, "invalid input:", err)
	}
	inputValidator := validator.New()
	if err := inputValidator.Struct(input); err != nil {
		fmt.Fprintln(os.Stderr, "wrong input format:", err)
	}
	fmt.Println(input)
}
