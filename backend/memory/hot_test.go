package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadHot_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nonexistent.json")

	data, err := loadHot(filePath)

	if err != nil {
		t.Errorf("Expected no error for missing file, got: %v", err)
	}
	if data == nil {
		t.Fatal("Expected non-nil HotMemoryData")
	}
	if len(data.ActiveTopics) != 0 || len(data.RecentKeywords) != 0 {
		t.Errorf("Expected empty HotMemoryData, got: %+v", data)
	}
}

func TestLoadHot_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "hot.json")

	expectedData := &HotMemoryData{
		ActiveTopics: []Topic{
			{Name: "project", Count: 5, LastActive: time.Now()},
		},
		RecentKeywords: []Keyword{
			{Word: "code", Count: 10, LastActive: time.Now()},
		},
		LastUpdated: time.Now(),
	}

	jsonData, err := json.MarshalIndent(expectedData, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	data, err := loadHot(filePath)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(data.ActiveTopics) != 1 || data.ActiveTopics[0].Name != "project" {
		t.Errorf("Expected project topic, got: %+v", data.ActiveTopics)
	}
}

func TestLoadHot_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(filePath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := loadHot(filePath)

	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestSaveHot_CreateFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "subdir", "hot.json")

	data := &HotMemoryData{
		ActiveTopics: []Topic{
			{Name: "test", Count: 3, LastActive: time.Now()},
		},
		LastUpdated: time.Now(),
	}

	err := saveHot(filePath, data)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	loaded, err := loadHot(filePath)
	if err != nil {
		t.Fatalf("Failed to load saved file: %v", err)
	}
	if len(loaded.ActiveTopics) != 1 || loaded.ActiveTopics[0].Name != "test" {
		t.Errorf("Expected test topic, got: %+v", loaded.ActiveTopics)
	}
}

func TestUpdateKeywords_NewKeywords(t *testing.T) {
	hotData := &HotMemoryData{}
	words := []string{"golang", "testing", "memory"}

	updateKeywords(hotData, words, time.Now())

	if len(hotData.RecentKeywords) != 3 {
		t.Errorf("Expected 3 keywords, got %d", len(hotData.RecentKeywords))
	}

	keywordMap := make(map[string]bool)
	for _, kw := range hotData.RecentKeywords {
		keywordMap[kw.Word] = true
		if kw.Count != 1 {
			t.Errorf("Expected count 1 for %s, got %d", kw.Word, kw.Count)
		}
	}

	for _, word := range words {
		if !keywordMap[word] {
			t.Errorf("Expected keyword %s not found", word)
		}
	}
}

func TestUpdateKeywords_ExistingKeywords(t *testing.T) {
	now := time.Now()
	hotData := &HotMemoryData{
		RecentKeywords: []Keyword{
			{Word: "golang", Count: 2, LastActive: now.Add(-1 * time.Hour)},
		},
	}

	words := []string{"golang", "testing"}
	updateKeywords(hotData, words, time.Now())

	if len(hotData.RecentKeywords) != 2 {
		t.Errorf("Expected 2 keywords, got %d", len(hotData.RecentKeywords))
	}

	var golangKw *Keyword
	for i := range hotData.RecentKeywords {
		if hotData.RecentKeywords[i].Word == "golang" {
			golangKw = &hotData.RecentKeywords[i]
			break
		}
	}

	if golangKw == nil {
		t.Fatal("golang keyword not found")
	}
	if golangKw.Count != 3 {
		t.Errorf("Expected count 3, got %d", golangKw.Count)
	}
}

func TestUpdateTopics_HighFrequencyKeywords(t *testing.T) {
	now := time.Now()
	hotData := &HotMemoryData{
		RecentKeywords: []Keyword{
			{Word: "project", Count: 5, LastActive: now},
			{Word: "code", Count: 3, LastActive: now},
			{Word: "test", Count: 2, LastActive: now},
		},
	}

	updateTopics(hotData, time.Now())

	if len(hotData.ActiveTopics) != 2 {
		t.Errorf("Expected 2 topics (count >= 3), got %d", len(hotData.ActiveTopics))
	}

	topicMap := make(map[string]bool)
	for _, topic := range hotData.ActiveTopics {
		topicMap[topic.Name] = true
	}

	if !topicMap["project"] || !topicMap["code"] {
		t.Errorf("Expected project and code topics, got: %+v", hotData.ActiveTopics)
	}
}

func TestCleanupExpired_RemoveOldEntries(t *testing.T) {
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	recentTime := time.Now()

	hotData := &HotMemoryData{
		RecentKeywords: []Keyword{
			{Word: "old", Count: 1, LastActive: oldTime},
			{Word: "recent", Count: 1, LastActive: recentTime},
		},
		ActiveTopics: []Topic{
			{Name: "oldtopic", Count: 1, LastActive: oldTime},
			{Name: "newtopic", Count: 1, LastActive: recentTime},
		},
		TopicSummaries: []TopicSummary{
			{Topic: "oldsummary", LastActive: oldTime},
			{Topic: "newsummary", LastActive: recentTime},
		},
	}

	cleanupExpired(hotData, time.Now())

	if len(hotData.RecentKeywords) != 1 || hotData.RecentKeywords[0].Word != "recent" {
		t.Errorf("Expected only recent keyword, got: %+v", hotData.RecentKeywords)
	}
	if len(hotData.ActiveTopics) != 1 || hotData.ActiveTopics[0].Name != "newtopic" {
		t.Errorf("Expected only newtopic, got: %+v", hotData.ActiveTopics)
	}
	if len(hotData.TopicSummaries) != 1 || hotData.TopicSummaries[0].Topic != "newsummary" {
		t.Errorf("Expected only newsummary, got: %+v", hotData.TopicSummaries)
	}
}

func TestExtractKeywords_BasicExtraction(t *testing.T) {
	content := "Hello, World! 今天天气怎么样？"
	keywords := extractKeywords(content)

	if len(keywords) == 0 {
		t.Error("Expected non-empty keywords")
	}

	keywordMap := make(map[string]bool)
	for _, kw := range keywords {
		keywordMap[kw] = true
	}

	if !keywordMap["hello"] {
		t.Error("Expected keyword 'hello'")
	}
	if !keywordMap["world"] {
		t.Error("Expected keyword 'world'")
	}
}

func TestExtractKeywords_StopWords(t *testing.T) {
	content := "The quick brown fox jumps over the lazy dog"
	keywords := extractKeywords(content)

	for _, kw := range keywords {
		if kw == "the" || kw == "over" {
			t.Errorf("Stop word '%s' should be filtered", kw)
		}
	}
}

func TestExtractKeywords_MaxLimit(t *testing.T) {
	words := make([]string, 20)
	for i := 0; i < 20; i++ {
		words[i] = "word" + string(rune('a'+i))
	}
	content := ""
	for _, w := range words {
		content += w + " "
	}

	keywords := extractKeywords(content)

	if len(keywords) > 10 {
		t.Errorf("Expected max 10 keywords, got %d", len(keywords))
	}
}

func TestExtractKeywords_SingleCharacterFilter(t *testing.T) {
	content := "a b c d e f testing"
	keywords := extractKeywords(content)

	for _, kw := range keywords {
		if len(kw) == 1 {
			t.Errorf("Single character '%s' should be filtered", kw)
		}
	}
}
