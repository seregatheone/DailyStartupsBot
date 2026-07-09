package digest

import (
	"fmt"
	"html"
	"sort"
	"strings"

	v1 "github.com/seregatheone/DailyStartupsBot/backend/internal/contracts/v1"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

func (generator Generator) PreviewResponse(request Request) v1.PreviewResponse {
	digest := generator.Generate(request)
	messages := generator.RenderMessages(digest)
	return v1.PreviewResponse{Messages: messages, Empty: digest.Empty}
}

func (generator Generator) DeliveryMessages(request Request) []v1.DigestMessage {
	return generator.RenderMessages(generator.Generate(request))
}

// StoredDeliveryMessages renders the immutable digest snapshot referenced by a
// queued delivery. Delivery workers must not regenerate the digest from current
// source signals because that could change the message between retry attempts.
func (generator Generator) StoredDeliveryMessages(run storage.DigestRun, storedItems []storage.DigestItem) []v1.DigestMessage {
	items := append([]storage.DigestItem(nil), storedItems...)
	sort.SliceStable(items, func(left, right int) bool {
		return items[left].Rank < items[right].Rank
	})

	digestItems := make([]Item, 0, len(items))
	for _, stored := range items {
		sources := make([]SourceAttribution, 0, len(stored.SourceURLs))
		for _, sourceURL := range stored.SourceURLs {
			sources = append(sources, SourceAttribution{SourceID: sourceURL, SourceURL: sourceURL})
		}
		digestItems = append(digestItems, Item{
			StartupName: stored.StartupName,
			Description: stored.Summary,
			Sources:     sources,
		})
	}

	return generator.RenderMessages(Digest{
		Date:     run.DigestDate,
		Timezone: run.Timezone,
		Items:    digestItems,
		Empty:    len(digestItems) == 0,
	})
}

func (generator Generator) RenderMessages(digest Digest) []v1.DigestMessage {
	limit := generator.MessageLimit
	if limit <= 0 {
		limit = DefaultMessageLength
	}

	if digest.Empty {
		return []v1.DigestMessage{{
			Sequence: 1,
			Text:     fmt.Sprintf("No matching startup signals found for %s.", html.EscapeString(digest.Date)),
			ParseAs:  "HTML",
		}}
	}

	header := fmt.Sprintf("<b>Daily startup digest</b> %s", html.EscapeString(digest.Date))
	if len(header) > limit {
		header = escapeAndTruncate("Daily startup digest "+digest.Date, limit)
	}
	maxBlockLength := limit - len(header) - 2
	if maxBlockLength <= 0 {
		return []v1.DigestMessage{{Sequence: 1, Text: header, ParseAs: "HTML"}}
	}
	var messages []string
	current := header
	for index, item := range digest.Items {
		block := renderItem(index+1, item)
		if len(block) > maxBlockLength {
			block = renderOversizedItem(index+1, item, maxBlockLength)
		}
		if len(current)+2+len(block) > limit && current != header {
			messages = append(messages, current)
			current = header + "\n\n" + block
			continue
		}
		current += "\n\n" + block
	}
	messages = append(messages, current)

	rendered := make([]v1.DigestMessage, 0, len(messages))
	for index, message := range messages {
		rendered = append(rendered, v1.DigestMessage{
			Sequence: index + 1,
			Text:     message,
			ParseAs:  "HTML",
		})
	}
	return rendered
}

func renderOversizedItem(index int, item Item, limit int) string {
	plain := fmt.Sprintf("%d. %s", index, item.StartupName)
	if item.Description != "" {
		plain += "\n" + item.Description
	}
	return escapeAndTruncate(plain, limit)
}

func escapeAndTruncate(value string, limit int) string {
	escaped := html.EscapeString(value)
	if len(escaped) <= limit {
		return escaped
	}
	const ellipsis = "…"
	if limit < len(ellipsis) {
		return strings.Repeat(".", limit)
	}

	var builder strings.Builder
	budget := limit - len(ellipsis)
	for _, character := range value {
		escapedCharacter := html.EscapeString(string(character))
		if builder.Len()+len(escapedCharacter) > budget {
			break
		}
		builder.WriteString(escapedCharacter)
	}
	builder.WriteString(ellipsis)
	return builder.String()
}

func renderItem(index int, item Item) string {
	parts := []string{
		fmt.Sprintf("%d. <b>%s</b>", index, html.EscapeString(item.StartupName)),
	}
	if item.Description != "" {
		parts = append(parts, html.EscapeString(item.Description))
	}
	details := renderDetails(item)
	if details != "" {
		parts = append(parts, details)
	}
	attribution := renderAttribution(item.Sources)
	if attribution != "" {
		parts = append(parts, attribution)
	}
	return strings.Join(parts, "\n")
}

func renderDetails(item Item) string {
	var details []string
	if item.SignalType != "" {
		details = append(details, "signal: "+html.EscapeString(item.SignalType))
	}
	if item.Region != "" {
		details = append(details, "region: "+html.EscapeString(item.Region))
	}
	if len(item.Categories) > 0 {
		details = append(details, "categories: "+html.EscapeString(strings.Join(item.Categories, ", ")))
	}
	if funding := renderFunding(item.Funding); funding != "" {
		details = append(details, funding)
	}
	if len(details) == 0 {
		return ""
	}
	return strings.Join(details, " | ")
}

func renderFunding(funding FundingInfo) string {
	var parts []string
	if funding.Round != "" {
		parts = append(parts, html.EscapeString(funding.Round))
	}
	if funding.Amount != "" {
		amount := html.EscapeString(funding.Amount)
		if funding.Currency != "" {
			amount += " " + html.EscapeString(funding.Currency)
		}
		parts = append(parts, amount)
	}
	if len(funding.Investors) > 0 {
		parts = append(parts, "investors: "+html.EscapeString(strings.Join(funding.Investors, ", ")))
	}
	if len(parts) == 0 {
		return ""
	}
	return "funding: " + strings.Join(parts, ", ")
}

func renderAttribution(sources []SourceAttribution) string {
	if len(sources) == 0 {
		return ""
	}
	var parts []string
	for _, source := range sources {
		label := html.EscapeString(source.SourceID)
		if source.SourceURL == "" {
			parts = append(parts, label)
			continue
		}
		parts = append(parts, fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(source.SourceURL), label))
	}
	return "sources: " + strings.Join(parts, ", ")
}
