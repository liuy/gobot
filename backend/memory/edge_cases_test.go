package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// P0-1: updateKeywords 纯函数测试 - slice 扩容不影响正确性
func TestUpdateKeywords_SliceRealloc(t *testing.T) {
	hotData := &HotMemoryData{}
	now := time.Now()

	// 触发多次 append，验证 slice 扩容后 map index 仍然正确
	for i := 0; i < 100; i++ {
		words := []string{fmt.Sprintf("keyword%d", i), "test"}
		updateKeywords(hotData, words, now)
	}

	// 验证结果
	if len(hotData.RecentKeywords) == 0 {
		t.Fatal("Expected some keywords, got none")
	}

	// "test" 应该出现一次，count=100
	for _, kw := range hotData.RecentKeywords {
		if kw.Word == "test" {
			if kw.Count != 100 {
				t.Errorf("'test' count should be 100, got %d", kw.Count)
			}
			t.Logf("'test' count: %d (expected 100)", kw.Count)
			break
		}
	}
	t.Logf("Total keywords: %d", len(hotData.RecentKeywords))
}

// P0-3: Append() 输入校验
func TestAppend_InvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		msg     Message
		wantErr bool
	}{
		{
			name: "empty ID",
			msg:  Message{ID: "", Content: "test", Timestamp: time.Now()},
			wantErr: true,
		},
		{
			name: "empty Content",
			msg:  Message{ID: "1", Content: "", Timestamp: time.Now()},
			wantErr: true,
		},
		{
			name: "whitespace Content",
			msg:  Message{ID: "1", Content: "   ", Timestamp: time.Now()},
			wantErr: true,
		},
		{
			name: "zero Timestamp",
			msg:  Message{ID: "1", Content: "test", Timestamp: time.Time{}},
			wantErr: true,
		},
		{
			name: "valid message",
			msg:  Message{ID: "1", Content: "test", Timestamp: time.Now()},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cache, _ := NewMemoryCache(tmpDir)
			defer cache.Close()

			err := cache.Append(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Append() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// P0-4: Search() 空白 query
func TestSearch_WhitespaceQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"empty", "", true},
		{"spaces", "   ", true},
		{"tabs", "\t\t", true},
		{"newlines", "\n\n", true},
		{"mixed", " \t\n ", true},
		{"valid", "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cache, _ := NewMemoryCache(tmpDir)
			defer cache.Close()

			_, err := cache.Search(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Search(%q) error = %v, wantErr %v", tt.query, err, tt.wantErr)
			}
		})
	}
}

// P0-6: Close() 后调用方法
func TestClose_AfterClose(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)

	// Close
	if err := cache.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Close again (should be idempotent)
	if err := cache.Close(); err != nil {
		t.Errorf("Second Close should not error, got: %v", err)
	}

	// Append after close
	msg := Message{ID: "1", Content: "test", Timestamp: time.Now()}
	err := cache.Append(msg)
	if err == nil {
		t.Error("Append after Close should error")
	}
	t.Logf("Append after Close: %v (expected error)", err)

	// Search after close
	_, err = cache.Search("test")
	if err == nil {
		t.Error("Search after Close should error")
	}
	t.Logf("Search after Close: %v (expected error)", err)
}

// P1: TopicSummaries 未实现
func TestTopicSummaries_NotImplemented(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	// Append some messages
	for i := 0; i < 5; i++ {
		cache.Append(Message{
			ID:        string(rune('1' + i)),
			Content:   "test topic keyword",
			Timestamp: time.Now(),
		})
	}

	hot, _ := cache.GetHot()
	if hot == nil {
		t.Fatal("GetHot returned nil")
	}

	// TopicSummaries 应该为空（未实现）
	if len(hot.TopicSummaries) > 0 {
		t.Logf("TopicSummaries has %d items (unexpected)", len(hot.TopicSummaries))
	} else {
		t.Log("TopicSummaries is empty (expected - not implemented yet)")
	}
}

