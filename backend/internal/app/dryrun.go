package app

import (
	"context"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/digest"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/ops"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

type DryRunResult struct {
	Ingestion ingestion.RunResult
	Preview   digest.Digest
	Messages  []string
	Health    ops.HealthSummary
}

func RunDryRun(ctx context.Context, cfg config.Config, now time.Time) (DryRunResult, error) {
	registry, sources, err := ingestion.AssembleRuntime(true, cfg.Sources)
	if err != nil {
		return DryRunResult{}, err
	}
	cfg.Sources = sources
	ingestionService := ingestion.NewService(registry, nil)
	ingestionResult, err := ingestionService.Run(ctx, cfg.Sources)
	if err != nil {
		return DryRunResult{}, err
	}

	preferences := storage.Preferences{
		DeliveryTime: cfg.DeliveryTime,
		Timezone:     cfg.Timezone,
		MaxItems:     digest.DefaultItemLimit,
	}
	digestDate := now.Format("2006-01-02")
	generator := digest.Generator{}
	preview := generator.Generate(digest.Request{
		Signals:     ingestionResult.Signals,
		Preferences: preferences,
		DigestDate:  digestDate,
		Timezone:    cfg.Timezone,
	})
	digestMessages := generator.RenderMessages(preview)
	messages := make([]string, 0, len(digestMessages))
	for _, message := range digestMessages {
		messages = append(messages, message.Text)
	}

	health := ops.HealthFromDryRun(now, ingestionResult)
	return DryRunResult{
		Ingestion: ingestionResult,
		Preview:   preview,
		Messages:  messages,
		Health:    health,
	}, nil
}
