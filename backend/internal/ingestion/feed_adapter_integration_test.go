package ingestion_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/config"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/digest"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func TestHostileFeedContentIsSafeThroughTelegramDigestRendering(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
		_, _ = fmt.Fprintf(writer, `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>hostile-entry</id>
    <title>Acme &lt;b&gt;Growth&lt;/b&gt; &amp;amp; Co launches platform</title>
    <updated>2026-07-09T08:00:00Z</updated>
    <link rel="alternate" href="%s/article?a=1&amp;b=2" />
    <summary>&lt;script&gt;alert(1)&lt;/script&gt; Useful &amp;amp; safe summary.</summary>
  </entry>
</feed>`, server.URL)
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := ingestion.NewFeedAdapter(ingestion.FeedAdapterOptions{
		ID:                  "hostile-feed",
		DisplayName:         "Hostile Feed",
		FeedURL:             server.URL,
		AccessMethod:        "atom",
		FetchCadence:        "hourly",
		RateLimit:           "one request per hour",
		Tags:                []string{"feed"},
		AllowedHosts:        []string{parsed.Host},
		AllowedContentTypes: []string{"application/atom+xml"},
		Timeout:             time.Second,
		MaxRedirects:        1,
		MaxResponseBytes:    1 << 20,
		MaxItems:            10,
		UserAgent:           ingestion.DefaultFeedUserAgent,
		Transport:           server.Client().Transport,
		Mapper: func(item ingestion.FeedItem) (ingestion.SourceRecord, error) {
			return ingestion.SourceRecord{
				StartupName:  item.Title + " <i>mapped</i> & partner\u202e",
				CanonicalURL: "https://startup.example/path",
				SourceURL:    item.SourceURL,
				SignalType:   `<em>launch</em>`,
				PublishedAt:  item.PublishedAt,
				Description:  item.Description + ` <script>mapped()</script> & details`,
				Region:       `<b>UK</b> & Europe`,
				Categories:   []string{`<i>AI</i> & tooling`},
				Funding: ingestion.Funding{
					Round: `<b>seed</b>`, Amount: `<i>8m</i>`, Currency: `GBP & EUR`, Investors: []string{`<u>Northwind & Co</u>`},
				},
				RawPayload: `<feed><script>raw()</script></feed>`,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("new feed adapter: %v", err)
	}

	fetched, err := adapter.Fetch(context.Background(), config.SourceConfig{})
	if err != nil || len(fetched.Records) != 1 || fetched.Skipped != 0 {
		t.Fatalf("fetch hostile fixture: result=%#v err=%v", fetched, err)
	}
	signal, err := ingestion.NormalizeSignal("hostile-feed", fetched.Records[0])
	if err != nil {
		t.Fatalf("normalize signal: %v", err)
	}
	if strings.Contains(signal.RawPayload, "<feed") || strings.Contains(signal.RawPayload, "script") {
		t.Fatalf("raw feed content reached storage payload: %q", signal.RawPayload)
	}

	preview := (digest.Generator{}).PreviewResponse(digest.Request{
		Signals:     []storage.StartupSignal{signal},
		Preferences: storage.Preferences{MaxItems: 10},
		DigestDate:  "2026-07-09",
		Timezone:    "UTC",
	})
	if len(preview.Messages) != 1 || preview.Messages[0].ParseAs != "HTML" {
		t.Fatalf("unexpected preview messages: %#v", preview.Messages)
	}
	text := preview.Messages[0].Text
	for _, unsafe := range []string{"<script>", "</script>", "alert(1)", "mapped()", "<i>mapped</i>", "<em>launch</em>", "\u202e", `?a=1&b=2`} {
		if strings.Contains(text, unsafe) {
			t.Fatalf("unsafe feed content reached Telegram HTML: %q in %q", unsafe, text)
		}
	}
	if !strings.Contains(text, "&amp;") || !strings.Contains(text, `?a=1&amp;b=2`) {
		t.Fatalf("Telegram text or href was not escaped: %q", text)
	}
}
