package types

// StreamChunk is a chunk of streaming response from LLM provider.
type StreamChunk struct {
	Content string // Normal content
	Thinking string // Thinking/reasoning content
	IsDone  bool   // Stream finished
}
