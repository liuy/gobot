package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrUnauthorized    = errors.New("provider: unauthorized (invalid API key)")
	ErrRateLimited     = errors.New("provider: rate limited")
	ErrContextCanceled = errors.New("provider: context canceled")
	ErrModelNotFound   = errors.New("provider: model not found")
	ErrAPIError        = errors.New("provider: API error")
	ErrNetworkError    = errors.New("provider: network error")
	ErrTimeout         = errors.New("provider: request timeout")
	ErrInvalidResponse = errors.New("provider: invalid response")
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type DefaultHTTPClient struct {
	client *http.Client
}

func (d *DefaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return d.client.Do(req)
}

type Message struct {
	Role             string         `json:"role"`
	Content          string         `json:"content,omitempty"`
	Media            []string       `json:"media,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	Extensions       map[string]any `json:"-"`
}

type LLMResponse struct {
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	FinishReason     string         `json:"finish_reason,omitempty"`
	Usage            *UsageInfo     `json:"usage,omitempty"`
	IsStreaming      bool           `json:"is_streaming,omitempty"`
	IsDone           bool           `json:"is_done,omitempty"`
	Extensions       map[string]any `json:"extensions,omitempty"`
}

type ToolCall struct {
	ID         string         `json:"id,omitempty"`
	Type       string         `json:"type,omitempty"`
	Function   *FunctionCall  `json:"function,omitempty"`
	Name       string         `json:"name,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type FunctionCall struct {
	Name             string `json:"name,omitempty"`
	Arguments        string `json:"arguments,omitempty"`
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

type ToolDefinition struct {
	Type     string       `json:"type,omitempty"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type ModelConfig struct {
	Model    string
	APIKey   string
	APIBase  string
	Proxy    string
	Protocol string
}

type ChatOptions struct {
	Temperature float64
	MaxTokens   int
	TopP        float64
	Tools       []ToolDefinition
	Thinking    *bool
}

func DefaultChatOptions() ChatOptions {
	return ChatOptions{
		Temperature: 0.7,
		MaxTokens:   4096,
		TopP:        1.0,
	}
}

type LLMProvider interface {
	Chat(ctx context.Context, messages []Message, model string, options ChatOptions) (*LLMResponse, error)
	ChatStream(ctx context.Context, messages []Message, model string, options ChatOptions, handler StreamHandler) error
	GetDefaultModel() string
}

type StreamHandler func(chunk *LLMResponse) error

type ThinkingCapable interface {
	LLMProvider
	SupportsThinking() bool
}

type ProviderBuilder interface {
	Build(cfg *ModelConfig) (LLMProvider, string, error)
}

var providerRegistry = make(map[string]ProviderBuilder)

func RegisterProvider(protocol string, builder ProviderBuilder) {
	providerRegistry[protocol] = builder
}

func CreateProvider(cfg *ModelConfig) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", errors.New("config is nil")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, "", errors.New("model is required")
	}

	protocol, modelID := ExtractProtocol(cfg.Model)
	if builder, ok := providerRegistry[protocol]; ok {
		return builder.Build(cfg)
	}

	apiBase := strings.TrimSpace(cfg.APIBase)
	if apiBase == "" {
		switch protocol {
		case "zhipu":
			apiBase = DefaultZhipuAPIBase
		case "zhipu-coding":
			apiBase = DefaultZhipuCodingAPIBase
		}
	}

	p := NewProvider(cfg.APIKey, apiBase, cfg.Proxy)
	if p == nil {
		return nil, "", fmt.Errorf("%w: failed to create provider", ErrAPIError)
	}
	return p, modelID, nil
}

func ExtractProtocol(model string) (protocol, modelID string) {
	clean := strings.TrimSpace(model)
	if parts := strings.SplitN(clean, "/", 2); len(parts) == 2 {
		prefix := strings.TrimSpace(parts[0])
		id := strings.TrimSpace(parts[1])
		if prefix != "" && id != "" {
			return prefix, id
		}
	}
	return "openai", clean
}
