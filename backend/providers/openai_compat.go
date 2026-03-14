package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gobot/types"
)

type HTTPProvider struct {
	apiKey           string
	apiBase          string
	httpClient       HTTPClient
	params           map[string]any
	extractReasoning ExtractReasoningFunc
}

type Option func(*HTTPProvider)

func WithHTTPClient(client HTTPClient) Option {
	return func(p *HTTPProvider) {
		if client != nil {
			p.httpClient = client
		}
	}
}

func WithParams(params map[string]any) Option {
	return func(p *HTTPProvider) {
		p.params = params
	}
}

func WithExtractReasoning(f ExtractReasoningFunc) Option {
	return func(p *HTTPProvider) {
		p.extractReasoning = f
	}
}

func NewProvider(apiKey, apiBase, proxy string, opts ...Option) *HTTPProvider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.TrimSpace(proxy) != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	p := &HTTPProvider{
		apiKey:     apiKey,
		apiBase:    strings.TrimSpace(apiBase),
		httpClient: &DefaultHTTPClient{client: &http.Client{Timeout: 120 * time.Second, Transport: transport}},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.httpClient == nil {
		p.httpClient = &DefaultHTTPClient{client: &http.Client{Timeout: 120 * time.Second}}
	}
	return p
}

func (p *HTTPProvider) Chat(ctx context.Context, messages []Message, model string, params map[string]any) (*LLMResponse, error) {
	resp, err := p.sendChatRequest(ctx, messages, model, params, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := mapHTTPStatusError(resp); err != nil {
		return nil, err
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content           string `json:"content"`
				Thinking          string `json:"thinking"`
				ReasoningContent  string `json:"reasoning_content"`
				ToolCalls         []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	if len(payload.Choices) == 0 {
		return &LLMResponse{}, nil
	}

	out := &LLMResponse{
		Content:          payload.Choices[0].Message.Content,
		ReasoningContent: payload.Choices[0].Message.Thinking + payload.Choices[0].Message.ReasoningContent,
		FinishReason:     payload.Choices[0].FinishReason,
		StopReason:       payload.Choices[0].FinishReason,
	}
	if payload.Usage != nil {
		out.Usage = &UsageInfo{
			PromptTokens:     payload.Usage.PromptTokens,
			CompletionTokens: payload.Usage.CompletionTokens,
			TotalTokens:      payload.Usage.TotalTokens,
		}
	}
	for _, tc := range payload.Choices[0].Message.ToolCalls {
		call := ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: &FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
			Name:      tc.Function.Name,
			Arguments: map[string]any{},
		}
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &call.Arguments)
		}
		out.ToolCalls = append(out.ToolCalls, call)
	}
	return out, nil
}

func (p *HTTPProvider) ChatStream(ctx context.Context, messages []Message, model string, params map[string]any) (<-chan types.StreamChunk, error) {
	ch := make(chan types.StreamChunk, 10)

	resp, err := p.sendChatRequest(ctx, messages, model, params, true)
	if err != nil {
		return nil, err
	}

	if err := mapHTTPStatusError(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

		var eventData []string
		flush := func() {
			if len(eventData) == 0 {
				return
			}
			payload := strings.Join(eventData, "\n")
			eventData = nil
			if payload == "[DONE]" {
				ch <- types.StreamChunk{StopReason: "stop"}
				return
			}

			var rawChunk map[string]any
			if err := json.Unmarshal([]byte(payload), &rawChunk); err != nil {
				return
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string `json:"content"`
						Thinking         string `json:"thinking"`
						ReasoningContent string `json:"reasoning_content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				return
			}
			for _, c := range chunk.Choices {
				reasoning := c.Delta.Thinking + c.Delta.ReasoningContent
				if reasoning == "" && p.extractReasoning != nil {
					reasoning = p.extractReasoning(rawChunk)
				}
				ch <- types.StreamChunk{
					Content:    c.Delta.Content,
					Thinking:   reasoning,
					StopReason: c.FinishReason,
				}
			}
		}

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				flush()
				continue
			}
			if strings.HasPrefix(line, "data:") {
				eventData = append(eventData, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if len(eventData) > 0 {
			flush()
		}
	}()

	return ch, nil
}

func (p *HTTPProvider) GetDefaultModel() string {
	return ""
}

func (p *HTTPProvider) sendChatRequest(ctx context.Context, messages []Message, model string, params map[string]any, stream bool) (*http.Response, error) {
	if p == nil || strings.TrimSpace(p.apiBase) == "" {
		return nil, fmt.Errorf("%w: api base is required", ErrAPIError)
	}

	body := make(map[string]any)
	for k, v := range p.params {
		body[k] = v
	}
	for k, v := range params {
		body[k] = v
	}
	body["model"] = model
	body["messages"] = messages
	body["stream"] = stream

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIError, err)
	}

	endpoint := strings.TrimRight(p.apiBase, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIError, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("%w: %v", ErrContextCanceled, err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrNetworkError, err)
	}
	return resp, nil
}

func mapHTTPStatusError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	data, _ := io.ReadAll(resp.Body)
	msg := strings.TrimSpace(string(data))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrUnauthorized, msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrRateLimited, msg)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrModelNotFound, msg)
	default:
		return fmt.Errorf("%w: status %d %s", ErrAPIError, resp.StatusCode, msg)
	}
}
