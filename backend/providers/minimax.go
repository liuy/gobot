package providers

var minimaxParams = map[string]any{
	"temperature":    0.2,
	"top_p":          0.1,
	"max_tokens":     16384,
	"reasoning_split": true,
}

func registerMinimax() {
	RegisterProvider("minimax", &minimaxBuilder{
		apiBase: "https://api.minimaxi.com/v1",
		params:  minimaxParams,
	})
}

type minimaxBuilder struct {
	apiBase string
	params  map[string]any
}

func (b *minimaxBuilder) Build(cfg *ModelConfig) (LLMProvider, string, error) {
	apiBase := cfg.APIBase
	if apiBase == "" {
		apiBase = b.apiBase
	}

	p := NewProvider(
		cfg.APIKey,
		apiBase,
		cfg.Proxy,
		WithParams(b.params),
	)
	return p, "", nil
}

func init() {
	registerMinimax()
}
