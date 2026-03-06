package providers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type mockHTTPClient struct {
	do func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.do(req)
}

func TestWithMaxTokensField(t *testing.T) {
	p := &HTTPProvider{}
	WithMaxTokensField("max_completion_tokens")(p)
	if p.maxTokensField != "max_completion_tokens" {
		t.Fatalf("maxTokensField = %q, want %q", p.maxTokensField, "max_completion_tokens")
	}
}

func TestWithRequestTimeout(t *testing.T) {
	p := &HTTPProvider{}
	WithRequestTimeout(7 * time.Second)(p)
	c, ok := p.httpClient.(*DefaultHTTPClient)
	if !ok || c.client.Timeout != 7*time.Second {
		t.Fatalf("http timeout not set to 7s")
	}
}

func TestWithHTTPClient(t *testing.T) {
	m := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) { return nil, nil }}
	p := &HTTPProvider{}
	WithHTTPClient(m)(p)
	if p.httpClient != m {
		t.Fatalf("httpClient not set by WithHTTPClient")
	}
}

func TestWithHTTPClientNilPreservesProxy(t *testing.T) {
	p := NewProvider("k", "https://example.com/v1", "http://127.0.0.1:1080", WithHTTPClient(nil))
	c, ok := p.httpClient.(*DefaultHTTPClient)
	if !ok || c.client == nil {
		t.Fatalf("httpClient type = %T, want *DefaultHTTPClient", p.httpClient)
	}
	tr, ok := c.client.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("transport proxy is not configured")
	}
	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}
	proxyURL, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("proxy func returned error: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:1080" {
		t.Fatalf("proxy URL = %v, want http://127.0.0.1:1080", proxyURL)
	}
}

func TestNewProviderDefaults(t *testing.T) {
	p := NewProvider("k", "https://example.com/v1", "")
	if p == nil {
		t.Fatal("NewProvider returned nil")
	}
	if p.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
	if _, ok := p.httpClient.(*DefaultHTTPClient); !ok {
		t.Fatalf("httpClient type = %T, want *DefaultHTTPClient", p.httpClient)
	}
}

func TestHTTPProviderChatBuildsNonStreamingRequest(t *testing.T) {
	var body []byte
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		if !strings.HasSuffix(req.URL.String(), "/chat/completions") {
			t.Fatalf("url = %s, want suffix /chat/completions", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"choices":[{"message":{
					"content":"hello",
					"tool_calls":[{"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"hi\"}"}}]
				}}],
				"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}
			}`)),
			Header: make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	resp, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", ChatOptions{
		Tools: []ToolDefinition{{Type: "function", Function: ToolFunction{Name: "echo"}}},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp == nil || resp.Content != "hello" {
		t.Fatalf("Chat content = %v, want hello", resp)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 3 {
		t.Fatalf("Chat usage = %+v, want total_tokens=3", resp.Usage)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("Chat tool calls len = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function == nil || resp.ToolCalls[0].Function.Name != "echo" {
		t.Fatalf("Chat tool call function = %+v, want name=echo", resp.ToolCalls[0].Function)
	}
	if got := resp.ToolCalls[0].Arguments["text"]; got != "hi" {
		t.Fatalf("Chat tool call arguments[text] = %v, want hi", got)
	}
	if !bytes.Contains(body, []byte(`"stream":false`)) {
		t.Fatalf("request does not set stream=false: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`"tools"`)) || !bytes.Contains(body, []byte(`"tool_choice":"auto"`)) {
		t.Fatalf("request does not include tools/tool_choice=auto: %s", string(body))
	}
}

func TestHTTPProviderChatMapsZeroUsage(t *testing.T) {
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"choices":[{"message":{"content":"hello"}}],
				"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}
			}`)),
			Header: make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))
	resp, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions())
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("Chat usage = nil, want zero-valued usage")
	}
	if resp.Usage.PromptTokens != 0 || resp.Usage.CompletionTokens != 0 || resp.Usage.TotalTokens != 0 {
		t.Fatalf("Chat usage = %+v, want zeros", resp.Usage)
	}
}

func TestHTTPProviderChatPropagatesContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	_, err := p.Chat(ctx, []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions())
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestHTTPProviderChatMapsStatusCodeToSentinelError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{name: "401 unauthorized", status: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "429 rate limited", status: http.StatusTooManyRequests, want: ErrRateLimited},
		{name: "500 api error", status: http.StatusInternalServerError, want: ErrAPIError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.status,
					Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"boom"}}`)),
					Header:     make(http.Header),
				}, nil
			}}
			p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

			_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions())
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want sentinel %v", err, tt.want)
			}
		})
	}
}

func TestHTTPProviderChatInvalidJSONReturnsError(t *testing.T) {
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[`)),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions())
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestHTTPProviderChatStreamSSEAndDone(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(req.Body)
		if !bytes.Contains(payload, []byte(`"stream":true`)) {
			t.Fatalf("request does not set stream=true: %s", string(payload))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(stream)),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	var chunks []*LLMResponse
	err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions(),
		func(chunk *LLMResponse) error {
			chunks = append(chunks, chunk)
			return nil
		})
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want >= 2", len(chunks))
	}
	if !chunks[0].IsStreaming {
		t.Fatalf("first chunk IsStreaming = false, want true")
	}
	if !chunks[len(chunks)-1].IsDone {
		t.Fatalf("final chunk IsDone = false, want true")
	}
}

func TestHTTPProviderChatStreamParsesThinking(t *testing.T) {
	tests := []struct {
		name             string
		stream           string
		wantReasoning    string
		thinkingField    string // "thinking" or "reasoning_content"
	}{
		{
			name:          "GLM thinking field",
			stream:        `data: {"choices":[{"delta":{"thinking":"Let me think","content":"Hello"}}]}` + "\n\ndata: [DONE]\n\n",
			wantReasoning: "Let me think",
			thinkingField: "thinking",
		},
		{
			name:          "OpenAI reasoning_content field",
			stream:        `data: {"choices":[{"delta":{"reasoning_content":"Let me think","content":"Hello"}}]}` + "\n\ndata: [DONE]\n\n",
			wantReasoning: "Let me think",
			thinkingField: "reasoning_content",
		},
		{
			name:          "both thinking and content",
			stream:        `data: {"choices":[{"delta":{"thinking":"Reasoning...","content":"Final"}}]}` + "\n\ndata: [DONE]\n\n",
			wantReasoning: "Reasoning...",
			thinkingField: "thinking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(tt.stream)),
					Header:     make(http.Header),
				}, nil
			}}
			p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

			var gotReasoning string
			err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions(),
				func(chunk *LLMResponse) error {
					gotReasoning += chunk.ReasoningContent
					return nil
				})
			if err != nil {
				t.Fatalf("ChatStream error: %v", err)
			}
			if gotReasoning != tt.wantReasoning {
				t.Fatalf("ReasoningContent = %q, want %q", gotReasoning, tt.wantReasoning)
			}
		})
	}
}

func TestHTTPProviderChatStreamWithToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"hi\"}"}}]}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(stream)),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	var got *LLMResponse
	err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions(),
		func(chunk *LLMResponse) error {
			if !chunk.IsDone && len(chunk.ToolCalls) > 0 {
				got = chunk
			}
			return nil
		})
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}
	if got == nil {
		t.Fatal("did not receive chunk with tool calls")
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("tool calls len = %d, want 1", len(got.ToolCalls))
	}
	if got.ToolCalls[0].Function == nil || got.ToolCalls[0].Function.Name != "echo" {
		t.Fatalf("tool call function = %+v, want name=echo", got.ToolCalls[0].Function)
	}
	if got.ToolCalls[0].Arguments["text"] != "hi" {
		t.Fatalf("tool call arguments[text] = %v, want hi", got.ToolCalls[0].Arguments["text"])
	}
}

func TestHTTPProviderChatStreamTruncatedSSEReturnsError(t *testing.T) {
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}\n\n")),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions(),
		func(chunk *LLMResponse) error { return nil })
	if err == nil {
		t.Fatal("expected error for truncated SSE payload")
	}
}

func TestHTTPProviderChatStreamHandlerErrorStops(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(stream)),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	wantErr := errors.New("stop")
	err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", DefaultChatOptions(),
		func(chunk *LLMResponse) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestHTTPProviderGetDefaultModel(t *testing.T) {
	p := &HTTPProvider{}
	if got := p.GetDefaultModel(); got != "" {
		t.Fatalf("GetDefaultModel() = %q, want empty string", got)
	}
}

func BenchmarkHTTPProviderChat(b *testing.B) {
	responseBody := []byte(`{"choices":[{"message":{"content":"ok"}}]}`)
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(responseBody)),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))
	msgs := []Message{{Role: "user", Content: "hi"}}
	opts := DefaultChatOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.Chat(context.Background(), msgs, "gpt-4o", opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHTTPProviderChatStream(b *testing.B) {
	streamBody := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(streamBody)),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))
	msgs := []Message{{Role: "user", Content: "hi"}}
	opts := DefaultChatOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.ChatStream(context.Background(), msgs, "gpt-4o", opts, func(chunk *LLMResponse) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
