package ingestion

import (
	"fmt"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
)

type Registry struct {
	adapters        map[string]SourceAdapter
	displayEligible map[string]bool
	revision        string
}

func NewRegistry(adapters ...SourceAdapter) Registry {
	byID := map[string]SourceAdapter{}
	for _, adapter := range adapters {
		metadata := adapter.Metadata()
		byID[metadata.ID] = adapter
	}
	return Registry{
		adapters:        byID,
		displayEligible: map[string]bool{},
		revision:        "unreviewed-runtime",
	}
}

func DefaultRegistry() Registry {
	return newRegistryWithDisplayPolicy(
		[]SourceAdapter{NewSamplePublicAdapter()},
		map[string]bool{"sample-public": true},
		"dry-run-sample-v1",
	)
}

func NewRegistryWithDisplayPolicy(
	adapters []SourceAdapter,
	displayEligibleSourceIDs []string,
	revision string,
) Registry {
	displayEligible := make(map[string]bool, len(displayEligibleSourceIDs))
	for _, sourceID := range displayEligibleSourceIDs {
		if sourceID != "" {
			displayEligible[sourceID] = true
		}
	}
	return newRegistryWithDisplayPolicy(adapters, displayEligible, revision)
}

func newRegistryWithDisplayPolicy(
	adapters []SourceAdapter,
	displayEligible map[string]bool,
	revision string,
) Registry {
	registry := NewRegistry(adapters...)
	registry.displayEligible = make(map[string]bool, len(displayEligible))
	for sourceID, eligible := range displayEligible {
		registry.displayEligible[sourceID] = eligible
	}
	registry.revision = revision
	return registry
}

func (registry Registry) DisplayEligible(sourceID string) bool {
	return sourceID != "" && registry.displayEligible[sourceID]
}

func (registry Registry) Revision() string {
	return registry.revision
}

func (registry Registry) Resolve(configs []config.SourceConfig) ([]RegisteredSource, []SourceResult) {
	var sources []RegisteredSource
	var skipped []SourceResult

	for _, sourceConfig := range configs {
		if !sourceConfig.Active {
			skipped = append(skipped, SourceResult{
				SourceID: sourceConfig.ID,
				Status:   StatusSkipped,
				Skipped:  1,
				Message:  "source disabled by configuration",
			})
			continue
		}

		adapter, ok := registry.adapters[sourceConfig.ID]
		if !ok {
			skipped = append(skipped, SourceResult{
				SourceID: sourceConfig.ID,
				Status:   StatusConfigError,
				Message:  "source has no registered adapter",
			})
			continue
		}

		metadata := adapter.Metadata()
		if sourceConfig.AccessMethod != metadata.AccessMethod {
			skipped = append(skipped, SourceResult{
				SourceID: sourceConfig.ID,
				Status:   StatusConfigError,
				Message: fmt.Sprintf(
					"configured access method %q does not match approved adapter method %q",
					sourceConfig.AccessMethod,
					metadata.AccessMethod,
				),
			})
			continue
		}

		if missing := missingCredentials(metadata.RequiredCredentials, sourceConfig.Credentials); len(missing) > 0 {
			skipped = append(skipped, SourceResult{
				SourceID: sourceConfig.ID,
				Status:   StatusConfigError,
				Message:  fmt.Sprintf("missing credentials: %v", missing),
			})
			continue
		}

		sources = append(sources, RegisteredSource{
			Config:   sourceConfig,
			Adapter:  adapter,
			Metadata: metadata,
		})
	}

	return sources, skipped
}

func missingCredentials(required []string, credentials map[string]string) []string {
	var missing []string
	for _, name := range required {
		if credentials[name] == "" {
			missing = append(missing, name)
		}
	}
	return missing
}