// 边界条件：超长内容
func TestAppend_VeryLongContent(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	// 100KB 内容
	longContent := strings.Repeat("test ", 20000)
	msg := Message{
		ID:        "1",
		Content:   longContent,
		Timestamp: time.Now(),
	}

	err := cache.Append(msg)
	if err != nil {
		t.Errorf("Append with long content failed: %v", err)
	}

	// 搜索应该能找到
	results, err := cache.Search("test")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search should find the message with long content")
	}
}

// 边界条件：并发 Append 更多 goroutines
func TestAppend_HighConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	const goroutines = 10
	const messagesPerGoroutine = 10

	done := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			for i := 0; i < messagesPerGoroutine; i++ {
				msg := Message{
					ID:        fmt.Sprintf("g%d-m%d", gid, i),
					Content:   "concurrent test",
					Timestamp: time.Now(),
				}
				if err := cache.Append(msg); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}(g)
	}

	errors := 0
	for g := 0; g < goroutines; g++ {
		if err := <-done; err != nil {
			t.Errorf("Goroutine failed: %v", err)
			errors++
		}
	}

	if errors > 0 {
		t.Fatalf("%d goroutines failed", errors)
	}

	// 验证搜索能找到（FTS5 结果数量取决于分词）
	results, err := cache.Search("concurrent")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search should find at least one message")
	}
	t.Logf("Found %d messages via FTS5 search", len(results))
}

