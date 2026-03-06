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
	"reflect"
	"strings"
	"time"
)

type HTTPProvider struct {
	apiKey         string
	apiBase        string
	maxTokensField string
	httpClient     HTTPClient
}

type Option func(*HTTPProvider)

func WithMaxTokensField(maxTokensField string) Option {
	return func(p *HTTPProvider) {
		if maxTokensField != "" {
			p.maxTokensField = maxTokensField
		}
	}
}

func WithRequestTimeout(timeout time.Duration) Option {
	return func(p *HTTPProvider) {
		if timeout <= 0 {
			return
		}
		if c, ok := p.httpClient.(*DefaultHTTPClient); ok && c.client != nil {
			c.client.Timeout = timeout
			return
		}
		p.httpClient = &DefaultHTTPClient{client: &http.Client{Timeout: timeout}}
	}
}

func WithHTTPClient(client HTTPClient) Option {
	return func(p *HTTPProvider) {
		if client == nil {
			return
		}
		v := reflect.ValueOf(client)
		if (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface || v.Kind() == reflect.Map || v.Kind() == reflect.Slice || v.Kind() == reflect.Func || v.Kind() == reflect.Chan) && v.IsNil() {
			return
		}
		p.httpClient = client
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
		apiKey:         apiKey,
		apiBase:        strings.TrimSpace(apiBase),
		maxTokensField: DefaultZhipuMaxTokensField,
		httpClient: &DefaultHTTPClient{
			client: &http.Client{
				Timeout:   120 * time.Second,
				Transport: transport,
			},
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.httpClient == nil {
		p.httpClient = &DefaultHTTPClient{client: &http.Client{Timeout: 120 * time.Second}}
	}
	return p
}

func (p *HTTPProvider) Chat(ctx context.Context, messages []Message, model string, options ChatOptions) (*LLMResponse, error) {
	resp, err := p.sendChatRequest(ctx, messages, model, options, false)
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
				Content   string `json:"content"`
				ToolCalls []struct {
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
		Content:      payload.Choices[0].Message.Content,
		FinishReason: payload.Choices[0].FinishReason,
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

func (p *HTTPProvider) ChatStream(ctx context.Context, messages []Message, model string, options ChatOptions, handler StreamHandler) error {
	if handler == nil {
		return fmt.Errorf("%w: nil stream handler", ErrAPIError)
	}
	resp, err := p.sendChatRequest(ctx, messages, model, options, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := mapHTTPStatusError(resp); err != nil {
		return err
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	var eventData []string
	flush := func() error {
		if len(eventData) == 0 {
			return nil
		}
		payload := strings.Join(eventData, "\n")
		eventData = nil
		if payload == "[DONE]" {
			return handler(&LLMResponse{IsDone: true})
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					Thinking         string `json:"thinking"`         // GLM/DeepSeek
					ReasoningContent string `json:"reasoning_content"` // OpenAI
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidResponse, err)
		}
		for _, c := range chunk.Choices {
			out := &LLMResponse{
				Content:          c.Delta.Content,
				ReasoningContent: c.Delta.Thinking + c.Delta.ReasoningContent,
				FinishReason:     c.FinishReason,
				IsStreaming:      true,
			}
			for _, tc := range c.Delta.ToolCalls {
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
			if err := handler(out); err != nil {
				return err
			}
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			eventData = append(eventData, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrNetworkError, err)
	}
	if len(eventData) > 0 {
		if err := flush(); err != nil {
			return err
		}
	}
	return nil
}

func (p *HTTPProvider) GetDefaultModel() string {
	return ""
}

func (p *HTTPProvider) sendChatRequest(ctx context.Context, messages []Message, model string, options ChatOptions, stream bool) (*http.Response, error) {
	if p == nil || strings.TrimSpace(p.apiBase) == "" {
		return nil, fmt.Errorf("%w: api base is required", ErrAPIError)
	}

	body := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	}
	if options.Temperature != 0 {
		body["temperature"] = options.Temperature
	}
	if options.TopP != 0 {
		body["top_p"] = options.TopP
	}
	if options.MaxTokens > 0 {
		key := p.maxTokensField
		if key == "" {
			key = "max_tokens"
		}
		body[key] = options.MaxTokens
	}
	if len(options.Tools) > 0 {
		body["tools"] = options.Tools
		body["tool_choice"] = "auto"
	}
	if options.Thinking != nil {
		body["thinking"] = *options.Thinking
	}

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
