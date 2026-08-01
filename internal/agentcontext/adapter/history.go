// Package adapter 将现有 Repository/Cache 适配为核心 Provider 接口。
// Adapter 只依赖窄接口，不直接暴露 cache.ChatCache 或 Repository 类型到核心包。
package adapter

import (
	"context"
	"fmt"

	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/internal/model/entity"
	"YoudaoNoteLm/pkg/logger"

	"go.uber.org/zap"
)

// RecentRoundsLimit 缓存中保留的最近对话轮数（与 Legacy ContextBuilder 一致）
const RecentRoundsLimit = 10

// ChatCacheReader 定义 HistoryProvider 实际使用的缓存方法子集。
// 生产适配器接收 *cache.ChatCache；测试可替换为 fake 实现。
type ChatCacheReader interface {
	GetRecentMessages(ctx context.Context, conversationID uint) ([]MessagePair, error)
	GetSummary(ctx context.Context, conversationID uint) (string, bool, error)
}

// MessagePair 消息对（与 cache.MessagePair 结构一致，避免直接依赖）
type MessagePair struct {
	User      string
	Assistant string
}

// ConversationSummaryReader 读取会话摘要的窄接口
type ConversationSummaryReader interface {
	FindByID(id uint) (*entity.Conversation, error)
}

// MessageHistoryReader 读取消息历史的窄接口
type MessageHistoryReader interface {
	FindRecentByConversationID(conversationID uint, limit int) ([]*entity.Message, error)
}

// LegacyHistoryProvider 包装现有 ChatCache 和 Repository，实现 HistoryProvider 接口。
// 保留 Redis → MySQL 回退以及 Provider 自己拥有的缓存回填。
// 不把当前用户输入混入历史结果；不建立第二份数据缓存。
type LegacyHistoryProvider struct {
	cache    ChatCacheReader
	convRepo ConversationSummaryReader
	msgRepo  MessageHistoryReader
}

// NewLegacyHistoryProvider 创建历史提供者
func NewLegacyHistoryProvider(
	cache ChatCacheReader,
	convRepo ConversationSummaryReader,
	msgRepo MessageHistoryReader,
) *LegacyHistoryProvider {
	return &LegacyHistoryProvider{
		cache:    cache,
		convRepo: convRepo,
		msgRepo:  msgRepo,
	}
}

// LoadHistory 加载会话历史，返回摘要、最近消息和来源元数据。
// 复用现有读取语义：缓存优先，数据库降级。
func (p *LegacyHistoryProvider) LoadHistory(ctx context.Context, query agentcontext.HistoryQuery) (agentcontext.HistorySnapshot, error) {
	if query.ConversationID == 0 {
		return agentcontext.HistorySnapshot{}, nil
	}

	var snapshot agentcontext.HistorySnapshot

	// 1. 加载摘要（缓存优先，数据库降级）
	summary, source, err := p.loadSummary(ctx, query.ConversationID)
	if err != nil {
		logger.Warn("[LegacyHistoryProvider] 加载摘要失败",
			zap.Uint("conversationID", query.ConversationID),
			zap.Error(err),
		)
		// 摘要失败不阻断，继续加载历史
	}
	if summary != "" {
		snapshot.Summary = &agentcontext.ConversationSummary{
			Content: summary,
			// 首期不追踪 ThroughMessageID/ThroughSequence/Version，
			// 因为 Legacy 模式没有精确的摘要边界
		}
		_ = source // 来源信息可用于后续 observability
	}

	// 2. 加载最近消息（缓存优先，数据库降级）
	messages, err := p.loadRecentMessages(ctx, query.ConversationID, query.Limit)
	if err != nil {
		return agentcontext.HistorySnapshot{}, fmt.Errorf("加载历史消息失败: %w", err)
	}

	// 3. 转换为 HistoryMessage（不含当前用户输入）
	snapshot.Messages = make([]agentcontext.HistoryMessage, 0, len(messages))
	for _, pair := range messages {
		snapshot.Messages = append(snapshot.Messages, agentcontext.HistoryMessage{
			Role:    "user",
			Content: pair.User,
		})
		snapshot.Messages = append(snapshot.Messages, agentcontext.HistoryMessage{
			Role:    "assistant",
			Content: pair.Assistant,
		})
	}

	return snapshot, nil
}

// loadSummary 加载摘要，缓存优先，数据库降级
func (p *LegacyHistoryProvider) loadSummary(ctx context.Context, conversationID uint) (string, string, error) {
	// 1. 尝试从缓存获取
	summary, found, err := p.cache.GetSummary(ctx, conversationID)
	if err == nil && found && summary != "" {
		return summary, "cache", nil
	}

	if err != nil {
		logger.Warn("[LegacyHistoryProvider] 从缓存获取摘要失败，降级到数据库",
			zap.Uint("conversationID", conversationID),
			zap.Error(err),
		)
	}

	// 2. 降级：从数据库获取
	conv, dbErr := p.convRepo.FindByID(conversationID)
	if dbErr != nil || conv == nil {
		return "", "none", dbErr
	}

	return conv.Summary, "database", nil
}

// loadRecentMessages 加载最近消息，缓存优先，数据库降级
func (p *LegacyHistoryProvider) loadRecentMessages(ctx context.Context, conversationID uint, limit int) ([]MessagePair, error) {
	if limit <= 0 {
		limit = RecentRoundsLimit
	}

	// 1. 尝试从缓存获取
	cachePairs, err := p.cache.GetRecentMessages(ctx, conversationID)
	if err == nil && len(cachePairs) > 0 {
		return toMessagePairs(cachePairs, limit), nil
	}

	if err != nil {
		logger.Warn("[LegacyHistoryProvider] 从缓存获取消息失败，降级到数据库",
			zap.Uint("conversationID", conversationID),
			zap.Error(err),
		)
	}

	// 2. 降级：从数据库获取
	msgs, dbErr := p.msgRepo.FindRecentByConversationID(conversationID, limit*2)
	if dbErr != nil {
		return nil, dbErr
	}

	return convertToMessagePairs(msgs, limit), nil
}

// toMessagePairs 将缓存 MessagePair 转为 adapter MessagePair，并截断到 limit
func toMessagePairs(pairs []MessagePair, limit int) []MessagePair {
	if len(pairs) > limit {
		pairs = pairs[len(pairs)-limit:]
	}
	return pairs
}

// convertToMessagePairs 将数据库消息转为 MessagePair（与 Legacy ContextBuilder 逻辑一致）
func convertToMessagePairs(msgs []*entity.Message, limit int) []MessagePair {
	var pairs []MessagePair
	var pendingUserMsg string
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			pendingUserMsg = msg.Content
		case "assistant":
			pairs = append(pairs, MessagePair{
				User:      pendingUserMsg,
				Assistant: msg.Content,
			})
			pendingUserMsg = ""
		}
	}
	if len(pairs) > limit {
		pairs = pairs[len(pairs)-limit:]
	}
	return pairs
}
