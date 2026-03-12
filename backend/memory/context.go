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

	hot, err := b.cache.GetHot()
	if err != nil {
		return nil, err
	}

	recent := b.cache.GetRecent(msg.ChatID, 20)

	ctx := &Context{
		Longterm: longterm,
		Hot:      hot,
		Recent:   recent,
	}

	totalTokens := countTokens(longterm)
	if hot != nil {
		totalTokens += countTokens(hotToText(hot))
	}
	for _, m := range recent {
		totalTokens += countTokens(m.Content)
	}
	totalTokens += countTokens(msg.Content)

	if totalTokens > b.maxTokens {
		if hot != nil {
			hotTokens := countTokens(hotToText(hot))
			if float64(hotTokens)/float64(totalTokens) >= 0.2 {
				ctx.Hot = nil
				totalTokens -= hotTokens
			}
		}

		for totalTokens > b.maxTokens && len(ctx.Recent) > 0 {
			oldest := ctx.Recent[len(ctx.Recent)-1]
			totalTokens -= countTokens(oldest.Content)
			ctx.Recent = ctx.Recent[:len(ctx.Recent)-1]
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

func hotToText(hot *HotMemoryData) string {
	if hot == nil {
		return ""
	}

	var text string
	for _, topic := range hot.ActiveTopics {
		text += topic.Name + " "
	}
	for _, kw := range hot.RecentKeywords {
		text += kw.Word + " "
	}
	return text
}
