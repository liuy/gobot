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
