package main

import (
	"fmt"
	"os"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/app"
)

func main() {
	cfg := app.DefaultConfig()
	fmt.Fprintln(os.Stdout, app.StartupMessage(cfg))
}
