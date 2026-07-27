package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agent/chat/prompts"
	"YoudaoNoteLm/internal/agentcontext"
)

func TestStaticPromptProvider_WithSources(t *testing.T) {
	lookup := &StaticSourceLookup{
		Sources: []prompts.SourceInfo{
			{ID: 1, Name: "资料A"},
			{ID: 2, Name: "资料B"},
		},
	}

	provider := NewStaticPromptProvider(lookup)

	prompt, err := provider.LoadPrompt(context.Background(), agentcontext.PromptQuery{
		AgentID:    agentcontext.AgentIDChat,
		ProfileKey: agentcontext.ChatV1,
	})

	require.NoError(t, err)
	assert.Equal(t, "chat.v1", prompt.ID)
	assert.Equal(t, "v1", prompt.Version)
	assert.Contains(t, prompt.Content, "资料A")
	assert.Contains(t, prompt.Content, "资料B")
	assert.Contains(t, prompt.Content, "智能知识问答助手")
}

func TestStaticPromptProvider_NoSources(t *testing.T) {
	lookup := &StaticSourceLookup{
		Sources: []prompts.SourceInfo{},
	}

	provider := NewStaticPromptProvider(lookup)

	prompt, err := provider.LoadPrompt(context.Background(), agentcontext.PromptQuery{
		AgentID:    agentcontext.AgentIDChat,
		ProfileKey: agentcontext.ChatV1,
	})

	require.NoError(t, err)
	assert.Contains(t, prompt.Content, "用户未选定特定资料")
}

func TestStaticPromptProvider_NilLookup(t *testing.T) {
	provider := NewStaticPromptProvider(nil)

	prompt, err := provider.LoadPrompt(context.Background(), agentcontext.PromptQuery{
		AgentID:    agentcontext.AgentIDChat,
		ProfileKey: agentcontext.ChatV1,
	})

	require.NoError(t, err)
	assert.Contains(t, prompt.Content, "用户未选定特定资料")
}

func TestStaticPromptProvider_LookupError(t *testing.T) {
	lookup := &failingLookup{err: errors.New("lookup failed")}
	provider := NewStaticPromptProvider(lookup)

	_, err := provider.LoadPrompt(context.Background(), agentcontext.PromptQuery{
		AgentID:    agentcontext.AgentIDChat,
		ProfileKey: agentcontext.ChatV1,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "lookup failed")
}

func TestStaticPromptProvider_ConsistentWithBuilder(t *testing.T) {
	// 验证 PromptProvider 和 ChatAgentBuilder 生成的 Prompt 一致
	sources := []prompts.SourceInfo{
		{ID: 1, Name: "资料A"},
		{ID: 2, Name: "资料B"},
	}

	// Provider 渲染
	lookup := &StaticSourceLookup{Sources: sources}
	provider := NewStaticPromptProvider(lookup)
	prompt, err := provider.LoadPrompt(context.Background(), agentcontext.PromptQuery{
		AgentID:    agentcontext.AgentIDChat,
		ProfileKey: agentcontext.ChatV1,
	})
	require.NoError(t, err)

	// 直接渲染
	expected := prompts.RenderSystemPrompt(sources)

	assert.Equal(t, expected, prompt.Content)
}

// failingLookup 总是返回错误的查找实现
type failingLookup struct {
	err error
}

func (f *failingLookup) GetSources(_ context.Context, _ agentcontext.AgentID) ([]prompts.SourceInfo, error) {
	return nil, f.err
}
