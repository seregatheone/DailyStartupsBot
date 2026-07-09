package main

import (
	"fmt"
	"os"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/app"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

func main() {
	cfg, err := config.LoadFromEnv(os.Environ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stdout, app.StartupMessage(cfg))
}
