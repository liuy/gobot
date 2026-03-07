//go:build jieba

package memory

import (
	"strings"
	"sync"

	"github.com/yanyiwu/gojieba"
)

var (
	jieba     *gojieba.Jieba
	jiebaOnce sync.Once
)

func tokenizeChinese(content string) string {
	jiebaOnce.Do(func() {
		jieba = gojieba.NewJieba()
	})

	// Use CutForSearch for better tokenization
	// This ensures sub-tokens are also indexed (e.g., "今天天气" -> "今天", "天天", "天气", "今天天气")
	words := jieba.CutForSearch(content, false)
	tokenized := strings.Join(words, " ")
	return tokenized
}
