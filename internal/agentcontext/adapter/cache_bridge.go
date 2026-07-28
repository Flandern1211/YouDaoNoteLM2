package adapter

import (
	"context"

	"YoudaoNoteLm/pkg/cache"
)

// ChatCacheBridge 将现有 cache.ChatCache 的返回类型映射到窄 HistoryProvider 端口。
type ChatCacheBridge struct {
	cache *cache.ChatCache
}

func NewChatCacheBridge(chatCache *cache.ChatCache) *ChatCacheBridge {
	return &ChatCacheBridge{cache: chatCache}
}

func (b *ChatCacheBridge) GetRecentMessages(
	ctx context.Context,
	conversationID uint,
) ([]MessagePair, error) {
	pairs, err := b.cache.GetRecentMessages(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	result := make([]MessagePair, 0, len(pairs))
	for _, pair := range pairs {
		result = append(result, MessagePair{
			User:      pair.User,
			Assistant: pair.Assistant,
		})
	}
	return result, nil
}

func (b *ChatCacheBridge) GetSummary(
	ctx context.Context,
	conversationID uint,
) (string, bool, error) {
	return b.cache.GetSummary(ctx, conversationID)
}
