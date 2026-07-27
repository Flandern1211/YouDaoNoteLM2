package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/model/entity"
	"YoudaoNoteLm/pkg/cache"
)

// fakeChatCache 实现 chatCacheReader 接口，用于测试
type fakeChatCache struct {
	messages      []cache.MessagePair
	messagesErr   error
	summary       string
	summaryOK     bool
	summaryErr    error
	setSummaryErr error
}

func (f *fakeChatCache) GetRecentMessages(_ context.Context, _ uint) ([]cache.MessagePair, error) {
	return f.messages, f.messagesErr
}

func (f *fakeChatCache) GetSummary(_ context.Context, _ uint) (string, bool, error) {
	return f.summary, f.summaryOK, f.summaryErr
}

func (f *fakeChatCache) SetSummary(_ context.Context, _ uint, _ string) error {
	return f.setSummaryErr
}

// fakeConversationRepository 实现最小 ConversationRepository 接口
type fakeConversationRepository struct {
	conv *entity.Conversation
	err  error
}

func (f *fakeConversationRepository) FindByID(_ uint) (*entity.Conversation, error) {
	return f.conv, f.err
}

// 以下是接口的其他方法，测试中不会调用，但需要实现以满足接口
func (f *fakeConversationRepository) Create(_ *entity.Conversation) error { return nil }
func (f *fakeConversationRepository) FindByIDAndUserID(_, _ uint) (*entity.Conversation, error) {
	return nil, nil
}
func (f *fakeConversationRepository) FindByNotebookID(_ uint) ([]*entity.Conversation, error) {
	return nil, nil
}
func (f *fakeConversationRepository) FindByNotebookIDAndUserID(_, _ uint) ([]*entity.Conversation, error) {
	return nil, nil
}
func (f *fakeConversationRepository) Update(_ *entity.Conversation) error  { return nil }
func (f *fakeConversationRepository) UpdateTitle(_ uint, _ string) error   { return nil }
func (f *fakeConversationRepository) UpdateSummary(_ uint, _ string) error { return nil }
func (f *fakeConversationRepository) Delete(_ uint) error                  { return nil }
func (f *fakeConversationRepository) DeleteByNotebookID(_ uint) error      { return nil }
func (f *fakeConversationRepository) DeleteWithMessages(_ uint) error      { return nil }

// fakeMessageRepository 实现最小 MessageRepository 接口
type fakeMessageRepository struct {
	messages []*entity.Message
	err      error
}

func (f *fakeMessageRepository) FindRecentByConversationID(_ uint, _ int) ([]*entity.Message, error) {
	return f.messages, f.err
}

// 以下是接口的其他方法
func (f *fakeMessageRepository) Create(_ *entity.Message) error        { return nil }
func (f *fakeMessageRepository) CreateBatch(_ []*entity.Message) error { return nil }
func (f *fakeMessageRepository) FindByConversationID(_ uint) ([]*entity.Message, error) {
	return nil, nil
}
func (f *fakeMessageRepository) FindOlderThan(_ uint, _ int) ([]*entity.Message, error) {
	return nil, nil
}
func (f *fakeMessageRepository) CountByConversationID(_ uint) (int64, error) { return 0, nil }
func (f *fakeMessageRepository) DeleteByConversationID(_ uint) error         { return nil }

func TestContextBuilder_BuildMessages_WithCacheHit(t *testing.T) {
	// 测试场景：Redis 缓存命中时不读取 Repository
	cache := &fakeChatCache{
		messages: []cache.MessagePair{
			{User: "你好", Assistant: "你好！有什么可以帮助你的吗？"},
			{User: "今天天气怎么样？", Assistant: "今天天气不错！"},
		},
		summary:   "之前的对话摘要",
		summaryOK: true,
	}

	builder := &ContextBuilder{
		conversationRepo: &fakeConversationRepository{},
		messageRepo:      &fakeMessageRepository{},
		cache:            cache,
	}

	messages, err := builder.BuildMessages(context.Background(), 1, "新消息")
	require.NoError(t, err)

	// 验证消息顺序：摘要 -> 历史消息 -> 当前输入
	assert.Len(t, messages, 6) // 1摘要 + 2轮历史(4条) + 1当前输入
	assert.Equal(t, schema.System, messages[0].Role)
	assert.Contains(t, messages[0].Content, "之前的对话摘要")
	assert.Equal(t, schema.User, messages[1].Role)
	assert.Equal(t, "你好", messages[1].Content)
	assert.Equal(t, schema.Assistant, messages[2].Role)
	assert.Equal(t, "你好！有什么可以帮助你的吗？", messages[2].Content)
	assert.Equal(t, schema.User, messages[5].Role)
	assert.Equal(t, "新消息", messages[5].Content)
}

