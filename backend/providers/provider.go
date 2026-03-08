package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gobot/types"
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
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	Media            []string   `json:"media,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
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

type ModelConfig struct {
	Model   string
	APIKey  string
	APIBase string
	Proxy   string
}

type ExtractReasoningFunc func(chunk map[string]any) string

type ProviderDef struct {
	APIBase          string
	Params           map[string]any
	ExtractReasoning ExtractReasoningFunc
}

type ProviderBuilder interface {
	Build(cfg *ModelConfig) (LLMProvider, string, error)
}

var providerRegistry = make(map[string]ProviderBuilder)

func RegisterProvider(protocol string, builder ProviderBuilder) {
	providerRegistry[protocol] = builder
}

type LLMProvider interface {
	Chat(ctx context.Context, messages []Message, model string, params map[string]any) (*LLMResponse, error)
	ChatStream(ctx context.Context, messages []Message, model string, params map[string]any) (<-chan types.StreamChunk, error)
	GetDefaultModel() string
}

func CreateProvider(cfg *ModelConfig) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("%w: config is nil", ErrAPIError)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, "", fmt.Errorf("%w: model is required", ErrAPIError)
	}

	protocol, modelID := ExtractProtocol(cfg.Model)
	builder, ok := providerRegistry[protocol]
	if !ok {
		return nil, "", fmt.Errorf("provider not registered: %s", protocol)
	}
	provider, _, err := builder.Build(cfg)
	return provider, modelID, err
}

func ExtractProtocol(model string) (protocol, modelID string) {
	clean := strings.TrimSpace(model)
	if parts := strings.SplitN(clean, "/", 2); len(parts) == 2 {
		protocol = strings.TrimSpace(parts[0])
		modelID = strings.TrimSpace(parts[1])
		if protocol != "" && modelID != "" {
			return protocol, modelID
		}
	}
	return "", clean
}
