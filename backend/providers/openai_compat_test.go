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

	"gobot/types"
)

type mockHTTPClient struct {
	do func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.do(req)
}

func TestWithHTTPClient(t *testing.T) {
	m := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) { return nil, nil }}
	p := &HTTPProvider{}
	WithHTTPClient(m)(p)
	if p.httpClient != m {
		t.Fatalf("httpClient not set by WithHTTPClient")
	}
}

func TestWithParams(t *testing.T) {
	params := map[string]any{"temperature": 0.5, "max_tokens": 1000}
	p := &HTTPProvider{}
	WithParams(params)(p)
	if p.params["temperature"] != 0.5 {
		t.Fatalf("params temperature = %v, want 0.5", p.params["temperature"])
	}
	if p.params["max_tokens"] != 1000 {
		t.Fatalf("params max_tokens = %v, want 1000", p.params["max_tokens"])
	}
}

func TestWithExtractReasoning(t *testing.T) {
	f := func(chunk map[string]any) string { return "test" }
	p := &HTTPProvider{}
	WithExtractReasoning(f)(p)
	if p.extractReasoning == nil {
		t.Fatal("extractReasoning not set")
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

	resp, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", map[string]any{
		"tools": []ToolDefinition{{Type: "function", Function: ToolFunction{Name: "echo"}}},
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
	resp, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
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

	_, err := p.Chat(ctx, []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
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

			_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
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

	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
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

	ch, err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}

	var chunks []types.StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want >= 2", len(chunks))
	}
	if chunks[len(chunks)-1].StopReason == "" {
		t.Fatalf("final chunk StopReason = empty, want 'stop'")
	}
}

func TestHTTPProviderChatStreamParsesThinking(t *testing.T) {
	tests := []struct {
		name          string
		stream        string
		wantReasoning string
	}{
		{
			name:          "GLM thinking field",
			stream:        `data: {"choices":[{"delta":{"thinking":"Let me think","content":"Hello"}}]}` + "\n\ndata: [DONE]\n\n",
			wantReasoning: "Let me think",
		},
		{
			name:          "OpenAI reasoning_content field",
			stream:        `data: {"choices":[{"delta":{"reasoning_content":"Let me think","content":"Hello"}}]}` + "\n\ndata: [DONE]\n\n",
			wantReasoning: "Let me think",
		},
		{
			name:          "both thinking and content",
			stream:        `data: {"choices":[{"delta":{"thinking":"Reasoning...","content":"Final"}}]}` + "\n\ndata: [DONE]\n\n",
			wantReasoning: "Reasoning...",
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

			ch, err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
			if err != nil {
				t.Fatalf("ChatStream error: %v", err)
			}

			var gotReasoning string
			for chunk := range ch {
				gotReasoning += chunk.Thinking
			}
			if gotReasoning != tt.wantReasoning {
				t.Fatalf("Reasoning = %q, want %q", gotReasoning, tt.wantReasoning)
			}
		})
	}
}

func TestHTTPProviderChatStreamWithToolCalls(t *testing.T) {
	// Note: StreamChunk no longer includes tool calls - they are handled internally by provider
	// This test now just verifies the stream completes without error
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":""}}]}`,
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

	ch, err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}

	var gotDone bool
	for chunk := range ch {
		if chunk.StopReason != "" {
			gotDone = true
		}
	}
	if !gotDone {
		t.Fatal("did not receive StopReason chunk")
	}
}

func TestHTTPProviderChatStreamTruncatedSSEHandled(t *testing.T) {
	// Truncated SSE is now silently ignored in the goroutine
	// The channel will just close without StopReason
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}\n\n")),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	ch, err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}

	// Just consume the channel - it should close without error
	for range ch {
	}
}

func TestHTTPProviderChatStreamConsumesAll(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\ndata: [DONE]\n\n"
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(stream)),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client))

	ch, err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}

	var gotContent string
	var gotDone bool
	for chunk := range ch {
		gotContent += chunk.Content
		if chunk.StopReason != "" {
			gotDone = true
		}
	}
	if gotContent != "x" {
		t.Fatalf("content = %q, want %q", gotContent, "x")
	}
	if !gotDone {
		t.Fatal("did not get StopReason")
	}
}

func TestHTTPProviderGetDefaultModel(t *testing.T) {
	p := &HTTPProvider{}
	if got := p.GetDefaultModel(); got != "" {
		t.Fatalf("GetDefaultModel() = %q, want empty string", got)
	}
}

func TestHTTPProviderParamsMerging(t *testing.T) {
	var body []byte
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
			Header:     make(http.Header),
		}, nil
	}}
	defaultParams := map[string]any{
		"temperature": 0.7,
		"max_tokens":  1000,
	}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client), WithParams(defaultParams))

	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", map[string]any{
		"temperature": 0.5,
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if !bytes.Contains(body, []byte(`"temperature":0.5`)) {
		t.Fatalf("runtime params did not override default: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`"max_tokens":1000`)) {
		t.Fatalf("default params not preserved: %s", string(body))
	}
}

func TestHTTPProviderExtractReasoningCalled(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"content":"Hello"}}]}` + "\n\ndata: [DONE]\n\n"
	called := false
	extractFunc := func(chunk map[string]any) string {
		called = true
		return "extracted reasoning"
	}
	client := &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(stream)),
			Header:     make(http.Header),
		}, nil
	}}
	p := NewProvider("k", "https://example.com/v1", "", WithHTTPClient(client), WithExtractReasoning(extractFunc))

	ch, err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}

	var gotReasoning string
	for chunk := range ch {
		gotReasoning += chunk.Thinking
	}
	if !called {
		t.Fatal("extractReasoning was not called")
	}
	if gotReasoning != "extracted reasoning" {
		t.Fatalf("Reasoning = %q, want %q", gotReasoning, "extracted reasoning")
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.Chat(context.Background(), msgs, "gpt-4o", nil); err != nil {
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch, err := p.ChatStream(context.Background(), msgs, "gpt-4o", nil)
		if err != nil {
			b.Fatal(err)
		}
		for range ch {
		}
	}
}