// P0-1: hot.go map 指针问题 - append 后扩容导致指针失效
func TestUpdateKeywords_MapPointerRealloc(t *testing.T) {
	// 这个测试验证 map 存储的是 slice 元素指针，而不是 index
	// 如果 slice 扩容，指针会指向旧数组，导致数据不一致
	
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	// 先添加足够多的不同关键词，触发 RecentKeywords slice 扩容
	for i := 0; i < 50; i++ {
		// 每次用不同的关键词，强制 append
		msg := Message{
			ID:        fmt.Sprintf("realloc-%d", i),
			Content:   fmt.Sprintf("keyword%d test%d word%d", i, i, i),
			Timestamp: time.Now(),
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}

	// 等待 hot worker 处理
	time.Sleep(600 * time.Millisecond)

	hot, err := cache.GetHot()
	if err != nil {
		t.Fatalf("GetHot failed: %v", err)
	}

	// 如果 map 指针失效，Count 可能不正确
	for i, kw := range hot.RecentKeywords {
		if kw.Count <= 0 {
			t.Errorf("Keyword %d (%q) has invalid count %d", i, kw.Word, kw.Count)
		}
		t.Logf("Keyword %d: %q (count=%d)", i, kw.Word, kw.Count)
	}
}

// P0-1: updateTopics 同样的问题
// updateTopics 纯函数测试 - slice 扩容不影响正确性
func TestUpdateTopics_MapPointerRealloc(t *testing.T) {
	hotData := &HotMemoryData{}
	now := time.Now()

	// 添加大量 keywords，触发 topics 更新
	for i := 0; i < 50; i++ {
		// 添加一个高频词（count >= 3 会成为 topic）
		words := []string{fmt.Sprintf("topic%d", i), "commontopic", "commontopic", "commontopic"}
		for _, w := range words {
			updateKeywords(hotData, []string{w}, now)
		}
	}

	// 更新 topics
	updateTopics(hotData, now)

	// 验证 commontopic 成为 topic（count >= 3）
	found := false
	for _, topic := range hotData.ActiveTopics {
		if topic.Name == "commontopic" {
			found = true
			if topic.Count < 3 {
				t.Errorf("commontopic count should >= 3, got %d", topic.Count)
			}
			t.Logf("commontopic: count=%d", topic.Count)
			break
		}
	}

	if !found {
		t.Error("Expected 'commontopic' to become a topic (count >= 3)")
	}
	t.Logf("Total topics: %d", len(hotData.ActiveTopics))
}

// P0-5: rows.Err() 未检查
// 这个测试比较难直接验证，因为需要 mock DB 错误
// 但我们可以验证正常情况下 GetRecent() 的行为
func TestGetRecent_RowsError(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	// 添加超过 20 条消息
	for i := 0; i < 30; i++ {
		msg := Message{
			ID:        fmt.Sprintf("recent-%d", i),
			Content:   fmt.Sprintf("message %d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}

	recent := cache.GetRecent()
	
	// 应该返回最近 20 条
	if len(recent) != 20 {
		t.Errorf("Expected 20 recent messages, got %d", len(recent))
	}

	// 应该是按时间倒序（最新的在前）
	if len(recent) > 1 {
		// recent[0] 应该比 recent[1] 更新
		if recent[0].Timestamp.Before(recent[1].Timestamp) {
			t.Error("Recent messages should be in descending order (newest first)")
		}
	}
}

// updateKeywords 纯函数测试 - 重复更新同一个词
func TestUpdateKeywords_RepeatedUpdate(t *testing.T) {
	hotData := &HotMemoryData{}
	now := time.Now()

	// 第一次添加
	updateKeywords(hotData, []string{"testword"}, now)

	// 验证初始 count
	var initialCount int
	for _, kw := range hotData.RecentKeywords {
		if kw.Word == "testword" {
			initialCount = kw.Count
			break
		}
	}
	if initialCount != 1 {
		t.Fatalf("Initial count should be 1, got %d", initialCount)
	}

	// 添加大量其他 keywords，触发扩容
	for i := 0; i < 100; i++ {
		updateKeywords(hotData, []string{fmt.Sprintf("unique%d", i)}, now)
	}

	// 再次更新同一个 keyword
	updateKeywords(hotData, []string{"testword"}, now)

	// 验证 count 增加了
	for _, kw := range hotData.RecentKeywords {
		if kw.Word == "testword" {
			if kw.Count != 2 {
				t.Errorf("testword count should be 2, got %d", kw.Count)
			}
			t.Logf("testword: initial=%d, final=%d", initialCount, kw.Count)
			break
		}
	}
}

// 精准测试：同一批 words 中有重复词
func TestUpdateKeywords_DuplicateWords(t *testing.T) {
	hotData := &HotMemoryData{}
	now := time.Now()

	// 模拟 extractKeywords 返回重复词
	// 正确行为：重复词只计数一次（每条消息同一词只记一次）
	words := []string{"test", "test", "test", "unique"}
	
	updateKeywords(hotData, words, now)

	// 检查结果
	if len(hotData.RecentKeywords) != 2 {
		t.Errorf("Expected 2 keywords, got %d", len(hotData.RecentKeywords))
		for i, kw := range hotData.RecentKeywords {
			t.Logf("  Keyword %d: %q (count=%d)", i, kw.Word, kw.Count)
		}
	}

	// "test" 应该只出现一次，count=1（每条消息计数一次）
	for _, kw := range hotData.RecentKeywords {
		if kw.Word == "test" && kw.Count != 1 {
			t.Errorf("test count should be 1 (once per message), got %d", kw.Count)
		}
	}
}

// P0-1: GetLongterm() 读锁下写字段 - race condition
func TestGetLongterm_RaceCondition(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("NewMemoryCache failed: %v", err)
	}
	defer cache.Close()

	// 创建 longterm.md 文件
	longtermPath := filepath.Join(tmpDir, "memory", "longterm.md")
	if err := os.MkdirAll(filepath.Dir(longtermPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(longtermPath, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// 并发调用 GetLongterm
	const goroutines = 50
	done := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := cache.GetLongterm()
			done <- err
		}()
	}

	for i := 0; i < goroutines; i++ {
		if err := <-done; err != nil {
			t.Errorf("GetLongterm failed: %v", err)
		}
	}
}
