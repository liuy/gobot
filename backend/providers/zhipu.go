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

func zhipuExtractReasoning(chunk map[string]any) string {
	choices, ok := chunk["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}

	choice, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}

	if delta, ok := choice["delta"].(map[string]any); ok {
		if thinking, ok := delta["thinking"].(string); ok && thinking != "" {
			return thinking
		}
	}

	if msg, ok := choice["message"].(map[string]any); ok {
		if thinking, ok := msg["thinking"].(string); ok && thinking != "" {
			return thinking
		}
	}

	return ""
}

func registerZhipu() {
	RegisterProvider("zhipu", &zhipuBuilder{
		apiBase:          "https://open.bigmodel.cn/api/paas/v4",
		params:           zhipuParams,
		extractReasoning: zhipuExtractReasoning,
	})

	RegisterProvider("zhipu-coding", &zhipuBuilder{
		apiBase:          "https://open.bigmodel.cn/api/coding/paas/v4",
		params:           zhipuCodingParams,
		extractReasoning: zhipuExtractReasoning,
	})
}

type zhipuBuilder struct {
	apiBase          string
	params           map[string]any
	extractReasoning ExtractReasoningFunc
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
		WithExtractReasoning(b.extractReasoning),
	)
	return p, "", nil
}

func init() {
	registerZhipu()
}
