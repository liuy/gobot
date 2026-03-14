package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	_ "github.com/mattn/go-sqlite3"
)

func initColdDB(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s?_busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		content_tokens TEXT,
		timestamp TEXT NOT NULL,
		human_ids TEXT,
		channel TEXT,
		chat_id TEXT,
		role TEXT,
		stop_reason TEXT
	);
	
	CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_id);
	CREATE INDEX IF NOT EXISTS idx_messages_chat_id_timestamp ON messages(chat_id, timestamp DESC);
	
	CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
		content_tokens,
		content='messages',
		content_rowid='rowid',
		tokenize='unicode61'
	);
	
	CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
		INSERT INTO messages_fts(rowid, content_tokens) VALUES (NEW.rowid, NEW.content_tokens);
	END;
	
	CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
		INSERT INTO messages_fts(messages_fts, rowid, content_tokens) VALUES('delete', OLD.rowid, OLD.content_tokens);
	END;
	
	CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
		INSERT INTO messages_fts(messages_fts, rowid, content_tokens) VALUES('delete', OLD.rowid, OLD.content_tokens);
		INSERT INTO messages_fts(rowid, content_tokens) VALUES (NEW.rowid, NEW.content_tokens);
	END;
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// ExtractTextFromContent extracts plain text from Content (string or []ContentPart) for tokenization
func ExtractTextFromContent(content any) string {
	if content == nil {
		return ""
	}
	switch c := content.(type) {
	case string:
		return c
	case []ContentPart:
		var sb strings.Builder
		for _, part := range c {
			if part.Text != "" {
				sb.WriteString(part.Text)
				sb.WriteString(" ")
			}
			if part.Thinking != "" {
				sb.WriteString(part.Thinking)
				sb.WriteString(" ")
			}
		}
		return sb.String()
	default:
		// Fallback: try to marshal and extract
		if data, err := json.Marshal(c); err == nil {
			return string(data)
		}
		return ""
	}
}

func insertMessage(db *sql.DB, msg Message) error {
	humanIDsJSON, err := json.Marshal(msg.HumanIDs)
	if err != nil {
		return err
	}

	// Serialize content to JSON for storage
	contentJSON, err := json.Marshal(msg.Content)
	if err != nil {
		return err
	}

	// Tokenize content for FTS5 search
	contentTokens := tokenizeChinese(ExtractTextFromContent(msg.Content))

	_, err = db.Exec(`
		INSERT OR IGNORE INTO messages (id, content, content_tokens, timestamp, human_ids, channel, chat_id, role, stop_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.ID, string(contentJSON), contentTokens, msg.Timestamp.Format("2006-01-02 15:04:05"), string(humanIDsJSON),
		msg.Channel, msg.ChatID, msg.Role, msg.StopReason)

	return err
}

// getRecentMessages retrieves the most recent N messages for a chatID in chronological order (oldest first).
// Uses a subquery to first get the latest N messages, then orders them by timestamp ASC.
func getRecentMessages(db *sql.DB, chatID string, limit int) ([]Message, error) {
	// Subquery: get latest N messages, then order by timestamp ASC for chronological order
	rows, err := db.Query(`
		SELECT id, content, timestamp, human_ids, channel, chat_id, role, stop_reason
		FROM (
			SELECT id, content, timestamp, human_ids, channel, chat_id, role, stop_reason
			FROM messages
			WHERE chat_id = ?
			ORDER BY timestamp DESC
			LIMIT ?
		)
		ORDER BY timestamp ASC
	`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var humanIDsJSON string
		var timestampStr string
		var contentJSON string

		err := rows.Scan(&msg.ID, &contentJSON, &timestampStr, &humanIDsJSON,
			&msg.Channel, &msg.ChatID, &msg.Role, &msg.StopReason)
		if err != nil {
			return nil, err
		}

		// Deserialize content from JSON
		if err := json.Unmarshal([]byte(contentJSON), &msg.Content); err != nil {
			// Fallback to plain text if unmarshal fails
			msg.Content = contentJSON
		}

		if err := json.Unmarshal([]byte(humanIDsJSON), &msg.HumanIDs); err != nil {
			msg.HumanIDs = []string{}
		}

		msg.Timestamp = parseTimestamp(timestampStr)
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func searchMessages(db *sql.DB, query string) ([]Message, error) {
	tokenizedQuery := tokenizeChinese(query)
	sanitizedQuery := sanitizeFTS5Query(tokenizedQuery)

	rows, err := db.Query(`
		SELECT m.id, m.content, m.timestamp, m.human_ids, m.channel, m.chat_id, m.role, m.stop_reason
		FROM messages m
		JOIN messages_fts fts ON m.rowid = fts.rowid
		WHERE messages_fts MATCH ?
		ORDER BY bm25(messages_fts)
		LIMIT 20
	`, sanitizedQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var humanIDsJSON string
		var timestampStr string
		var contentJSON string

		err := rows.Scan(&msg.ID, &contentJSON, &timestampStr, &humanIDsJSON,
			&msg.Channel, &msg.ChatID, &msg.Role, &msg.StopReason)
		if err != nil {
			return nil, err
		}

		// Deserialize content from JSON
		if err := json.Unmarshal([]byte(contentJSON), &msg.Content); err != nil {
			msg.Content = contentJSON
		}

		if err := json.Unmarshal([]byte(humanIDsJSON), &msg.HumanIDs); err != nil {
			msg.HumanIDs = []string{}
		}

		msg.Timestamp = parseTimestamp(timestampStr)
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func sanitizeFTS5Query(query string) string {
	var builder strings.Builder
	builder.Grow(len(query))

	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

func parseTimestamp(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return time.Time{}
	}
	return t
}
