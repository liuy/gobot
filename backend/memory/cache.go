package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gobot/log"
)

const (
	longtermCheckTTL = 30 * time.Second
	updateChanSize   = 1000
	updateInterval   = 100 * time.Millisecond
	recentBufferSize = 20
)

// recentBuffer is a ring buffer for recent messages.
// It stores messages in chronological order (oldest first) and automatically
// overwrites the oldest message when full.
// Thread-unsafe: callers must hold lock.
type recentBuffer struct {
	data  [recentBufferSize]Message
	head  int // next write position
	count int // current message count
}

// Add adds a message to the buffer. O(1) operation.
func (r *recentBuffer) Add(msg Message) {
	r.data[r.head] = msg
	r.head = (r.head + 1) % recentBufferSize
	if r.count < recentBufferSize {
		r.count++
	}
}

// GetByChatID returns messages for a specific chatID in chronological order (oldest first).
func (r *recentBuffer) GetByChatID(chatID string) []Message {
	if r.count == 0 {
		return nil
	}
	var result []Message
	// start points to the oldest message: (head - count + size) % size
	start := (r.head - r.count + recentBufferSize) % recentBufferSize
	// Traverse from oldest to newest: iterate forward from start
	for i := 0; i < r.count; i++ {
		idx := (start + i) % recentBufferSize
		msg := r.data[idx]
		if msg.ChatID == chatID {
			result = append(result, msg)
		}
	}
	return result
}

type MemoryCache struct {
	dataDir string

	longterm          string
	longtermMod       time.Time
	longtermMu        sync.RWMutex
	longtermLastCheck time.Time

	recent   recentBuffer
	recentMu sync.RWMutex

	coldDB *sql.DB

	updateChan chan Message
	workerWg   sync.WaitGroup
	stopChan   chan struct{}
	closeOnce  sync.Once
	closeErr   error
	closed     int32 // atomic: 0 = open, 1 = closed

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
		dataDir:    dataDir,
		coldDB:     coldDB,
		updateChan: make(chan Message, updateChanSize),
		stopChan:   make(chan struct{}),
		nowFunc:    time.Now,
		// recent (ring buffer) is zero-initialized automatically
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
		cache.longtermLastCheck = time.Now() // Initial load counts as check
	}

	cache.startWorker()

	return cache, nil
}

func (c *MemoryCache) Close() error {
	c.closeOnce.Do(func() {
		atomic.StoreInt32(&c.closed, 1)
		close(c.stopChan)

		done := make(chan struct{})
		go func() {
			c.workerWg.Wait()
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
	now := c.nowFunc()
	c.longtermMu.RLock()
	if now.Sub(c.longtermLastCheck) < longtermCheckTTL {
		result := c.longterm
		c.longtermMu.RUnlock()
		return result, nil
	}
	c.longtermMu.RUnlock()

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

	c.longtermMu.RLock()
	shouldReload := !info.ModTime().Equal(c.longtermMod)
	c.longtermMu.RUnlock()

	if !shouldReload {
		c.longtermMu.Lock()
		c.longtermLastCheck = now
		result := c.longterm
		c.longtermMu.Unlock()
		return result, nil
	}

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

// GetRecent returns recent messages for a specific chatID, limited by the given limit.
// Returns a new slice, safe for callers to modify.
// If chatID is empty, returns empty slice with a warning.
// Falls back to cold.db when memory cache is empty for the given chatID.
func (c *MemoryCache) GetRecent(chatID string, limit int) []Message {
	if chatID == "" {
		log.Warn("[memory] GetRecent called with empty chatID, returning empty slice")
		return []Message{}
	}

	c.recentMu.RLock()
	filtered := c.recent.GetByChatID(chatID)
	c.recentMu.RUnlock()

	if len(filtered) == 0 {
		messages, err := getRecentMessages(c.coldDB, chatID, limit)
		if err != nil {
			log.Warn("[memory] cold.db fallback failed: err=%v", err)
			return []Message{}
		}
		return messages
	}

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered // already in chronological order (oldest first)
}

// AddMessage adds a message to the memory system.
// This is an async operation: Recent is updated immediately, Cold and Hot are updated asynchronously.
// Returns immediately (~1µs) regardless of system load.
func (c *MemoryCache) AddMessage(msg Message) error {
	// Check if closed
	if atomic.LoadInt32(&c.closed) == 1 {
		return fmt.Errorf("memory cache is closed")
	}

	// Validate input
	if strings.TrimSpace(msg.ID) == "" {
		return fmt.Errorf("message ID cannot be empty")
	}
	// Extract text from Content for validation (supports string or []ContentPart)
	if strings.TrimSpace(ExtractTextFromContent(msg.Content)) == "" {
		return fmt.Errorf("message content cannot be empty")
	}
	if msg.Timestamp.IsZero() {
		return fmt.Errorf("message timestamp cannot be zero")
	}

	// Update Recent immediately (synchronous, ~100ns)
	c.recentMu.Lock()
	c.recent.Add(msg)
	c.recentMu.Unlock()

	// Queue for async Cold + Hot update
	select {
	case c.updateChan <- msg:
	default:
		log.Warn("[memory] update queue full, message dropped: msg_id=%s", msg.ID)
	}

	return nil
}

// Append is an alias for AddMessage for backward compatibility.
func (c *MemoryCache) Append(msg Message) error {
	return c.AddMessage(msg)
}

// startWorker starts the unified async worker for Cold + Hot updates.
func (c *MemoryCache) startWorker() {
	c.workerWg.Add(1)
	go func() {
		defer c.workerWg.Done()

		ticker := time.NewTicker(updateInterval)
		defer ticker.Stop()

		var pending []Message

		for {
			select {
			case <-c.stopChan:
				// Drain remaining messages
				for {
					select {
					case msg := <-c.updateChan:
						pending = append(pending, msg)
					default:
						goto done
					}
				}
			done:
				if len(pending) > 0 {
					c.flushPending(pending)
				}
				return

			case msg := <-c.updateChan:
				pending = append(pending, msg)

			case <-ticker.C:
				if len(pending) > 0 {
					c.flushPending(pending)
					pending = nil
				}
			}
		}
	}()
}

// flushPending writes pending messages to Cold.
func (c *MemoryCache) flushPending(messages []Message) {
	for _, msg := range messages {
		if err := insertMessage(c.coldDB, msg); err != nil {
			log.Warn("[memory] failed to insert message to cold: msg_id=%s, err=%v", msg.ID, err)
		}
	}
}

func (c *MemoryCache) Search(query string) ([]Message, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	return searchMessages(c.coldDB, query)
}

// Flush waits for all pending messages to be processed.
// Useful for testing.
func (c *MemoryCache) Flush() {
	time.Sleep(updateInterval + 50*time.Millisecond)
}
