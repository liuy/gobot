package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

func loadHot(filePath string) (*HotMemoryData, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &HotMemoryData{}, nil
		}
		return nil, err
	}

	var hotData HotMemoryData
	if err := json.Unmarshal(data, &hotData); err != nil {
		return nil, err
	}

	return &hotData, nil
}

func saveHot(filePath string, data *HotMemoryData) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, filePath)
}

func updateKeywords(hotData *HotMemoryData, words []string, now time.Time) {
	// Use index instead of pointer to avoid issues with slice reallocation
	keywordIndex := make(map[string]int)
	for i, kw := range hotData.RecentKeywords {
		keywordIndex[kw.Word] = i
	}

	// Track which words we've seen in this batch
	seenInBatch := make(map[string]bool)

	for _, word := range words {
		// Skip if already processed in this batch
		if seenInBatch[word] {
			continue
		}
		seenInBatch[word] = true

		if idx, exists := keywordIndex[word]; exists {
			// Update existing keyword
			hotData.RecentKeywords[idx].Count++
			hotData.RecentKeywords[idx].LastActive = now
		} else {
			// Add new keyword
			hotData.RecentKeywords = append(hotData.RecentKeywords, Keyword{
				Word:       word,
				LastActive: now,
				Count:      1,
			})
			// Update index for new element
			keywordIndex[word] = len(hotData.RecentKeywords) - 1
		}
	}
}

func updateTopics(hotData *HotMemoryData, now time.Time) {
	// Use index instead of pointer to avoid issues with slice reallocation
	topicIndex := make(map[string]int)
	for i, topic := range hotData.ActiveTopics {
		topicIndex[topic.Name] = i
	}

	for _, kw := range hotData.RecentKeywords {
		if kw.Count >= 3 {
			if idx, exists := topicIndex[kw.Word]; exists {
				// Update existing topic
				hotData.ActiveTopics[idx].Count = kw.Count
				hotData.ActiveTopics[idx].LastActive = now
			} else {
				// Add new topic
				hotData.ActiveTopics = append(hotData.ActiveTopics, Topic{
					Name:       kw.Word,
					LastActive: now,
					Count:      kw.Count,
				})
				// Update index for new element
				topicIndex[kw.Word] = len(hotData.ActiveTopics) - 1
			}
		}
	}
}

func cleanupExpired(hotData *HotMemoryData, now time.Time) {
	ttl := 7 * 24 * time.Hour

	var validKeywords []Keyword
	for _, kw := range hotData.RecentKeywords {
		if now.Sub(kw.LastActive) <= ttl {
			validKeywords = append(validKeywords, kw)
		}
	}
	hotData.RecentKeywords = validKeywords

	var validTopics []Topic
	for _, topic := range hotData.ActiveTopics {
		if now.Sub(topic.LastActive) <= ttl {
			validTopics = append(validTopics, topic)
		}
	}
	hotData.ActiveTopics = validTopics

	var validSummaries []TopicSummary
	for _, summary := range hotData.TopicSummaries {
		if now.Sub(summary.LastActive) <= ttl {
			validSummaries = append(validSummaries, summary)
		}
	}
	hotData.TopicSummaries = validSummaries
}

func extractKeywords(content string) []string {
	// Check if content is primarily Chinese
	chineseCount := 0
	totalRunes := 0
	for _, r := range content {
		totalRunes++
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}

	text := content
	// Only use jieba if > 30% Chinese characters
	if chineseCount > totalRunes/3 {
		text = tokenizeChinese(content)
	}

	var builder strings.Builder
	builder.Grow(len(text))

	for _, r := range text {
		if !isPunctuation(r) {
			builder.WriteRune(unicode.ToLower(r))
		} else {
			builder.WriteRune(' ')
		}
	}

	words := strings.Fields(builder.String())

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true,
		"would": true, "could": true, "should": true, "may": true,
		"might": true, "must": true, "shall": true, "can": true,
		"need": true, "dare": true, "ought": true, "used": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true,
		"as": true, "into": true, "through": true, "during": true,
		"before": true, "after": true, "above": true, "below": true,
		"between": true, "under": true, "again": true, "further": true,
		"then": true, "once": true, "here": true, "there": true,
		"when": true, "where": true, "why": true, "how": true,
		"all": true, "each": true, "few": true, "more": true,
		"most": true, "other": true, "some": true, "such": true,
		"no": true, "nor": true, "not": true, "only": true,
		"own": true, "same": true, "so": true, "than": true,
		"too": true, "very": true, "just": true, "and": true,
		"but": true, "if": true, "or": true, "because": true,
		"until": true, "while": true, "although": true, "though": true,
		"over": true, "about": true, "against": true, "along": true,
		"的": true, "了": true, "和": true, "是": true, "在": true,
		"有": true, "我": true, "他": true, "这": true, "为": true,
		"之": true, "以": true, "及": true, "与": true, "或": true,
		"也": true, "但": true, "而": true, "就": true, "都": true,
	}

	var keywords []string
	for _, word := range words {
		if len(word) > 1 && !stopWords[word] {
			keywords = append(keywords, word)
			if len(keywords) >= 10 {
				break
			}
		}
	}

	return keywords
}

func isPunctuation(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r) || r == '\n' || r == '\t'
}
