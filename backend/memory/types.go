package memory

import (
	"time"
)

// ContentPart represents a single part of a message content, compatible with OpenAI/Anthropic format
type ContentPart struct {
	Type      string `json:"type"` // "text", "thinking", "tool_call", "image", "tool_result"
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Name      string `json:"name,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
}

type Message struct {
	ID          string    `json:"id"`
	Content     any       `json:"content"` // string | []ContentPart | nil
	Timestamp   int64     `json:"timestamp"`
	HumanIDs    []string  `json:"humanIDs"`
	Channel     string    `json:"channel"`
	ChatID      string    `json:"chatID"`
	Role        string    `json:"role"` // "user", "assistant", "system"
	StopReason  string    `json:"stopReason,omitempty"` // "stop", "length", "content_filter", "error"
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
