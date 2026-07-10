package digest

import (
	"sort"
	"strings"

	"github.com/seregatheone/DailyStartupsBot/backend/internal/ingestion"
	"github.com/seregatheone/DailyStartupsBot/backend/internal/storage"
)

type identifiedSignal struct {
	signal   storage.StartupSignal
	identity ingestion.SignalIdentity
}

func (generator Generator) Generate(request Request) Digest {
	items := generator.groupSignals(request.Signals, request.Preferences, request.DigestDate)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		leftName := strings.ToLower(items[i].StartupName)
		rightName := strings.ToLower(items[j].StartupName)
		if leftName != rightName {
			return leftName < rightName
		}
		return items[i].identity < items[j].identity
	})

	limit := request.Preferences.MaxItems
	if limit <= 0 {
		limit = DefaultItemLimit
	}
	if limit > MaximumItemLimit {
		limit = MaximumItemLimit
	}
	if len(items) > limit {
		items = items[:limit]
	}

	return Digest{
		Date:     request.DigestDate,
		Timezone: request.Timezone,
		Items:    items,
		Empty:    len(items) == 0,
	}
}

func (generator Generator) groupSignals(
	signals []storage.StartupSignal,
	preferences storage.Preferences,
	digestDate string,
) []Item {
	identified := make([]identifiedSignal, 0, len(signals))
	for _, signal := range signals {
		identified = append(identified, identifiedSignal{
			signal:   signal,
			identity: ingestion.SignalIdentityForScope(signal, digestDate),
		})
	}
	sort.SliceStable(identified, func(left, right int) bool {
		if !identified[left].signal.PublishedAt.Equal(identified[right].signal.PublishedAt) {
			return identified[left].signal.PublishedAt.After(identified[right].signal.PublishedAt)
		}
		return identified[left].signal.ID < identified[right].signal.ID
	})

	groups := newUnionFind(len(identified))
	canonicalOwner := make(map[string]int)
	for index, item := range identified {
		if item.identity.CanonicalURL == "" {
			continue
		}
		if owner, ok := canonicalOwner[item.identity.CanonicalURL]; ok {
			groups.union(index, owner)
		} else {
			canonicalOwner[item.identity.CanonicalURL] = index
		}
	}

	aliases := newUnionFind(len(identified))
	aliasOwner := make(map[string]int)
	for index, item := range identified {
		keys := aliasKeys(item.identity)
		for _, key := range keys {
			if owner, ok := aliasOwner[key]; ok {
				aliases.union(index, owner)
			} else {
				aliasOwner[key] = index
			}
		}
	}

	aliasMembers := make(map[int][]int)
	aliasAnchors := make(map[int]map[string]struct{})
	for index, item := range identified {
		root := aliases.find(index)
		aliasMembers[root] = append(aliasMembers[root], index)
		if item.identity.CanonicalURL != "" {
			if aliasAnchors[root] == nil {
				aliasAnchors[root] = make(map[string]struct{})
			}
			aliasAnchors[root][item.identity.CanonicalURL] = struct{}{}
		}
	}
	for root, members := range aliasMembers {
		if len(aliasAnchors[root]) > 1 || len(members) < 2 {
			continue
		}
		for _, member := range members[1:] {
			groups.union(members[0], member)
		}
	}

	identities := make(map[int]string)
	for index, item := range identified {
		root := groups.find(index)
		identities[root] = minimumIdentity(identities[root], item)
	}
	byRoot := make(map[int]*Item)
	order := make([]int, 0)
	for index, item := range identified {
		root := groups.find(index)
		grouped, ok := byRoot[root]
		if !ok {
			grouped = &Item{identity: identities[root]}
			byRoot[root] = grouped
			order = append(order, root)
		}
		mergeSignal(grouped, item.signal)
	}

	items := make([]Item, 0, len(order))
	for _, root := range order {
		item := *byRoot[root]
		sortMergedItem(&item)
		item.Score = generator.score(item, preferences)
		items = append(items, item)
	}
	return items
}

func aliasKeys(identity ingestion.SignalIdentity) []string {
	keys := make([]string, 0, 3)
	if identity.ExactName != "" {
		keys = append(keys, identity.ExactName)
	}
	if identity.SuffixName != "" {
		if identity.SourceEvent != "" {
			keys = append(keys, identity.SuffixName+":source:"+identity.SourceEvent)
		}
		if identity.FundingFingerprint != "" {
			keys = append(keys, identity.SuffixName+":funding:"+identity.FundingFingerprint)
		}
	}
	sort.Strings(keys)
	return keys
}

func minimumIdentity(current string, item identifiedSignal) string {
	candidates := []string{
		item.identity.CanonicalURL,
		item.identity.ExactName,
		item.identity.SuffixName,
		"signal:" + item.signal.ID,
	}
	for _, candidate := range candidates {
		if candidate != "" && (current == "" || identityRank(candidate) < identityRank(current) ||
			(identityRank(candidate) == identityRank(current) && candidate < current)) {
			current = candidate
		}
	}
	return current
}

