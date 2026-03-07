package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gobot/log"
)

const longtermCheckTTL = 30 * time.Second

type MemoryCache struct {
	dataDir string

	longterm          string
	longtermMod       time.Time
	longtermMu        sync.RWMutex
	longtermLastCheck time.Time // zero value intentionally triggers first reload

	hotData   *HotMemoryData
	hotDataMu sync.RWMutex

	recentMessages []Message
	recentMu       sync.RWMutex

	coldDB *sql.DB

	hotUpdateChan chan Message
	hotWorkerWg   sync.WaitGroup
	stopChan      chan struct{}
	closeOnce     sync.Once
	closeErr      error

	nowFunc func() time.Time
}

func NewMemoryCache(dataDir string) (*MemoryCache, error) {
	memoryDir := filepath.Join(dataDir, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(memoryDir, "cold.db")
	coldDB, err := initColdDB(dbPath)
	if err != nil {
		return nil, err
	}

	cache := &MemoryCache{
		dataDir:        dataDir,
		coldDB:         coldDB,
		hotUpdateChan:  make(chan Message, 100),
		stopChan:       make(chan struct{}),
		nowFunc:        time.Now,
		hotData:        &HotMemoryData{},
		recentMessages: []Message{},
	}

	longtermPath := filepath.Join(memoryDir, "longterm.md")
	longtermContent, err := loadLongterm(longtermPath)
	if err != nil {
		coldDB.Close()
		return nil, err
	}
	cache.longterm = longtermContent

	if info, err := os.Stat(longtermPath); err == nil {
		cache.longtermMod = info.ModTime()
	}

	hotPath := filepath.Join(memoryDir, "hot.json")
	hotData, err := loadHot(hotPath)
	if err != nil {
		coldDB.Close()
		return nil, err
	}
	cache.hotData = hotData

	recent, err := getRecentMessages(coldDB, 20)
	if err != nil {
		coldDB.Close()
		return nil, err
	}
	cache.recentMessages = recent

	cache.startHotWorker()

	return cache, nil
}

func (c *MemoryCache) Close() error {
	c.closeOnce.Do(func() {
		close(c.stopChan)

		done := make(chan struct{})
		go func() {
			c.hotWorkerWg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}

		c.closeErr = c.coldDB.Close()
	})

	return c.closeErr
}

func (c *MemoryCache) GetLongterm() (string, error) {
	// Fast path: return cached content if within TTL
	// Note: We cache even empty string to avoid repeated os.Stat calls
	now := c.nowFunc()
	c.longtermMu.RLock()
	if now.Sub(c.longtermLastCheck) < longtermCheckTTL {
		result := c.longterm
		c.longtermMu.RUnlock()
		return result, nil
	}
	c.longtermMu.RUnlock()

	// Slow path: check file modification time
	memoryDir := filepath.Join(c.dataDir, "memory")
	longtermPath := filepath.Join(memoryDir, "longterm.md")

	info, err := os.Stat(longtermPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.longtermMu.Lock()
			c.longterm = ""
			c.longtermMod = time.Time{}
			c.longtermLastCheck = now
			c.longtermMu.Unlock()
			return "", nil
		}
		return "", err
	}

	// Check if we need to reload (based on file modification time only)
	// Note: Empty content is a valid cached value, don't trigger reload based on it
	c.longtermMu.RLock()
	shouldReload := !info.ModTime().Equal(c.longtermMod)
	c.longtermMu.RUnlock()

	if !shouldReload {
		// Cache hit - update last check time under write lock
		c.longtermMu.Lock()
		c.longtermLastCheck = now
		result := c.longterm
		c.longtermMu.Unlock()
		return result, nil
	}

	// Cache miss - load from disk
	content, err := loadLongterm(longtermPath)
	if err != nil {
		return "", err
	}

	c.longtermMu.Lock()
	c.longterm = content
	c.longtermMod = info.ModTime()
	c.longtermLastCheck = now
	c.longtermMu.Unlock()

	return content, nil
}

// GetHot returns the hot memory data.
// Note: Returns internal pointer for performance. Callers must NOT modify the returned data.
// This is safe because:
// 1. Current usage is 100% internal (ContextBuilder) which only reads
// 2. All modifications go through Append() → hot worker → updates c.hotData
// 3. No external process modifies hot.json
func (c *MemoryCache) GetHot() (*HotMemoryData, error) {
	c.hotDataMu.RLock()
	defer c.hotDataMu.RUnlock()

	if c.hotData == nil {
		return &HotMemoryData{}, nil
	}
	return c.hotData, nil
}

