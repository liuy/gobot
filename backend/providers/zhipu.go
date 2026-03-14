package providers

var zhipuParams = map[string]any{
	"do_sample":   true,
	"temperature": 0.7,
	"top_p":       1.0,
	"max_tokens":  65536,
	"thinking":    map[string]string{"type": "enabled"},
}

var zhipuCodingParams = map[string]any{
	"do_sample":   true,
	"temperature": 0.2,
	"top_p":       1.0,
	"max_tokens":  131072,
	"thinking":    map[string]string{"type": "enabled"},
}

func registerZhipu() {
	RegisterProvider("zhipu", &zhipuBuilder{
		apiBase: "https://open.bigmodel.cn/api/paas/v4",
		params:  zhipuParams,
	})

	RegisterProvider("zhipu-coding", &zhipuBuilder{
		apiBase: "https://open.bigmodel.cn/api/coding/paas/v4",
		params:  zhipuCodingParams,
	})
}

type zhipuBuilder struct {
	apiBase string
	params  map[string]any
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
	)
	return p, "", nil
}

func init() {
	registerZhipu()
}
