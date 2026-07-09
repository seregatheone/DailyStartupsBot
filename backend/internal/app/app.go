package app

import "fmt"

type Config struct {
	ServiceName   string
	Environment   string
	ListenAddress string
}

func DefaultConfig() Config {
	return Config{
		ServiceName:   "daily-startups-backend",
		Environment:   "local",
		ListenAddress: "127.0.0.1:8080",
	}
}

func StartupMessage(cfg Config) string {
	return fmt.Sprintf("%s starting in %s on %s", cfg.ServiceName, cfg.Environment, cfg.ListenAddress)
}
