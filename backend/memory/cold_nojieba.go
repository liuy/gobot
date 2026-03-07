//go:build !jieba

package memory

func tokenizeChinese(content string) string {
	return content
}
