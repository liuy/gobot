package providers

import "testing"

func TestZhipuConstants(t *testing.T) {
	if DefaultZhipuAPIBase != "https://open.bigmodel.cn/api/paas/v4" {
		t.Fatalf("DefaultZhipuAPIBase = %q", DefaultZhipuAPIBase)
	}
	if DefaultZhipuCodingAPIBase != "https://open.bigmodel.cn/api/coding/paas/v4" {
		t.Fatalf("DefaultZhipuCodingAPIBase = %q", DefaultZhipuCodingAPIBase)
	}
	if DefaultZhipuModel != "glm-5" {
		t.Fatalf("DefaultZhipuModel = %q", DefaultZhipuModel)
	}
	if DefaultZhipuMaxTokens != 131072 {
		t.Fatalf("DefaultZhipuMaxTokens = %d", DefaultZhipuMaxTokens)
	}
	if DefaultZhipuMaxTokensField != "max_tokens" {
		t.Fatalf("DefaultZhipuMaxTokensField = %q", DefaultZhipuMaxTokensField)
	}
	if !DefaultZhipuThinkingEnabled {
		t.Fatal("DefaultZhipuThinkingEnabled = false, want true")
	}
}
