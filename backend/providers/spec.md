// providers SPEC
//
// =============================================================================
// Directory Structure
// =============================================================================
//
// backend/
// ├── providers/
// │   ├── SPEC.md            # This file
// │   ├── provider.go        # Interfaces, types, factory functions
// │   ├── openai_compat.go   # HTTPProvider implementation
// │   └── zhipu.go          # Zhipu AI constants
// ├── main.go
// ├── protocol/
// └── log/
//
// =============================================================================
// MODULE SPEC: providers
// =============================================================================
//
// RELY:
//   - net/http for HTTP client
//   - context for request cancellation
//   - errors for sentinel errors
//
// GUARANTEE:
//   - LLMProvider interface for chat completion
//   - Clean type definitions without vendor-specific fields
//   - Type-safe ChatOptions instead of map[string]any
//   - Registry pattern for provider extensibility
//   - HTTPClient interface for testability
//   - Structured error definitions
//   - Streaming support via ChatStream
//   - Multi-modal support via Message.Media
//   - Tool calling support via ToolDefinition
//   - Zhipu AI configuration constants

package providers

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// =============================================================================
// File: provider.go
// =============================================================================

// --- Sentinel Errors ---

var (
	ErrUnauthorized  = errors.New("provider: unauthorized (invalid API key)")
	ErrRateLimited   = errors.New("provider: rate limited")
	ErrContextCanceled = errors.New("provider: context canceled")
	ErrModelNotFound = errors.New("provider: model not found")
	ErrAPIError      = errors.New("provider: API error")
	ErrNetworkError  = errors.New("provider: network error")
	ErrTimeout       = errors.New("provider: request timeout")
	ErrInvalidResponse = errors.New("provider: invalid response")
)

// --- HTTPClient Interface (for testability) ---

// HTTPClient defines the interface for making HTTP requests.
// This allows easy mocking in tests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient wraps *http.Client to implement HTTPClient
type DefaultHTTPClient struct {
	client *http.Client
}

func (d *DefaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return d.client.Do(req)
}

// --- Type Definitions ---

// NOTE: All structs must include appropriate JSON tags for OpenAI API compatibility.
// Example: Content string `json:"content,omitempty"`

// Message represents a chat message (vendor-neutral)
type Message struct {
	Role             string         // "user", "assistant", "system"
	Content          string         // text content
	Media            []string       // base64 data URLs for multi-modal
	ReasoningContent string         // thinking/reasoning content (unified)
	ToolCalls        []ToolCall    // tool calls in assistant message
	ToolCallID       string        // tool result message ID

	// Extensions holds vendor-specific fields in a generic way.
	// Keys: provider name or field name (e.g., "anthropic", "google", "openai")
	// Values: provider-specific data
	Extensions map[string]any
}

// LLMResponse represents LLM API response (vendor-neutral)
type LLMResponse struct {
	Content          string        // response text
	ReasoningContent string        // thinking text (unified)
	ToolCalls        []ToolCall   // tool calls
	FinishReason     string        // "stop", "tool_calls", "length"
	Usage            *UsageInfo   // token usage
	IsStreaming      bool          // true for streaming chunks
	IsDone           bool          // true for final chunk

	// Extensions holds vendor-specific fields in a generic way.
	Extensions map[string]any
}

// ToolCall represents a tool/function call
type ToolCall struct {
	ID        string
	Type      string           // "function"
	Function  *FunctionCall
	Name      string           // convenience field
	Arguments map[string]any   // parsed arguments

	// Extensions holds vendor-specific fields.
	// Examples:
	//   - "google": map with "thought_signature"
	//   - "anthropic": map with "extra_content"
	Extensions map[string]any
}

// FunctionCall represents function call details
type FunctionCall struct {
	Name             string // function name
	Arguments        string // JSON string
	ThoughtSignature string // thinking signature (unified)
}

// ToolDefinition represents a tool definition
type ToolDefinition struct {
	Type     string
	Function ToolFunction
}

// ToolFunction represents function definition
type ToolFunction struct {
	Name        string         // function name
	Description string         // function description
	Parameters  map[string]any // JSON Schema
}

// UsageInfo represents token usage statistics
type UsageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ModelConfig represents model configuration
type ModelConfig struct {
	Model    string // "zhipu/glm-4-flash", "zhipu-coding/glm-5", or "openai/gpt-4o"
	APIKey   string
	APIBase  string // optional override
	Proxy    string // optional proxy
	Protocol string // optional explicit protocol
}

