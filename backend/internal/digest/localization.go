package digest

import (
	"fmt"
	"html"
	"strings"
)

var categoryTranslations = map[string]string{
	"ai":                      "ИИ",
	"artificial intelligence": "искусственный интеллект",
	"analytics":               "аналитика",
	"biotech":                 "биотехнологии",
	"climate":                 "климатические технологии",
	"climate tech":            "климатические технологии",
	"consumer":                "потребительские продукты",
	"cybersecurity":           "кибербезопасность",
	"data":                    "данные",
	"deep tech":               "глубокие технологии",
	"deeptech":                "глубокие технологии",
	"developer tools":         "инструменты для разработчиков",
	"education":               "образование",
	"edtech":                  "образовательные технологии",
	"energy":                  "энергетика",
	"enterprise":              "корпоративные решения",
	"finance":                 "финансы",
	"fintech":                 "финтех",
	"hardware":                "аппаратные решения",
	"healthcare":              "здравоохранение",
	"healthtech":              "медтех",
	"open source":             "открытый код",
	"productivity":            "продуктивность",
	"robotics":                "робототехника",
	"software":                "программное обеспечение",
}

var regionTranslations = map[string]string{
	"asia":          "Азия",
	"eu":            "Европа",
	"europe":        "Европа",
	"france":        "Франция",
	"germany":       "Германия",
	"global":        "весь мир",
	"india":         "Индия",
	"north america": "Северная Америка",
	"uk":            "Великобритания",
	"us":            "США",
	"usa":           "США",
}

var fundingRoundTranslations = map[string]string{
	"accelerator": "акселерационная программа",
	"debt":        "долговое финансирование",
	"grant":       "грант",
	"growth":      "раунд роста",
	"pre-seed":    "предпосевной раунд",
	"seed":        "посевной раунд",
	"series a":    "раунд A",
	"series b":    "раунд B",
	"series c":    "раунд C",
	"series d":    "раунд D",
}

func renderDescription(item Item) string {
	description := plainDescription(item)
	if description == "" {
		return ""
	}
	return "📝 <b>Описание:</b> " + html.EscapeString(description)
}

func plainDescription(item Item) string {
	description := strings.TrimSpace(item.Description)
	if description != "" {
		return description
	}
	return synthesizedDescription(item)
}

func synthesizedDescription(item Item) string {
	var facts []string
	if len(item.Categories) > 0 {
		facts = append(facts, "сфера — "+strings.Join(displayCategories(item.Categories), ", "))
	}
	if region := strings.TrimSpace(item.Region); region != "" {
		facts = append(facts, "регион — "+displayRegion(region))
	}
	if len(facts) > 0 {
		return "Стартап: " + strings.Join(facts, "; ") + "."
	}
	if signalType := strings.TrimSpace(item.SignalType); signalType != "" {
		return "Стартап с актуальным сигналом: " + displaySignalType(signalType) + "."
	}
	return ""
}

func renderWhyInteresting(item Item) string {
	reason := whyInterestingText(item)
	if reason == "" {
		return ""
	}
	return "💡 <b>Почему интересно:</b> " + html.EscapeString(reason)
}

func whyInterestingText(item Item) string {
	var reasons []string
	signalType := strings.ToLower(strings.TrimSpace(item.SignalType))
	switch signalType {
	case "launch":
		reasons = append(reasons, "Свежий запуск показывает, что продукт уже представлен рынку.")
	case "funding":
		reasons = append(reasons, fundingReason(item.Funding))
	case "news":
		reasons = append(reasons, "О компании вышла свежая новость — можно проследить актуальное изменение в её развитии.")
	case "acquisition":
		reasons = append(reasons, "Зафиксировано приобретение компании — это существенное событие в развитии проекта.")
	case "award":
		reasons = append(reasons, "Компания получила награду — это внешний сигнал признания проекта.")
	case "ranking":
		reasons = append(reasons, "Компания вошла в отраслевой рейтинг — это внешний сигнал заметности проекта.")
	}
	if signalType != "funding" && hasFunding(item.Funding) {
		reasons = append(reasons, fundingReason(item.Funding))
	}
	if len(item.Categories) > 0 {
		reasons = append(reasons, "Фокус проекта: "+strings.Join(displayCategories(item.Categories), ", ")+".")
	}
	if region := strings.TrimSpace(item.Region); region != "" {
		reasons = append(reasons, "Регион проекта: "+displayRegion(region)+".")
	}
	if sourceCount := len(attributedSourceKeys(item.Sources)); sourceCount > 1 {
		reasons = append(reasons, fmt.Sprintf("Сигнал отмечен в %d независимых источниках.", sourceCount))
	}
	if len(reasons) == 0 {
		return ""
	}
	return strings.Join(reasons, " ")
}

func fundingReason(funding FundingInfo) string {
	details := fundingFacts(funding)
	if details == "" {
		return "Зафиксирован новый раунд финансирования — у компании появились ресурсы для следующего этапа развития."
	}
	return "Компания привлекла финансирование (" + details + ") — это ресурс для следующего этапа развития."
}

func fundingFacts(funding FundingInfo) string {
	var facts []string
	if round := strings.TrimSpace(funding.Round); round != "" {
		facts = append(facts, displayFundingRound(round))
	}
	if amount := strings.TrimSpace(funding.Amount); amount != "" {
		if currency := strings.TrimSpace(funding.Currency); currency != "" {
			amount += " " + currency
		}
		facts = append(facts, amount)
	}
	if len(funding.Investors) > 0 {
		facts = append(facts, "инвесторы: "+strings.Join(funding.Investors, ", "))
	}
	return strings.Join(facts, ", ")
}

func hasFunding(funding FundingInfo) bool {
	return strings.TrimSpace(funding.Round) != "" || strings.TrimSpace(funding.Amount) != "" ||
		strings.TrimSpace(funding.Currency) != "" || len(funding.Investors) > 0
}

func displayCategories(categories []string) []string {
	displayed := make([]string, 0, len(categories))
	for _, category := range categories {
		displayed = append(displayed, translatedOrOriginal(category, categoryTranslations))
	}
	return displayed
}

func displayRegion(region string) string {
	return translatedOrOriginal(region, regionTranslations)
}

func displayFundingRound(round string) string {
	trimmed := strings.TrimSpace(round)
	if translated, ok := fundingRoundTranslations[strings.ToLower(trimmed)]; ok {
		return translated
	}
	fields := strings.Fields(strings.ToLower(trimmed))
	if len(fields) == 2 && fields[0] == "series" && len(fields[1]) == 1 && fields[1][0] >= 'a' && fields[1][0] <= 'z' {
		return "раунд " + strings.ToUpper(fields[1])
	}
	return trimmed
}

func translatedOrOriginal(value string, translations map[string]string) string {
	trimmed := strings.TrimSpace(value)
	if translated, ok := translations[strings.ToLower(trimmed)]; ok {
		return translated
	}
	return trimmed
}
