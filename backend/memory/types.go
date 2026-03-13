package memory

import "time"

type Message struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	HumanIDs   []string  `json:"humanIDs"`
	Channel    string    `json:"channel"`
	ChatID     string    `json:"chatID"`
	Role string `json:"role"` // "user", "assistant", "system"
	Type string `json:"type,omitempty"`
}

type TaskInfo struct {
	Name     string
	Type     string
	Status   string
	Keywords []string
	Summary  string
}

type TaskSummary struct {
	Name      string
	Type      string
	Completed time.Time
	Summary   string
}

type TaskIndex struct {
	Active    map[string]TaskInfo
	Paused    map[string]TaskInfo
	Completed []TaskSummary
}

type Context struct {
	Longterm  string
	Recent    []Message
	TaskIndex *TaskIndex
	Tasks     []string
}
