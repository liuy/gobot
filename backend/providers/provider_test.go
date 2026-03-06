package providers

import (
	"context"
	"errors"
	"testing"
)

type stubProvider struct{}

func (s *stubProvider) Chat(context.Context, []Message, string, map[string]any) (*LLMResponse, error) {
	return &LLMResponse{Content: "ok"}, nil
}

func (s *stubProvider) ChatStream(context.Context, []Message, string, map[string]any, StreamHandler) error {
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

func TestSentinelErrorsDefined(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "ErrUnauthorized", err: ErrUnauthorized},
		{name: "ErrRateLimited", err: ErrRateLimited},
		{name: "ErrAPIError", err: ErrAPIError},
		{name: "ErrContextCanceled", err: ErrContextCanceled},
		{name: "ErrModelNotFound", err: ErrModelNotFound},
		{name: "ErrNetworkError", err: ErrNetworkError},
		{name: "ErrTimeout", err: ErrTimeout},
		{name: "ErrInvalidResponse", err: ErrInvalidResponse},
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
		{"minimax prefixed", "minimax/MiniMax-M2.5", "minimax", "MiniMax-M2.5"},
		{"no prefix returns empty", "gpt-4o", "", "gpt-4o"},
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
	orig := providerRegistry
	providerRegistry = make(map[string]ProviderBuilder)
	t.Cleanup(func() { providerRegistry = orig })

	if p, m, err := CreateProvider(nil); err == nil || p != nil || m != "" {
		t.Fatalf("CreateProvider(nil) = (%v, %q, %v), want (nil, \"\", error)", p, m, err)
	}

	cfg := &ModelConfig{APIKey: "k"}
	if p, m, err := CreateProvider(cfg); err == nil || p != nil || m != "" {
		t.Fatalf("CreateProvider(empty model) = (%v, %q, %v), want (nil, \"\", error)", p, m, err)
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
		t.Fatalf("provider mismatch")
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

func TestCreateProviderUnregisteredFailsFast(t *testing.T) {
	orig := providerRegistry
	providerRegistry = make(map[string]ProviderBuilder)
	t.Cleanup(func() { providerRegistry = orig })

	cfg := &ModelConfig{
		Model:  "unknown/model",
		APIKey: "k",
	}
	_, _, err := CreateProvider(cfg)
	if err == nil {
		t.Fatal("expected error for unregistered provider")
	}
	if err.Error() != "provider not registered: unknown" {
		t.Fatalf("error = %q, want %q", err.Error(), "provider not registered: unknown")
	}
}
