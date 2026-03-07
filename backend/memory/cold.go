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
		is_from_human INTEGER,
		type TEXT
	);
	
	CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp DESC);
	
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

func insertMessage(db *sql.DB, msg Message) error {
	humanIDsJSON, err := json.Marshal(msg.HumanIDs)
	if err != nil {
		return err
	}

	// Tokenize content for FTS5 search
	contentTokens := tokenizeChinese(msg.Content)

	_, err = db.Exec(`
		INSERT INTO messages (id, content, content_tokens, timestamp, human_ids, channel, chat_id, is_from_human, type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.ID, msg.Content, contentTokens, msg.Timestamp.Format("2006-01-02 15:04:05"), string(humanIDsJSON),
		msg.Channel, msg.ChatID, msg.IsFromHuman, msg.Type)

	return err
}

func getRecentMessages(db *sql.DB, limit int) ([]Message, error) {
	rows, err := db.Query(`
		SELECT id, content, timestamp, human_ids, channel, chat_id, is_from_human, type
		FROM messages
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var humanIDsJSON string
		var timestampStr string
		var isFromHuman int

		err := rows.Scan(&msg.ID, &msg.Content, &timestampStr, &humanIDsJSON,
			&msg.Channel, &msg.ChatID, &isFromHuman, &msg.Type)
		if err != nil {
			return nil, err
		}

		msg.IsFromHuman = isFromHuman == 1

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
		SELECT m.id, m.content, m.timestamp, m.human_ids, m.channel, m.chat_id, m.is_from_human, m.type
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
		var isFromHuman int

		err := rows.Scan(&msg.ID, &msg.Content, &timestampStr, &humanIDsJSON,
			&msg.Channel, &msg.ChatID, &isFromHuman, &msg.Type)
		if err != nil {
			return nil, err
		}

		msg.IsFromHuman = isFromHuman == 1

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
