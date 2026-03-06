package providers

var minimaxParams = map[string]any{
	"temperature":    1.0,
	"top_p":          1.0,
	"max_tokens":     8192,
	"reasoning_split": true,
}

var minimaxCodingParams = map[string]any{
	"temperature":    1.0,
	"top_p":          1.0,
	"max_tokens":     16384,
	"reasoning_split": true,
}

func minimaxExtractReasoning(chunk map[string]any) string {
	// Try reasoning_details field first (when reasoning_split=true)
	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			// Check delta.reasoning_details
			if delta, ok := choice["delta"].(map[string]any); ok {
				if details, ok := delta["reasoning_details"].([]any); ok && len(details) > 0 {
					if detail, ok := details[0].(map[string]any); ok {
						if text, ok := detail["text"].(string); ok {
							return text
						}
					}
				}
			}
			// Check message.reasoning_details
			if msg, ok := choice["message"].(map[string]any); ok {
				if details, ok := msg["reasoning_details"].([]any); ok && len(details) > 0 {
					if detail, ok := details[0].(map[string]any); ok {
						if text, ok := detail["text"].(string); ok {
							return text
						}
					}
				}
			}
		}
	}
	return ""
}

func registerMinimax() {
	RegisterProvider("minimax", &minimaxBuilder{
		apiBase:          "https://api.minimaxi.com/v1",
		params:           minimaxParams,
		extractReasoning: minimaxExtractReasoning,
	})

	RegisterProvider("minimax-coding", &minimaxBuilder{
		apiBase:          "https://api.minimaxi.com/v1",
		params:           minimaxCodingParams,
		extractReasoning: minimaxExtractReasoning,
	})
}

type minimaxBuilder struct {
	apiBase          string
	params           map[string]any
	extractReasoning ExtractReasoningFunc
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
		WithExtractReasoning(b.extractReasoning),
	)
	return p, "", nil
}

func init() {
	registerMinimax()
}