// --- ChatOptions (type-safe options) ---

// ChatOptions defines type-safe options for chat requests
// NOTE: Stream and StreamHandler are NOT included here.
// Chat() always uses stream=false internally.
// ChatStream() always uses stream=true and takes handler as parameter.
type ChatOptions struct {
	Temperature  float64          // 0.0-2.0
	MaxTokens    int              // max tokens to generate
	TopP         float64          // nucleus sampling
	Tools        []ToolDefinition // tool definitions
	Thinking     *bool            // enable thinking (provider-specific)
}

// DefaultChatOptions returns sensible defaults
func DefaultChatOptions() ChatOptions {
	return ChatOptions{
		Temperature: 0.7,
		MaxTokens:   4096,
		TopP:        1.0,
	}
}

// --- Interfaces ---

// LLMProvider is the interface for LLM providers
type LLMProvider interface {
	// Chat sends a non-streaming chat request
	Chat(ctx context.Context, messages []Message, model string, options ChatOptions) (*LLMResponse, error)

	// ChatStream sends a streaming chat request
	// handler is called for each chunk, should return error to stop
	ChatStream(ctx context.Context, messages []Message, model string, options ChatOptions, handler StreamHandler) error

	// GetDefaultModel returns the default model for this provider
	GetDefaultModel() string
}

// StreamHandler is the callback type for streaming responses
type StreamHandler func(chunk *LLMResponse) error

// ThinkingCapable is an optional interface for thinking support
type ThinkingCapable interface {
	LLMProvider
	SupportsThinking() bool
}

// --- Provider Builder (Registry Pattern) ---

// ProviderBuilder creates a provider from ModelConfig
type ProviderBuilder interface {
	Build(cfg *ModelConfig) (LLMProvider, string, error)
}

// providerRegistry stores builders for each protocol
var providerRegistry = make(map[string]ProviderBuilder)

// RegisterProvider registers a provider builder for a protocol
func RegisterProvider(protocol string, builder ProviderBuilder) {
	providerRegistry[protocol] = builder
}

// --- Factory Functions ---

// FUNC SPEC: CreateProvider
// File: provider.go
//
// PRE:
//   - cfg is not nil
//   - cfg.Model is not empty
//
// POST:
//   - Extracts protocol from cfg.Model using ExtractProtocol
//   - Looks up builder in providerRegistry
//   - Case builder found: calls builder.Build(cfg), returns result
//   - Case builder not found: falls back to OpenAI-compatible provider
//   - Case cfg is nil: returns nil, "", error("config is nil")
//   - Case cfg.Model is empty: returns nil, "", error("model is required")
//   - Case cfg.APIBase is set: uses custom API base instead of default
//   - Case protocol == "zhipu" and cfg.APIBase is empty: uses DefaultZhipuAPIBase
//   - Case protocol == "zhipu-coding" and cfg.APIBase is empty: uses DefaultZhipuCodingAPIBase
//
// INTENT:
//   - Create LLM provider using registry pattern for extensibility
func CreateProvider(cfg *ModelConfig) (LLMProvider, string, error)

// FUNC SPEC: ExtractProtocol
// File: provider.go
//
// PRE:
//   - model string (may have protocol prefix)
//
// POST:
//   - Case "zhipu/glm-4-flash": returns ("zhipu", "glm-4-flash")
//   - Case "openai/gpt-4o": returns ("openai", "gpt-4o")
//   - Case "gpt-4o" (no prefix): returns ("openai", "gpt-4o")
//   - Trims whitespace from input
//
// INTENT:
//   - Extract protocol prefix and model ID from model string
func ExtractProtocol(model string) (protocol, modelID string)

// =============================================================================
// File: openai_compat.go
// =============================================================================

// --- HTTPProvider Type ---

// HTTPProvider implements LLMProvider for OpenAI-compatible APIs
type HTTPProvider struct {
	apiKey         string
	apiBase        string
	maxTokensField string
	httpClient     HTTPClient // interface for testability
}

// --- Options ---

type Option func(*HTTPProvider)

// FUNC SPEC: WithMaxTokensField
// File: openai_compat.go
//
// PRE:
//   - maxTokensField is not empty
//
// POST:
//   - Sets maxTokensField option
//
// INTENT:
//   - Configure max tokens field name for specific models
func WithMaxTokensField(maxTokensField string) Option

