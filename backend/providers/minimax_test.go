package providers

import (
	"testing"
)

func TestMinimaxExtractReasoning(t *testing.T) {
	tests := []struct {
		name  string
		chunk map[string]any
		want  string
	}{
		{
			name: "delta reasoning_details",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"reasoning_details": []any{
								map[string]any{"text": "reasoning from details"},
							},
						},
					},
				},
			},
			want: "reasoning from details",
		},
		{
			name: "message reasoning_details",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"message": map[string]any{
							"reasoning_details": []any{
								map[string]any{"text": "non-stream reasoning"},
							},
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
						"delta": map[string]any{
							"reasoning_details": []any{
								map[string]any{"text": "from delta"},
							},
						},
						"message": map[string]any{
							"reasoning_details": []any{
								map[string]any{"text": "from message"},
							},
						},
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
			name: "empty reasoning_details",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"reasoning_details": []any{},
						},
					},
				},
			},
			want: "",
		},
		{
			name: "reasoning_details with empty text",
			chunk: map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"reasoning_details": []any{
								map[string]any{"text": ""},
							},
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
				t.Fatalf("minimaxExtractReasoning() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
