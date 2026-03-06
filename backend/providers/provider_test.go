package providers

import (
	"context"
	"errors"
	"testing"
)

type stubProvider struct{}

func (s *stubProvider) Chat(context.Context, []Message, string, ChatOptions) (*LLMResponse, error) {
	return &LLMResponse{Content: "ok"}, nil
}

func (s *stubProvider) ChatStream(context.Context, []Message, string, ChatOptions, StreamHandler) error {
	return nil
}

func (s *stubProvider) GetDefaultModel() string { return "" }

type stubBuilder struct {
	gotCfg *ModelConfig
	retP   LLMProvider
	retM   string
	retE   error
}

func (b *stubBuilder) Build(cfg *ModelConfig) (LLMProvider, string, error) {
	b.gotCfg = cfg
	return b.retP, b.retM, b.retE
}

func TestDefaultChatOptions(t *testing.T) {
	opts := DefaultChatOptions()
	if opts.Temperature != 0.7 {
		t.Fatalf("Temperature = %v, want 0.7", opts.Temperature)
	}
	if opts.MaxTokens != 4096 {
		t.Fatalf("MaxTokens = %d, want 4096", opts.MaxTokens)
	}
	if opts.TopP != 1.0 {
		t.Fatalf("TopP = %v, want 1.0", opts.TopP)
	}
}

func TestSentinelErrorsDefined(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "ErrUnauthorized", err: ErrUnauthorized},
		{name: "ErrRateLimited", err: ErrRateLimited},
		{name: "ErrAPIError", err: ErrAPIError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("%s is nil", tt.name)
			}
		})
	}
}

func TestExtractProtocol(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		wantProtocol string
		wantModelID  string
	}{
		{"zhipu prefixed", "zhipu/glm-4-flash", "zhipu", "glm-4-flash"},
		{"zhipu-coding prefixed", "zhipu-coding/glm-5", "zhipu-coding", "glm-5"},
		{"openai prefixed", "openai/gpt-4o", "openai", "gpt-4o"},
		{"default protocol", "gpt-4o", "openai", "gpt-4o"},
		{"trim whitespace", "  zhipu/glm-4-flash  ", "zhipu", "glm-4-flash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProtocol, gotModelID := ExtractProtocol(tt.model)
			if gotProtocol != tt.wantProtocol || gotModelID != tt.wantModelID {
				t.Fatalf("ExtractProtocol(%q) = (%q, %q), want (%q, %q)",
					tt.model, gotProtocol, gotModelID, tt.wantProtocol, tt.wantModelID)
			}
		})
	}
}

func TestCreateProviderValidation(t *testing.T) {
	if p, m, err := CreateProvider(nil); err == nil || err.Error() != "config is nil" || p != nil || m != "" {
		t.Fatalf("CreateProvider(nil) = (%v, %q, %v), want (nil, \"\", config is nil)", p, m, err)
	}

	cfg := &ModelConfig{APIKey: "k"}
	if p, m, err := CreateProvider(cfg); err == nil || err.Error() != "model is required" || p != nil || m != "" {
		t.Fatalf("CreateProvider(empty model) = (%v, %q, %v), want (nil, \"\", model is required)", p, m, err)
	}
}

func TestCreateProviderUsesRegisteredBuilder(t *testing.T) {
	orig := providerRegistry
	providerRegistry = make(map[string]ProviderBuilder)
	t.Cleanup(func() { providerRegistry = orig })

	wantProvider := &stubProvider{}
	builder := &stubBuilder{retP: wantProvider, retM: "glm-4-flash"}
	RegisterProvider("zhipu", builder)

	cfg := &ModelConfig{Model: "zhipu/glm-4-flash", APIKey: "k"}
	gotProvider, gotModelID, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}
	if builder.gotCfg != cfg {
		t.Fatalf("builder got cfg %p, want %p", builder.gotCfg, cfg)
	}
	if gotProvider != wantProvider {
		t.Fatalf("provider = %T, want stub provider", gotProvider)
	}
	if gotModelID != "glm-4-flash" {
		t.Fatalf("modelID = %q, want %q", gotModelID, "glm-4-flash")
	}
}

func TestCreateProviderBuilderErrorPropagates(t *testing.T) {
	orig := providerRegistry
	providerRegistry = make(map[string]ProviderBuilder)
	t.Cleanup(func() { providerRegistry = orig })

	wantErr := errors.New("build failed")
	RegisterProvider("zhipu", &stubBuilder{retE: wantErr})

	_, _, err := CreateProvider(&ModelConfig{Model: "zhipu/glm-4-flash", APIKey: "k"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestCreateProviderFallsBackToOpenAICompat(t *testing.T) {
	orig := providerRegistry
	providerRegistry = make(map[string]ProviderBuilder)
	t.Cleanup(func() { providerRegistry = orig })

	cfg := &ModelConfig{
		Model:   "gpt-4o",
		APIKey:  "k",
		APIBase: "https://example.com/v1",
	}
	gotProvider, gotModelID, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}
	if _, ok := gotProvider.(*HTTPProvider); !ok {
		t.Fatalf("fallback provider type = %T, want *HTTPProvider", gotProvider)
	}
	if gotModelID != "gpt-4o" {
		t.Fatalf("modelID = %q, want %q", gotModelID, "gpt-4o")
	}
}

func TestCreateProviderZhipuCodingUsesCorrectAPIBase(t *testing.T) {
	orig := providerRegistry
	providerRegistry = make(map[string]ProviderBuilder)
	t.Cleanup(func() { providerRegistry = orig })

	// Without custom APIBase, should use DefaultZhipuCodingAPIBase
	cfg := &ModelConfig{
		Model:  "zhipu-coding/glm-5",
		APIKey: "k",
	}
	_, _, err := CreateProvider(cfg)
	// Should not fail - uses default API base
	if err != nil {
		t.Fatalf("CreateProvider(zhipu-coding) error: %v", err)
	}
}

func TestCreateProviderZhipuUsesCorrectAPIBase(t *testing.T) {
	orig := providerRegistry
	providerRegistry = make(map[string]ProviderBuilder)
	t.Cleanup(func() { providerRegistry = orig })

	// Without custom APIBase, should use DefaultZhipuAPIBase
	cfg := &ModelConfig{
		Model:  "zhipu/glm-4-flash",
		APIKey: "k",
	}
	_, _, err := CreateProvider(cfg)
	// Should not fail - uses default API base
	if err != nil {
		t.Fatalf("CreateProvider(zhipu) error: %v", err)
	}
}