// FUNC SPEC: WithRequestTimeout
// File: openai_compat.go
//
// PRE:
//   - timeout > 0
//
// POST:
//   - Sets HTTP request timeout
//
// INTENT:
//   - Configure HTTP client timeout
func WithRequestTimeout(timeout time.Duration) Option

// FUNC SPEC: WithHTTPClient
// File: openai_compat.go
//
// PRE:
//   - client implements HTTPClient interface
//
// POST:
//   - Sets custom HTTP client (useful for testing)
//
// INTENT:
//   - Allow injection of mock HTTP client
func WithHTTPClient(client HTTPClient) Option

// --- Constructor ---

// FUNC SPEC: NewProvider
// File: openai_compat.go
//
// PRE:
//   - apiBase is not empty (or use DefaultZhipuAPIBase)
//
// POST:
//   - Creates HTTPProvider with default 120s timeout
//   - Configures proxy if provided
//   - Applies optional configurations
//   - Uses DefaultHTTPClient if no custom client provided
//
// INTENT:
//   - Create HTTPProvider instance directly (bypass factory)
func NewProvider(apiKey, apiBase, proxy string, opts ...Option) *HTTPProvider

// --- Methods ---

// FUNC SPEC: HTTPProvider.Chat
// File: openai_compat.go
//
// PRE:
//   - p.apiBase is not empty
//   - messages is not nil
//
// POST:
//   - Internally sets stream=false in JSON request
//   - Sends POST to {apiBase}/chat/completions
//   - Returns LLMResponse with content
//   - Case tools provided:
//     - Includes tools in request
//     - Sets tool_choice="auto"
//   - Case error:
//     - Returns nil, error (wraps ErrAPIError, ErrNetworkError, etc.)
//   - Uses req.WithContext(ctx) for timeout/cancellation
//
// INTENT:
//   - Send non-streaming chat completion request
func (p *HTTPProvider) Chat(ctx context.Context, messages []Message, model string, options ChatOptions) (*LLMResponse, error)

// FUNC SPEC: HTTPProvider.ChatStream
// File: openai_compat.go
//
// PRE:
//   - p.apiBase is not empty
//   - messages is not nil
//   - handler is not nil
//
// POST:
//   - Internally sets stream=true in JSON request
//   - Parses SSE events (format: "data: {...}")
//   - Handles "data: [DONE]" as stream termination marker
//   - Extracts thinking/reasoning from multiple provider-specific fields:
//     - "thinking" (GLM, DeepSeek)
//     - "reasoning_content" (OpenAI)
//   - Populates chunk.ReasoningContent with combined thinking data
//   - Calls handler for each chunk with IsStreaming=true
//   - Calls handler one final time with IsDone=true to signal completion
//   - Returns nil when stream ends or handler returns error
//   - Case error:
//     - Returns error (wraps ErrAPIError, ErrNetworkError, etc.)
//   - Uses req.WithContext(ctx) for timeout/cancellation
//
// INTENT:
//   - Send streaming chat completion request
func (p *HTTPProvider) ChatStream(ctx context.Context, messages []Message, model string, options ChatOptions, handler StreamHandler) error

// FUNC SPEC: HTTPProvider.GetDefaultModel
// File: openai_compat.go
//
// PRE:
//   - p is not nil
//
// POST:
//   - Returns "" (no default model for HTTPProvider)
//
// INTENT:
//   - Implement LLMProvider interface
func (p *HTTPProvider) GetDefaultModel() string

// =============================================================================
// File: zhipu.go
// =============================================================================

// --- Constants ---

// Default API base URL for Zhipu AI (general)
const DefaultZhipuAPIBase = "https://open.bigmodel.cn/api/paas/v4"

// API base URL for Zhipu AI Coding Plan
const DefaultZhipuCodingAPIBase = "https://open.bigmodel.cn/api/coding/paas/v4"

// Default model for Zhipu AI
const DefaultZhipuModel = "glm-5"

// Default max_tokens for GLM-5 (coding scenario)
const DefaultZhipuMaxTokens = 131072

// Max tokens field name for GLM models
const DefaultZhipuMaxTokensField = "max_tokens"

// Default thinking enabled for GLM-4.5+ models
const DefaultZhipuThinkingEnabled = true