func TestContextBuilder_BuildMessages_SummaryFallbackToDB(t *testing.T) {
	// 测试场景：Redis 摘要缺失时读取 ConversationRepository，并回填缓存
	cache := &fakeChatCache{
		messages:  []cache.MessagePair{},
		summary:   "",
		summaryOK: false,
	}
	convRepo := &fakeConversationRepository{
		conv: &entity.Conversation{
			Summary: "数据库中的摘要",
		},
	}

	builder := &ContextBuilder{
		conversationRepo: convRepo,
		messageRepo:      &fakeMessageRepository{},
		cache:            cache,
	}

	messages, err := builder.BuildMessages(context.Background(), 1, "新消息")
	require.NoError(t, err)

	// 验证摘要从数据库加载
	assert.Len(t, messages, 2) // 1摘要 + 1当前输入
	assert.Equal(t, schema.System, messages[0].Role)
	assert.Contains(t, messages[0].Content, "数据库中的摘要")
}

func TestContextBuilder_BuildMessages_HistoryFallbackToDB(t *testing.T) {
	// 测试场景：Redis 历史缺失时读取 MessageRepository
	cache := &fakeChatCache{
		messages:    nil, // Redis 无数据
		messagesErr: errors.New("redis connection error"),
	}
	msgRepo := &fakeMessageRepository{
		messages: []*entity.Message{
			{Role: "user", Content: "用户消息1"},
			{Role: "assistant", Content: "助手消息1"},
			{Role: "user", Content: "用户消息2"},
			{Role: "assistant", Content: "助手消息2"},
		},
	}

	builder := &ContextBuilder{
		conversationRepo: &fakeConversationRepository{},
		messageRepo:      msgRepo,
		cache:            cache,
	}

	messages, err := builder.BuildMessages(context.Background(), 1, "新消息")
	require.NoError(t, err)

	// 验证消息从数据库加载
	assert.Len(t, messages, 5) // 2轮历史(4条) + 1当前输入
	assert.Equal(t, schema.User, messages[0].Role)
	assert.Equal(t, "用户消息1", messages[0].Content)
	assert.Equal(t, schema.Assistant, messages[1].Role)
	assert.Equal(t, "助手消息1", messages[1].Content)
}

func TestContextBuilder_BuildMessages_EmptyHistory(t *testing.T) {
	// 测试场景：无历史消息（新对话）
	cache := &fakeChatCache{
		messages: []cache.MessagePair{},
	}

	builder := &ContextBuilder{
		conversationRepo: &fakeConversationRepository{},
		messageRepo:      &fakeMessageRepository{},
		cache:            cache,
	}

	messages, err := builder.BuildMessages(context.Background(), 0, "第一条消息")
	require.NoError(t, err)

	// 验证只有当前输入
	assert.Len(t, messages, 1)
	assert.Equal(t, schema.User, messages[0].Role)
	assert.Equal(t, "第一条消息", messages[0].Content)
}

func TestContextBuilder_BuildMessages_CurrentInputOnlyOnce(t *testing.T) {
	// 测试场景：当前输入只出现一次
	cache := &fakeChatCache{
		messages: []cache.MessagePair{
			{User: "历史消息", Assistant: "历史回复"},
		},
	}

	builder := &ContextBuilder{
		conversationRepo: &fakeConversationRepository{},
		messageRepo:      &fakeMessageRepository{},
		cache:            cache,
	}

	messages, err := builder.BuildMessages(context.Background(), 1, "当前输入")
	require.NoError(t, err)

	// 统计当前输入出现的次数
	count := 0
	for _, msg := range messages {
		if msg.Content == "当前输入" && msg.Role == schema.User {
			count++
		}
	}
	assert.Equal(t, 1, count, "当前输入应该只出现一次")
}

func TestContextBuilder_BuildMessages_RepositoryError(t *testing.T) {
	// 测试场景：Repository 错误沿现有路径返回
	cache := &fakeChatCache{
		messages:    nil,
		messagesErr: errors.New("redis error"),
	}
	msgRepo := &fakeMessageRepository{
		err: errors.New("database error"),
	}

	builder := &ContextBuilder{
		conversationRepo: &fakeConversationRepository{},
		messageRepo:      msgRepo,
		cache:            cache,
	}

	_, err := builder.BuildMessages(context.Background(), 1, "消息")
	// 当 Redis 和 MySQL 都失败时，应该返回空历史（不报错），只有当前输入
	require.NoError(t, err)
}
