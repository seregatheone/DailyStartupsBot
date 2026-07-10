package ingestion

import (
	"errors"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestNormalizeSignalCanonicalizesTrackingWithoutDroppingFunctionalQuery(t *testing.T) {
	publishedAt := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	first := SourceRecord{
		StartupName:  " Acme   AI ",
		CanonicalURL: "https://ACME.example:443/?utm_source=newsletter&gclid=click",
		SourceURL:    "https://source.example/item?utm_campaign=one",
		SignalType:   "LAUNCH", PublishedAt: publishedAt,
	}
	second := first
	second.CanonicalURL = "https://acme.example"
	second.SourceURL = "https://source.example/item?utm_campaign=two"

	one, err := NormalizeSignal("source", first)
	if err != nil {
		t.Fatalf("normalize first: %v", err)
	}
	two, err := NormalizeSignal("source", second)
	if err != nil {
		t.Fatalf("normalize second: %v", err)
	}
	if one.CanonicalURL != "https://acme.example" || one.ID != two.ID || one.StartupName != "Acme AI" {
		t.Fatalf("tracking variants did not normalize deterministically: one=%#v two=%#v", one, two)
	}

	refMain := first
	refMain.CanonicalURL = "https://acme.example/product?ref=main&product=1"
	refMain.SourceURL = "https://source.example/item?ref=main"
	refDev := refMain
	refDev.CanonicalURL = "https://acme.example/product?ref=dev&product=1"
	refDev.SourceURL = "https://source.example/item?ref=dev"
	mainSignal, err := NormalizeSignal("source", refMain)
	if err != nil {
		t.Fatalf("normalize ref main: %v", err)
	}
	devSignal, err := NormalizeSignal("source", refDev)
	if err != nil {
		t.Fatalf("normalize ref dev: %v", err)
	}
	if mainSignal.CanonicalURL != "https://acme.example/product?product=1&ref=main" ||
		devSignal.CanonicalURL != "https://acme.example/product?product=1&ref=dev" ||
		mainSignal.ID == devSignal.ID {
		t.Fatalf("functional query was removed: main=%#v dev=%#v", mainSignal, devSignal)
	}
}

func TestNormalizeSignalRejectsOneBoundedQualityReason(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	valid := SourceRecord{
		StartupName: "Acme", SourceURL: "https://source.example/acme",
		SignalType: "launch", PublishedAt: now.Add(-time.Hour),
	}
	tests := []struct {
		name     string
		sourceID string
		mutate   func(*SourceRecord)
		want     QualityRejectReason
	}{
		{name: "source id", sourceID: "", want: RejectMissingSourceID},
		{name: "name", sourceID: "source", mutate: func(record *SourceRecord) { record.StartupName = "" }, want: RejectMissingStartupName},
		{name: "unsafe name", sourceID: "source", mutate: func(record *SourceRecord) { record.StartupName = "Acme\u202e" }, want: RejectInvalidStartupName},
		{name: "source URL", sourceID: "source", mutate: func(record *SourceRecord) { record.SourceURL = "" }, want: RejectMissingSourceURL},
		{name: "invalid source URL", sourceID: "source", mutate: func(record *SourceRecord) { record.SourceURL = "http://source.example/acme" }, want: RejectInvalidSourceURL},
		{name: "relative source URL", sourceID: "source", mutate: func(record *SourceRecord) { record.SourceURL = "/acme" }, want: RejectInvalidSourceURL},
		{name: "credential source URL", sourceID: "source", mutate: func(record *SourceRecord) { record.SourceURL = "https://user:secret@source.example/acme" }, want: RejectInvalidSourceURL},
		{name: "published", sourceID: "source", mutate: func(record *SourceRecord) { record.PublishedAt = time.Time{} }, want: RejectMissingPublishedAt},
		{name: "canonical", sourceID: "source", mutate: func(record *SourceRecord) { record.CanonicalURL = "https://user:secret@acme.example" }, want: RejectInvalidCanonicalURL},
		{name: "type", sourceID: "source", mutate: func(record *SourceRecord) { record.SignalType = "rumor" }, want: RejectInvalidSignalType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := valid
			if test.mutate != nil {
				test.mutate(&record)
			}
			_, err := NormalizeSignalWithPolicy(test.sourceID, record, now, QualityPolicy{
				MaxAge: 7 * 24 * time.Hour, MaxFutureSkew: 15 * time.Minute,
			})
			assertQualityReason(t, err, test.want)
		})
	}
}

func TestNormalizeSignalEnforcesFreshnessBoundaries(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	policy := QualityPolicy{MaxAge: 7 * 24 * time.Hour, MaxFutureSkew: 15 * time.Minute}
	record := SourceRecord{
		StartupName: "Acme", SourceURL: "https://source.example/acme", SignalType: "launch",
	}
	for _, publishedAt := range []time.Time{now.Add(-policy.MaxAge), now.Add(policy.MaxFutureSkew)} {
		record.PublishedAt = publishedAt
		if _, err := NormalizeSignalWithPolicy("source", record, now, policy); err != nil {
			t.Fatalf("boundary record was rejected at %s: %v", publishedAt, err)
		}
	}
	record.PublishedAt = now.Add(-policy.MaxAge - time.Nanosecond)
	_, err := NormalizeSignalWithPolicy("source", record, now, policy)
	assertQualityReason(t, err, RejectStale)
	record.PublishedAt = now.Add(policy.MaxFutureSkew + time.Nanosecond)
	_, err = NormalizeSignalWithPolicy("source", record, now, policy)
	assertQualityReason(t, err, RejectFuture)
}

func TestSignalIdentitySeparatesExactAndLegalSuffixAliases(t *testing.T) {
	base := storageSignal("Acme, Ltd.", "", "EU")
	exactVariant := storageSignal("ACME LTD", "", "EU")
	otherSuffix := storageSignal("Acme Inc", "", "EU")

	first := SignalIdentityForScope(base, "2026-07-10")
	second := SignalIdentityForScope(exactVariant, "2026-07-10")
	third := SignalIdentityForScope(otherSuffix, "2026-07-10")
	if first.ExactName != second.ExactName || first.ExactName == third.ExactName ||
		first.SuffixName == "" || first.SuffixName != third.SuffixName {
		t.Fatalf("unexpected alias identities: first=%#v second=%#v third=%#v", first, second, third)
	}
	if first.ExactName == SignalIdentityForScope(base, "2026-07-11").ExactName ||
		first.ExactName == SignalIdentityForScope(storageSignal("Acme, Ltd.", "", "US"), "2026-07-10").ExactName {
		t.Fatal("digest scope or region was not included in alias identity")
	}
}

func assertQualityReason(t *testing.T, err error, want QualityRejectReason) {
	t.Helper()
	var qualityErr *QualityError
	if !errors.As(err, &qualityErr) || qualityErr.Reason != want {
		t.Fatalf("expected quality reason %q, got %v", want, err)
	}
}

func storageSignal(name, canonicalURL, region string) storage.StartupSignal {
	return storage.StartupSignal{
		ID: "signal", StartupName: name, CanonicalURL: canonicalURL,
		SourceID: "source", SourceURL: "https://source.example/item",
		SignalType: "launch", PublishedAt: time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC), Region: region,
	}
}