func identityRank(value string) int {
	switch {
	case strings.HasPrefix(value, "url:"):
		return 0
	case strings.HasPrefix(value, "exact:"):
		return 1
	case strings.HasPrefix(value, "suffix:"):
		return 2
	default:
		return 3
	}
}

func mergeSignal(item *Item, signal storage.StartupSignal) {
	payload := parsePayload(signal.RawPayload)
	item.Signals = append(item.Signals, signal)
	item.Sources = mergeSource(item.Sources, signal)
	if item.StartupName == "" {
		item.StartupName = signal.StartupName
	}
	if item.Description == "" && signal.Description != "" {
		item.Description = signal.Description
	}
	if item.SignalType == "" || signalWeight(signal.SignalType) > signalWeight(item.SignalType) {
		item.SignalType = signal.SignalType
	}
	if item.Region == "" && signal.Region != "" {
		item.Region = signal.Region
	}
	if signal.PublishedAt.After(item.PublishedAt) {
		item.PublishedAt = signal.PublishedAt
	}
	item.Categories = mergeStrings(item.Categories, payload.Categories)
	item.Funding = mergeFunding(item.Funding, payload.Funding)
}

func mergeSource(sources []SourceAttribution, signal storage.StartupSignal) []SourceAttribution {
	for _, source := range sources {
		if source.SourceID == signal.SourceID && source.SourceURL == signal.SourceURL {
			return sources
		}
	}
	return append(sources, SourceAttribution{SourceID: signal.SourceID, SourceURL: signal.SourceURL})
}

func mergeStrings(existing, incoming []string) []string {
	seen := map[string]bool{}
	for _, value := range existing {
		seen[strings.ToLower(strings.TrimSpace(value))] = true
	}
	for _, value := range incoming {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		existing = append(existing, value)
		seen[key] = true
	}
	return existing
}

func mergeFunding(existing, incoming FundingInfo) FundingInfo {
	existingInvestors := append([]string(nil), existing.Investors...)
	if fundingCompleteness(incoming) > fundingCompleteness(existing) {
		compatible := fundingCompatible(existing, incoming)
		existing = FundingInfo{
			Round: incoming.Round, Amount: incoming.Amount, Currency: incoming.Currency,
			Investors: append([]string(nil), incoming.Investors...),
		}
		if compatible {
			existing.Investors = mergeStrings(existing.Investors, existingInvestors)
		}
	} else if fundingCompatible(existing, incoming) {
		existing.Investors = mergeStrings(existing.Investors, incoming.Investors)
	}
	return existing
}

func fundingCompleteness(funding FundingInfo) int {
	score := 0
	if funding.Amount != "" && funding.Currency != "" {
		score += 2
	}
	if funding.Round != "" {
		score++
	}
	return score
}

func fundingCompatible(left, right FundingInfo) bool {
	if left.Amount != "" && right.Amount != "" &&
		(!strings.EqualFold(left.Amount, right.Amount) || !strings.EqualFold(left.Currency, right.Currency)) {
		return false
	}
	if left.Round != "" && right.Round != "" && !strings.EqualFold(left.Round, right.Round) {
		return false
	}
	return true
}

func sortMergedItem(item *Item) {
	sort.Slice(item.Sources, func(left, right int) bool {
		if item.Sources[left].SourceID == item.Sources[right].SourceID {
			return item.Sources[left].SourceURL < item.Sources[right].SourceURL
		}
		return item.Sources[left].SourceID < item.Sources[right].SourceID
	})
	sort.Slice(item.Categories, func(left, right int) bool {
		return strings.ToLower(item.Categories[left]) < strings.ToLower(item.Categories[right])
	})
	sort.Slice(item.Funding.Investors, func(left, right int) bool {
		return strings.ToLower(item.Funding.Investors[left]) < strings.ToLower(item.Funding.Investors[right])
	})
}

type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(size int) *unionFind {
	result := &unionFind{parent: make([]int, size), rank: make([]int, size)}
	for index := range result.parent {
		result.parent[index] = index
	}
	return result
}

func (set *unionFind) find(value int) int {
	if set.parent[value] != value {
		set.parent[value] = set.find(set.parent[value])
	}
	return set.parent[value]
}

func (set *unionFind) union(left, right int) {
	leftRoot := set.find(left)
	rightRoot := set.find(right)
	if leftRoot == rightRoot {
		return
	}
	if set.rank[leftRoot] < set.rank[rightRoot] {
		leftRoot, rightRoot = rightRoot, leftRoot
	}
	set.parent[rightRoot] = leftRoot
	if set.rank[leftRoot] == set.rank[rightRoot] {
		set.rank[leftRoot]++
	}
}
