package providers

import (
	"testing"
)

func TestMinimaxParamsDefined(t *testing.T) {
	if minimaxParams["temperature"] != 0.2 {
		t.Fatalf("minimaxParams temperature = %v, want 0.2", minimaxParams["temperature"])
	}
	if minimaxParams["top_p"] != 0.1 {
		t.Fatalf("minimaxParams top_p = %v, want 0.1", minimaxParams["top_p"])
	}
	if minimaxParams["max_tokens"] != 16384 {
		t.Fatalf("minimaxParams max_tokens = %v, want 16384", minimaxParams["max_tokens"])
	}
	if minimaxParams["reasoning_split"] != true {
		t.Fatalf("minimaxParams reasoning_split = %v, want true", minimaxParams["reasoning_split"])
	}
}

func TestMinimaxBuilderRegistered(t *testing.T) {
	if _, ok := providerRegistry["minimax"]; !ok {
		t.Fatal("minimax provider not registered")
	}
}

func TestMinimaxExtractReasoning(t *testing.T) {
	tests := []struct {
		name     string
		chunk    map[string]any
		want     string
	}{
		{
			name: "streaming delta reasoning_details",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"reasoning_details": []any{
								map[string]any{"text": "Step 1: thinking", "type": "reasoning.text"},
							},
						},
					},
				},
			},
			want: "Step 1: thinking",
		},
		{
			name: "non-streaming message reasoning_details",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"message": map[string]any{
							"reasoning_details": []any{
								map[string]any{"text": "Reasoning from message", "type": "reasoning.text"},
							},
						},
					},
				},
			},
			want: "Reasoning from message",
		},
		{
			name: "empty choices",
			chunk: map[string]any{
				"choices": []any{},
			},
			want: "",
		},
		{
			name: "no reasoning",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"content": "Hello",
						},
					},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minimaxExtractReasoning(tt.chunk)
			if got != tt.want {
				t.Errorf("minimaxExtractReasoning() = %q, want %q", got, tt.want)
			}
		})
	}
}
