package memory

type ContextBuilder struct {
	cache     *MemoryCache
	maxTokens int
}

func NewContextBuilder(cache *MemoryCache, maxTokens int) *ContextBuilder {
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	return &ContextBuilder{
		cache:     cache,
		maxTokens: maxTokens,
	}
}

func (b *ContextBuilder) Build(msg Message) (*Context, error) {
	longterm, err := b.cache.GetLongterm()
	if err != nil {
		return nil, err
	}

	recent := b.cache.GetRecent(msg.ChatID, 20)

	ctx := &Context{
		Longterm: longterm,
		Recent:   recent,
	}

	totalTokens := countTokens(longterm)
	for _, m := range recent {
		totalTokens += countTokens(ExtractTextFromContent(m.Content))
	}
	totalTokens += countTokens(ExtractTextFromContent(msg.Content))

	if totalTokens > b.maxTokens {
		for totalTokens > b.maxTokens && len(ctx.Recent) > 0 {
			// Drop oldest messages first (from the front), keep newest messages
			oldest := ctx.Recent[0]
			totalTokens -= countTokens(ExtractTextFromContent(oldest.Content))
			ctx.Recent = ctx.Recent[1:]
		}
	}

	return ctx, nil
}

func countTokens(text string) int {
	if text == "" {
		return 0
	}

	runes := []rune(text)
	nonASCII := 0
	for _, r := range runes {
		if r > 127 {
			nonASCII++
		}
	}

	if nonASCII > len(runes)/2 {
		if len(runes) < 1 {
			return 1
		}
		return len(runes)
	}

	tokens := len(text) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}
