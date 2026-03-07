package memory

import "time"

type Message struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
	HumanIDs    []string  `json:"humanIDs"`
	Channel     string    `json:"channel"`
	ChatID      string    `json:"chatID"`
	IsFromHuman bool      `json:"isFromHuman"`
	Type        string    `json:"type,omitempty"`
}

type HotMemoryData struct {
	ActiveTopics   []Topic        `json:"activeTopics"`
	TopicSummaries []TopicSummary `json:"topicSummaries"`
	RecentKeywords []Keyword      `json:"recentKeywords"`
	LastUpdated    time.Time      `json:"lastUpdated"`
}

type Topic struct {
	Name       string    `json:"name"`
	LastActive time.Time `json:"lastActive"`
	Count      int       `json:"count"`
}

type TopicSummary struct {
	Topic      string    `json:"topic"`
	Summary    string    `json:"summary"`
	KeyPoints  []string  `json:"keyPoints"`
	LastActive time.Time `json:"lastActive"`
}

type Keyword struct {
	Word       string    `json:"word"`
	LastActive time.Time `json:"lastActive"`
	Count      int       `json:"count"`
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
	Hot       *HotMemoryData
	Recent    []Message
	TaskIndex *TaskIndex
	Tasks     []string
}
