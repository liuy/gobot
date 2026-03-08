// providers SPEC
//
// =============================================================================
// Directory Structure
// =============================================================================
//
// backend/
// ├── types/
// │   └── types.go          # Shared types (StreamChunk)
// ├── providers/
// │   ├── spec.md            # This file
// │   ├── provider.go        # Interfaces, types, factory, registry
// │   ├── openai_compat.go  # HTTPProvider (shared HTTP logic)
// │   ├── zhipu.go          # Zhipu AI provider implementation
// │   └── minimax.go        # MiniMax provider implementation
// ├── main.go
// ├── protocol/
// └── log/
//
// =============================================================================
// ARCHITECTURE DESIGN
// =============================================================================
//
// 1. provider.go - Factory, protocol routing only
//    - No default values
//    - Creates specific Provider based on protocol
//    - Registry pattern for extensibility
//
// 2. Provider implementations (zhipu.go, minimax.go, etc.)
//    - Each file registers its own config
//    - API Base, Params, ExtractReasoning function
//    - Registers via RegisterProvider() in init()
//
// 3. openai_compat.go - Shared HTTP logic
//    - Pass-through Params (doesn't care about specific parameters)
//    - Uses provider's ExtractReasoning function
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
//   - Registry pattern for provider extensibility
//   - Each provider registers its own config (API Base, Params, ExtractReasoning)
//   - HTTPClient interface for testability
//   - Structured error definitions
//   - Streaming support via ChatStream

// =============================================================================
// File: provider.go
// =============================================================================

// --- Error Handling Rules ---
// DO NOT return raw internal errors. Always wrap them using sentinel errors.
// Example: fmt.Errorf("%w: %v", ErrAPIError, err)

// --- Sentinel Errors ---

var (
	ErrUnauthorized    = errors.New("provider: unauthorized (invalid API key)")
	ErrRateLimited    = errors.New("provider: rate limited")
	ErrContextCanceled = errors.New("provider: context canceled")
	ErrModelNotFound  = errors.New("provider: model not found")
	ErrAPIError       = errors.New("provider: API error")
	ErrNetworkError   = errors.New("provider: network error")
	ErrTimeout        = errors.New("provider: request timeout")
	ErrInvalidResponse = errors.New("provider: invalid response")
)

// --- HTTPClient Interface ---

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type DefaultHTTPClient struct {
	client *http.Client
}

func (d *DefaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return d.client.Do(req)
}

// --- Type Definitions ---

// IMPORTANT: All structs that interact with JSON APIs MUST include JSON tags.
// Use standard lowercase/underscore naming (e.g., "reasoning_content", not "ReasoningContent").

// StreamChunk is defined in types/types.go and shared across packages:
//   type StreamChunk struct {
//       Content  string // Normal content
//       Thinking string // Thinking/reasoning content
//       IsDone   bool   // Stream finished
//   }
//
// This type is used by LLMProvider.ChatStream return channel.