// GetRecent returns the recent messages.
// Note: Returns internal slice for performance. Callers must NOT modify the returned slice.
// Safe operations: ctx.Recent[:n] (only modifies slice header)
// Unsafe operations: append(), modifying elements
// This is safe because all callers are internal (ContextBuilder) which only does slice truncation.
func (c *MemoryCache) GetRecent() []Message {
	c.recentMu.RLock()
	defer c.recentMu.RUnlock()
	return c.recentMessages
}

func (c *MemoryCache) Append(msg Message) error {
	// Validate input
	if strings.TrimSpace(msg.ID) == "" {
		return fmt.Errorf("message ID cannot be empty")
	}
	if strings.TrimSpace(msg.Content) == "" {
		return fmt.Errorf("message content cannot be empty")
	}
	if msg.Timestamp.IsZero() {
		return fmt.Errorf("message timestamp cannot be zero")
	}

	if err := insertMessage(c.coldDB, msg); err != nil {
		return err
	}

	c.recentMu.Lock()
	c.recentMessages = append([]Message{msg}, c.recentMessages...)
	if len(c.recentMessages) > 20 {
		c.recentMessages = c.recentMessages[:20]
	}
	c.recentMu.Unlock()

	c.UpdateHotAsync(msg)

	return nil
}

func (c *MemoryCache) startHotWorker() {
	c.hotWorkerWg.Add(1)
	go func() {
		defer c.hotWorkerWg.Done()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var pendingMessages []Message

		for {
			select {
			case <-c.stopChan:
				for {
					select {
					case msg := <-c.hotUpdateChan:
						pendingMessages = append(pendingMessages, msg)
					default:
						goto done
					}
				}
			done:
				if len(pendingMessages) > 0 {
					c.processHotUpdate(pendingMessages)
				}
				return

			case msg := <-c.hotUpdateChan:
				pendingMessages = append(pendingMessages, msg)

			case <-ticker.C:
				if len(pendingMessages) > 0 {
					c.processHotUpdate(pendingMessages)
					pendingMessages = nil
				}
			}
		}
	}()
}

// UpdateHotAsync sends a message to the hot update worker.
// If the channel is full (high load), the message is dropped silently.
// This is acceptable because hot memory is a best-effort cache of recent keywords.
func (c *MemoryCache) UpdateHotAsync(msg Message) {
	select {
	case c.hotUpdateChan <- msg:
	default:
		log.Warn("[memory] hot update dropped: channel full, msg_id=%s", msg.ID)
	}
}

func (c *MemoryCache) processHotUpdate(messages []Message) {
	c.hotDataMu.Lock()
	defer c.hotDataMu.Unlock()

	now := c.nowFunc()
	newHotData := &HotMemoryData{
		ActiveTopics:   make([]Topic, len(c.hotData.ActiveTopics)),
		TopicSummaries: make([]TopicSummary, len(c.hotData.TopicSummaries)),
		RecentKeywords: make([]Keyword, len(c.hotData.RecentKeywords)),
		LastUpdated:    now,
	}

	copy(newHotData.ActiveTopics, c.hotData.ActiveTopics)
	copy(newHotData.TopicSummaries, c.hotData.TopicSummaries)
	copy(newHotData.RecentKeywords, c.hotData.RecentKeywords)

	for _, msg := range messages {
		keywords := extractKeywords(msg.Content)
		updateKeywords(newHotData, keywords, now)
	}

	updateTopics(newHotData, now)
	cleanupExpired(newHotData, now)

	memoryDir := filepath.Join(c.dataDir, "memory")
	hotPath := filepath.Join(memoryDir, "hot.json")

	// Atomic write: only update memory if disk write succeeds
	// This ensures memory and disk are consistent
	// TODO: Add error logging and consider updating memory even on disk failure
	if err := saveHot(hotPath, newHotData); err != nil {
		return
	}

	c.hotData = newHotData
}

func (c *MemoryCache) Search(query string) ([]Message, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	return searchMessages(c.coldDB, query)
}
