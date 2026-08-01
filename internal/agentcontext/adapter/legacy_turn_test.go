package adapter

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestLegacyTurnAdapter_PrepareChatSession(t *testing.T) {
	registry, err := agentcontext.NewRegistry(
		[]agentcontext.ContextProfile{
			agentcontext.NewChatV1Profile(),
		},
		nil,
	)
	require.NoError(t, err)

	adapter := NewLegacyTurnAdapter(registry)

	session, err := adapter.PrepareChatSession(1, 42, "hello")
	require.NoError(t, err)

	assert.Equal(t, agentcontext.AgentIDChat, session.Handle.AgentID)
	assert.Equal(t, uint(42), session.Handle.UserID)
	assert.Equal(t, uint(1), session.Handle.ConversationID)
	assert.Equal(t, "legacy-1", session.Handle.RunID)
	assert.Equal(t, "step-0", session.Handle.StepID)
	assert.Equal(t, "legacy", session.Handle.ContextMode.Mode)

	// 验证 Input
	input, ok := session.Handle.Input.(agentcontext.UserMessageInput)
	require.True(t, ok)
	assert.Equal(t, "hello", input.Content)

	// 验证 Profile
	assert.Equal(t, agentcontext.ChatV1, session.Profile.Key)
}

func TestLegacyTurnAdapter_PrepareSearchSession(t *testing.T) {
	registry, err := agentcontext.NewRegistry(
		[]agentcontext.ContextProfile{
			agentcontext.NewSearchV1Profile(),
		},
		nil,
	)
	require.NoError(t, err)

	adapter := NewLegacyTurnAdapter(registry)

	session, err := adapter.PrepareSearchSession(42, "Go context")
	require.NoError(t, err)

	assert.Equal(t, agentcontext.AgentIDSearch, session.Handle.AgentID)
	assert.Equal(t, uint(42), session.Handle.UserID)
	assert.Equal(t, "legacy-search-42", session.Handle.RunID)

	// 验证 Input
	input, ok := session.Handle.Input.(agentcontext.SearchTaskInput)
	require.True(t, ok)
	assert.Equal(t, "Go context", input.Task.Query)

	// 验证 Profile
	assert.Equal(t, agentcontext.SearchV1, session.Profile.Key)
}

func TestLegacyTurnAdapter_ChatProfileNotFound(t *testing.T) {
	registry, err := agentcontext.NewRegistry(nil, nil)
	require.NoError(t, err)

	adapter := NewLegacyTurnAdapter(registry)
	_, err = adapter.PrepareChatSession(1, 42, "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat.v1")
}

func TestLegacyTurnAdapter_SearchProfileNotFound(t *testing.T) {
	registry, err := agentcontext.NewRegistry(nil, nil)
	require.NoError(t, err)

	adapter := NewLegacyTurnAdapter(registry)
	_, err = adapter.PrepareSearchSession(42, "query")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search.v1")
}

func TestLegacyHistoryAdapter_BuildLegacyMessages(t *testing.T) {
	adapter := &LegacyHistoryAdapter{
		BuildMessages: func(ctx context.Context, conversationID uint, content string) ([]*schema.Message, error) {
			return []*schema.Message{
				schema.UserMessage(content),
			}, nil
		},
	}

	messages, err := adapter.BuildLegacyMessages(context.Background(), 1, "hello")
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, "hello", messages[0].Content)
}

func TestLegacyHistoryAdapter_NilBuildMessages(t *testing.T) {
	adapter := &LegacyHistoryAdapter{}
	_, err := adapter.BuildLegacyMessages(context.Background(), 1, "hello")
	require.Error(t, err)
}
