package providers

import (
	"testing"
)

func TestZhipuExtractReasoning(t *testing.T) {
	tests := []struct {
		name  string
		chunk map[string]any
		want  string
	}{
		{
			name: "delta thinking",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"thinking": "reasoning content here",
						},
					},
				},
			},
			want: "reasoning content here",
		},
		{
			name: "message thinking",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"message": map[string]any{
							"thinking": "non-stream reasoning",
						},
					},
				},
			},
			want: "non-stream reasoning",
		},
		{
			name: "delta takes precedence over message",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"delta":   map[string]any{"thinking": "from delta"},
						"message": map[string]any{"thinking": "from message"},
					},
				},
			},
			want: "from delta",
		},
		{
			name: "empty choices",
			chunk: map[string]any{
				"choices": []any{},
			},
			want: "",
		},
		{
			name: "no choices field",
			chunk: map[string]any{
				"data": "something",
			},
			want: "",
		},
		{
			name: "empty thinking",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"thinking": "",
						},
					},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zhipuExtractReasoning(tt.chunk)
			if got != tt.want {
				t.Fatalf("zhipuExtractReasoning() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZhipuParamsDefined(t *testing.T) {
	if zhipuParams["temperature"] != 0.7 {
		t.Fatalf("zhipuParams temperature = %v, want 0.7", zhipuParams["temperature"])
	}
	if zhipuParams["max_tokens"] != 65536 {
		t.Fatalf("zhipuParams max_tokens = %v, want 65536", zhipuParams["max_tokens"])
	}
	if zhipuCodingParams["max_tokens"] != 131072 {
		t.Fatalf("zhipuCodingParams max_tokens = %v, want 131072", zhipuCodingParams["max_tokens"])
	}
}

func TestZhipuBuilderRegistered(t *testing.T) {
	if _, ok := providerRegistry["zhipu"]; !ok {
		t.Fatal("zhipu provider not registered")
	}
	if _, ok := providerRegistry["zhipu-coding"]; !ok {
		t.Fatal("zhipu-coding provider not registered")
	}
}
