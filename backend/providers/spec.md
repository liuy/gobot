// providers SPEC
//
// =============================================================================
// Directory Structure
// =============================================================================
//
// backend/
// ├── providers/
// │   ├── spec.md            # This file
// │   ├── provider.go        # Interfaces, types, factory, registry
// │   ├── openai_compat.go # HTTPProvider (shared HTTP logic)
// │   └── zhipu.go         # Zhipu AI provider registration
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
// 2. zhipu.go - Specific Provider registers its own config
//    - API Base
//    - Params (temperature, top_p, max_tokens, thinking, etc.)
//    - ExtractReasoning function (provider-specific)
//    - Registers "zhipu" and "zhipu-coding"
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

type Message struct {
	Role              string `json:"role"`
	Content           string `json:"content"`
	Media             []string `json:"media,omitempty"`
	ReasoningContent  string `json:"reasoning_content,omitempty"`
	ToolCalls         []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID        string `json:"tool_call_id,omitempty"`
}

type LLMResponse struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	FinishReason     string `json:"finish_reason,omitempty"`
	Usage            *UsageInfo `json:"usage,omitempty"`
	IsStreaming      bool `json:"-"`
	IsDone           bool `json:"-"`
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
	Model   string // "zhipu/glm-5", "openai/gpt-4o"
	APIKey  string
	APIBase string // optional override
	Proxy   string
}

// --- ProviderDef (Registry) ---

// ExtractReasoningFunc defines how to extract reasoning/thinking content
// from provider-specific response. Each provider implements their own.
type ExtractReasoningFunc func(chunk map[string]any) string

// ProviderDef defines each specific Provider's config
type ProviderDef struct {
	APIBase          string                  // API endpoint
	Params           map[string]any          // default params
	ExtractReasoning ExtractReasoningFunc   // provider-specific reasoning extraction
}

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
	ChatStream(ctx context.Context, messages []Message, model string, params map[string]any, handler StreamHandler) error
	GetDefaultModel() string
}

type StreamHandler func(chunk *LLMResponse) error

// --- Factory Functions ---

// FUNC SPEC: CreateProvider
// File: provider.go
//
// PRE:
//   - registerZhipu() must be called before this function
//   - cfg is not nil
//   - cfg.Model is not empty
//
// POST:
//   - Extracts protocol from cfg.Model using ExtractProtocol
//   - Looks up builder in providerRegistry
//   - Case builder found: calls builder.Build(cfg), returns result
//   - Case builder NOT found: returns error "provider not registered: {protocol}"
//   - NO fallback to OpenAI - fail fast if not registered
//
// INTENT:
//   - Create LLM provider using registry pattern
func CreateProvider(cfg *ModelConfig) (LLMProvider, string, error)

func ExtractProtocol(model string) (protocol, modelID string)

// =============================================================================
// File: openai_compat.go
// =============================================================================

// --- HTTPProvider ---

type HTTPProvider struct {
	apiKey          string
	apiBase         string
	httpClient      HTTPClient
	params          map[string]any          // default params
	extractReasoning ExtractReasoningFunc // provider-specific extraction
}

// --- Options ---

type Option func(*HTTPProvider)

// PARAMETER MERGING RULE:
// When constructing API request payload, merge Provider's default `p.params` with runtime `params`.
// Runtime `params` MUST override default `p.params` if keys overlap.

// Option functions:
// - WithHTTPClient: Inject custom HTTP client (for testing with mocks)
// - WithParams: Set Provider default parameters
// - WithExtractReasoning: Set reasoning extraction function

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
//
// Zhipu AI Provider Registration
// Registers two sub-providers: zhipu (general) and zhipu-coding (coding)
//
// Zhipu API Parameters:
// - do_sample: bool, enable sampling, default true
// - temperature: float, randomness, default 0.7
// - top_p: float, nucleus sampling, default 1.0
// - max_tokens: int, max tokens, default 65536 (glm-5) / 131072 (glm-5 coding)
// - thinking: object, chain of thought {type: "enabled"/"disabled"}, default enabled
//
// IMPORTANT: "thinking" here is a REQUEST CONTROL PARAMETER (object).
// The response field for reasoning content is provider-specific.
// =============================================================================

// Zhipu Default Parameters
var zhipuParams = map[string]any{
	"do_sample":   true,
	"temperature": 0.7,
	"top_p":       1.0,
	"max_tokens":  65536,
	"thinking":    map[string]string{"type": "enabled"},
}

var zhipuCodingParams = map[string]any{
	"do_sample":   true,
	"temperature": 0.7,
	"top_p":       1.0,
	"max_tokens":  131072,
	"thinking":    map[string]string{"type": "enabled"},
}

// Zhipu Reasoning Extraction
// Based on actual Zhipu API response format
// Response reasoning appears in: choices[0].delta.thinking (stream)
// or choices[0].message.thinking (non-stream)
func zhipuExtractReasoning(chunk map[string]any) string {
	// Try to extract from choices array
	choices, ok := chunk["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}

	choice, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}

	// Check delta (streaming) first
	if delta, ok := choice["delta"].(map[string]any); ok {
		if thinking, ok := delta["thinking"].(string); ok && thinking != "" {
			return thinking
		}
	}

	// Check message (non-streaming)
	if msg, ok := choice["message"].(map[string]any); ok {
		if thinking, ok := msg["thinking"].(string); ok && thinking != "" {
			return thinking
		}
	}

	return ""
}

// Register Zhipu providers
// IMPORTANT: Must be called before CreateProvider
// Call in main.go or use registerAll() helper
func registerZhipu() {
	// zhipu general API
	RegisterProvider("zhipu", &zhipuBuilder{
		apiBase:          "https://open.bigmodel.cn/api/paas/v4",
		params:           zhipuParams,
		extractReasoning: zhipuExtractReasoning,
	})

	// zhipu-coding coding API
	RegisterProvider("zhipu-coding", &zhipuBuilder{
		apiBase:          "https://open.bigmodel.cn/api/coding/paas/v4",
		params:           zhipuCodingParams,
		extractReasoning: zhipuExtractReasoning,
	})
}

type zhipuBuilder struct {
	apiBase          string
	params           map[string]any
	extractReasoning ExtractReasoningFunc
}

func (b *zhipuBuilder) Build(cfg *ModelConfig) (LLMProvider, string, error) {
	apiBase := cfg.APIBase
	if apiBase == "" {
		apiBase = b.apiBase
	}

	p := NewProvider(
		cfg.APIKey,
		apiBase,
		cfg.Proxy,
		WithParams(b.params),
		WithExtractReasoning(b.extractReasoning),
	)
	return p, "", nil
}

// =============================================================================
// Design Notes
// =============================================================================
//
// 1. Why no ChatOptions?
//    Each provider has different parameters. Let them manage via Params.
//    HTTP layer just passes through.
//
// 2. Why ExtractReasoning in ProviderDef?
//    Different providers return reasoning in different fields/paths.
//    Provider knows their own response format best.
//
// 3. Extending new Provider:
//    - Create xxx.go
//    - Define Params (provider defaults)
//    - Implement ExtractReasoningFunc
//    - Register in init()
//    - No changes needed to openai_compat.go

// =============================================================================
// TEST SPEC
// =============================================================================
//
// - Implement Table-Driven Tests for CreateProvider logic.
// - Use Mock HTTPClient to test openai_compat.go without actual network calls.
// - Provide at least one test for zhipuExtractReasoning with mock JSON response.
