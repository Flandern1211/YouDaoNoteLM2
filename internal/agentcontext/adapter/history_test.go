package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/internal/model/entity"
)

// --- fakes ---

type fakeChatCache struct {
	pairs      []MessagePair
	summary    string
	found      bool
	pairsErr   error
	summaryErr error
}

func (f *fakeChatCache) GetRecentMessages(_ context.Context, _ uint) ([]MessagePair, error) {
	return f.pairs, f.pairsErr
}

func (f *fakeChatCache) GetSummary(_ context.Context, _ uint) (string, bool, error) {
	return f.summary, f.found, f.summaryErr
}

type fakeConvRepo struct {
	conv *entity.Conversation
	err  error
}

func (f *fakeConvRepo) FindByID(_ uint) (*entity.Conversation, error) {
	return f.conv, f.err
}

type fakeMsgRepo struct {
	msgs []*entity.Message
	err  error
}

func (f *fakeMsgRepo) FindRecentByConversationID(_ uint, _ int) ([]*entity.Message, error) {
	return f.msgs, f.err
}

// --- tests ---

func TestLegacyHistoryProvider_CacheHit(t *testing.T) {
	cache := &fakeChatCache{
		pairs: []MessagePair{
			{User: "hello", Assistant: "hi"},
			{User: "how are you", Assistant: "I'm fine"},
		},
		summary: "previous summary",
		found:   true,
	}

	provider := NewLegacyHistoryProvider(cache, &fakeConvRepo{}, &fakeMsgRepo{})

	snapshot, err := provider.LoadHistory(context.Background(), agentcontext.HistoryQuery{
		ConversationID: 1,
		Limit:          10,
	})

	require.NoError(t, err)
	assert.NotNil(t, snapshot.Summary)
	assert.Equal(t, "previous summary", snapshot.Summary.Content)
	assert.Len(t, snapshot.Messages, 4) // 2 pairs = 4 messages
	assert.Equal(t, "user", snapshot.Messages[0].Role)
	assert.Equal(t, "hello", snapshot.Messages[0].Content)
	assert.Equal(t, "assistant", snapshot.Messages[1].Role)
	assert.Equal(t, "hi", snapshot.Messages[1].Content)
}

func TestLegacyHistoryProvider_DatabaseFallback(t *testing.T) {
	// 缓存无数据，降级到数据库
	cache := &fakeChatCache{
		pairs:    nil,
		pairsErr: errors.New("redis down"),
	}

	convRepo := &fakeConvRepo{
		conv: &entity.Conversation{
			Summary: "db summary",
		},
	}

	msgRepo := &fakeMsgRepo{
		msgs: []*entity.Message{
			{Role: "user", Content: "msg1"},
			{Role: "assistant", Content: "reply1"},
		},
	}

	provider := NewLegacyHistoryProvider(cache, convRepo, msgRepo)

	snapshot, err := provider.LoadHistory(context.Background(), agentcontext.HistoryQuery{
		ConversationID: 1,
		Limit:          10,
	})

	require.NoError(t, err)
	assert.NotNil(t, snapshot.Summary)
	assert.Equal(t, "db summary", snapshot.Summary.Content)
	assert.Len(t, snapshot.Messages, 2)
}

func TestLegacyHistoryProvider_EmptyHistory(t *testing.T) {
	cache := &fakeChatCache{
		pairs: nil,
	}

	provider := NewLegacyHistoryProvider(cache, &fakeConvRepo{}, &fakeMsgRepo{})

	snapshot, err := provider.LoadHistory(context.Background(), agentcontext.HistoryQuery{
		ConversationID: 1,
		Limit:          10,
	})

	require.NoError(t, err)
	assert.Nil(t, snapshot.Summary)
	assert.Empty(t, snapshot.Messages)
}

func TestLegacyHistoryProvider_ZeroConversationID(t *testing.T) {
	provider := NewLegacyHistoryProvider(&fakeChatCache{}, &fakeConvRepo{}, &fakeMsgRepo{})

	snapshot, err := provider.LoadHistory(context.Background(), agentcontext.HistoryQuery{
		ConversationID: 0,
	})

	require.NoError(t, err)
	assert.Nil(t, snapshot.Summary)
	assert.Empty(t, snapshot.Messages)
}

func TestLegacyHistoryProvider_LimitTruncation(t *testing.T) {
	// 缓存返回超过 limit 的消息
	pairs := make([]MessagePair, 10)
	for i := range pairs {
		pairs[i] = MessagePair{
			User:      "user" + string(rune('A'+i)),
			Assistant: "assistant" + string(rune('A'+i)),
		}
	}

	cache := &fakeChatCache{pairs: pairs}
	provider := NewLegacyHistoryProvider(cache, &fakeConvRepo{}, &fakeMsgRepo{})

	snapshot, err := provider.LoadHistory(context.Background(), agentcontext.HistoryQuery{
		ConversationID: 1,
		Limit:          3,
	})

	require.NoError(t, err)
	// 3 pairs = 6 messages
	assert.Len(t, snapshot.Messages, 6)
}

func TestLegacyHistoryProvider_DatabaseError(t *testing.T) {
	cache := &fakeChatCache{
		pairsErr: errors.New("cache down"),
	}

	convRepo := &fakeConvRepo{
		err: errors.New("db down"),
	}

	msgRepo := &fakeMsgRepo{
		err: errors.New("db down"),
	}

	provider := NewLegacyHistoryProvider(cache, convRepo, msgRepo)

	_, err := provider.LoadHistory(context.Background(), agentcontext.HistoryQuery{
		ConversationID: 1,
		Limit:          10,
	})

	require.Error(t, err)
}

func TestLegacyHistoryProvider_StableOrder(t *testing.T) {
	// 验证消息顺序稳定：user, assistant, user, assistant, ...
	cache := &fakeChatCache{
		pairs: []MessagePair{
			{User: "q1", Assistant: "a1"},
			{User: "q2", Assistant: "a2"},
			{User: "q3", Assistant: "a3"},
		},
	}

	provider := NewLegacyHistoryProvider(cache, &fakeConvRepo{}, &fakeMsgRepo{})

	snapshot, err := provider.LoadHistory(context.Background(), agentcontext.HistoryQuery{
		ConversationID: 1,
		Limit:          10,
	})

	require.NoError(t, err)
	require.Len(t, snapshot.Messages, 6)

	for i := 0; i < 3; i++ {
		assert.Equal(t, "user", snapshot.Messages[i*2].Role)
		assert.Equal(t, "assistant", snapshot.Messages[i*2+1].Role)
	}
}

func TestConvertToMessagePairs(t *testing.T) {
	msgs := []*entity.Message{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}

	pairs := convertToMessagePairs(msgs, 10)
	require.Len(t, pairs, 2)
	assert.Equal(t, "q1", pairs[0].User)
	assert.Equal(t, "a1", pairs[0].Assistant)
	assert.Equal(t, "q2", pairs[1].User)
	assert.Equal(t, "a2", pairs[1].Assistant)
}

func TestConvertToMessagePairs_LimitApplied(t *testing.T) {
	msgs := []*entity.Message{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}

	pairs := convertToMessagePairs(msgs, 1)
	require.Len(t, pairs, 1)
	assert.Equal(t, "q2", pairs[0].User) // 保留最近的
}