type Message struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	Media            []string `json:"media,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string `json:"tool_call_id,omitempty"`
}

type LLMResponse struct {
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	FinishReason     string         `json:"finish_reason,omitempty"`
	Usage            *UsageInfo     `json:"usage,omitempty"`
	IsStreaming      bool           `json:"-"`
	IsDone           bool           `json:"-"`
	Extensions       map[string]any `json:"-"`
}

type ToolCall struct {
	ID         string
	Type       string
	Function   *FunctionCall
	Name       string
	Arguments  map[string]any
	Extensions map[string]any
}

type FunctionCall struct {
	Name             string
	Arguments        string
	ThoughtSignature string
}

type ToolDefinition struct {
	Type     string
	Function ToolFunction
}

type ToolFunction struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type UsageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// --- ModelConfig ---

// ModelConfig defines model configuration
type ModelConfig struct {
	Model   string // "zhipu/glm-5", "minimax/MiniMax-M2.5"
	APIKey  string
	APIBase string // optional override
	Proxy   string
}

// --- ProviderDef (Registry) ---

// ExtractReasoningFunc defines how to extract reasoning/thinking content
// from provider-specific response. Each provider implements their own.
type ExtractReasoningFunc func(chunk map[string]any) string

// ProviderBuilder creates Provider
type ProviderBuilder interface {
	Build(cfg *ModelConfig) (LLMProvider, string, error)
}

// Registry must be initialized in init() of each provider file.
// CONCURRENCY: Provider registration happens in init() phase (before main).
// No runtime concurrency protection needed if registered during init.

var providerRegistry = make(map[string]ProviderBuilder)

// RegisterProvider registers Provider
func RegisterProvider(protocol string, builder ProviderBuilder) {
	providerRegistry[protocol] = builder
}

// --- Interfaces ---

type LLMProvider interface {
	Chat(ctx context.Context, messages []Message, model string, params map[string]any) (*LLMResponse, error)
	ChatStream(ctx context.Context, messages []Message, model string, params map[string]any) (<-chan types.StreamChunk, error)
	GetDefaultModel() string
}

// Note: StreamChunk is defined in types/types.go, not here.
// It is a public type shared across packages:
//   type StreamChunk struct {
//       Content  string  // Normal content
//       Thinking string  // Thinking/reasoning content
//       IsDone   bool    // Stream finished
//   }

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
//   - Case builder NOT found: returns error "provider not registered: {protocol}"
//   - NO fallback - fail fast if not registered
//
// INTENT:
//   - Create LLM provider using registry pattern
func CreateProvider(cfg *ModelConfig) (LLMProvider, string, error)

// FUNC SPEC: ExtractProtocol
// File: provider.go
//
// PRE:
//   - model is not empty
//
// POST:
//   - Returns (protocol, modelID) if format is "protocol/modelID"
//   - Returns ("", modelID) if no "/" separator
//
// INTENT:
//   - Parse provider protocol from model string
func ExtractProtocol(model string) (protocol, modelID string)

// =============================================================================
// File: openai_compat.go
// =============================================================================

// --- HTTPProvider ---

type HTTPProvider struct {
	apiKey           string
	apiBase          string
	httpClient       HTTPClient
	params           map[string]any
	extractReasoning ExtractReasoningFunc
}

// --- Options ---

type Option func(*HTTPProvider)

// PARAMETER MERGING RULE:
// When constructing API request payload, merge Provider's default `p.params` with runtime `params`.
// Runtime `params` MUST override default `p.params` if keys overlap.

func WithHTTPClient(client HTTPClient) Option
func WithParams(params map[string]any) Option
func WithExtractReasoning(f ExtractReasoningFunc) Option

// --- Constructor ---

// STREAMING RULE:
// ChatStream must:
// - Set HTTP header "Accept: text/event-stream"
// - Use bufio.Scanner or similar to parse SSE "data: " lines
// - Handle "data: [DONE]" marker as stream termination
// - Pass each parsed chunk to handler(chunk)

func NewProvider(apiKey, apiBase, proxy string, opts ...Option) *HTTPProvider

// --- Methods ---

func (p *HTTPProvider) Chat(ctx context.Context, messages []Message, model string, params map[string]any) (*LLMResponse, error)
func (p *HTTPProvider) ChatStream(ctx context.Context, messages []Message, model string, params map[string]any, handler StreamHandler) error
func (p *HTTPProvider) GetDefaultModel() string

// =============================================================================
// File: zhipu.go
// =============================================================================
// SPEC: Zhipu AI Provider
// Registers: "zhipu", "zhipu-coding"
//
// API Parameters:
//   - do_sample: bool, enable sampling, default true
//   - temperature: float, randomness, default 0.7
//   - top_p: float, nucleus sampling, default 1.0
//   - max_tokens: int, max tokens, default 65536 / 131072 (coding)
//   - thinking: object, chain of thought {type: "enabled"/"disabled"}
//
// Response Reasoning:
//   - choices[0].delta.thinking (streaming)
//   - choices[0].message.thinking (non-streaming)
//
// API Base:
//   - General: https://open.bigmodel.cn/api/paas/v4
//   - Coding: https://open.bigmodel.cn/api/coding/paas/v4
// =============================================================================

// Register zhipu providers
func registerZhipu()

// =============================================================================
// File: minimax.go
// =============================================================================
// SPEC: MiniMax Provider
// Registers: "minimax"
//
// API Parameters:
//   - temperature: float, randomness, range (0.0, 1.0], default 0.2
//   - top_p: float, nucleus sampling, default 0.1
//   - max_tokens: int, max tokens, default 16384
//   - reasoning_split: bool, separate thinking to reasoning_details, default true
//
// Response Reasoning:
//   - choices[0].delta.reasoning_details[0].text (streaming)
//   - choices[0].message.reasoning_details[0].text (non-streaming)
//
// API Base:
//   - https://api.minimaxi.com/v1
// =============================================================================

// Register minimax provider
func registerMinimax()

// =============================================================================
// Design Notes
// =============================================================================
//
// 1. Why no ChatOptions?
//    Each provider has different parameters. Let them manage via Params.
//    HTTP layer just passes through.
//
// 2. Why ExtractReasoning in ProviderBuilder?
//    Different providers return reasoning in different fields/paths.
//    Provider knows their own response format best.
//
// 3. Extending new Provider:
//    - Create xxx.go
//    - Define Params (provider defaults)
//    - Implement ExtractReasoningFunc
//    - Register in init()
//    - Add spec to spec.md (interface only, not implementation)

// =============================================================================
// TEST SPEC
// =============================================================================
//
// - Implement Table-Driven Tests for CreateProvider logic.
// - Use Mock HTTPClient to test openai_compat.go without actual network calls.
// - Provide at least one test for ExtractReasoning with mock JSON response.
