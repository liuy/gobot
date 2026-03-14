package providers

import (
	"testing"
)

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
