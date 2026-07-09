package app

import (
	"fmt"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

func StartupMessage(cfg config.Config) string {
	return fmt.Sprintf("%s starting in %s on %s", cfg.ServiceName, cfg.Environment, cfg.ListenAddress)
}
