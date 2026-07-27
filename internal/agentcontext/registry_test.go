package agentcontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry_Success(t *testing.T) {
	profiles := []ContextProfile{
		NewChatV1Profile(),
		NewMainV1Profile(),
		NewSearchV1Profile(),
	}

	models := map[string]ModelCapabilities{
		"openai/gpt-4o": {
			ContextWindow:     128000,
			MaxOutputTokens:   4096,
			SupportsToolCalls: true,
		},
	}

	registry, err := NewRegistry(profiles, models)
	require.NoError(t, err)
	assert.NotNil(t, registry)
}

func TestNewRegistry_DuplicateProfile(t *testing.T) {
	profiles := []ContextProfile{
		NewChatV1Profile(),
		NewChatV1Profile(), // 重复
	}

	_, err := NewRegistry(profiles, nil)
	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeDuplicateKey))
}

func TestNewRegistry_InvalidProfile(t *testing.T) {
	profiles := []ContextProfile{
		{
			// 缺少必要字段：version 和 agentID
			Key: ProfileKey{Name: "invalid"},
		},
	}

	_, err := NewRegistry(profiles, nil)
	require.Error(t, err)
	// 错误来自 Profile.Validate()
	assert.Contains(t, err.Error(), "invalid")
}

func TestNewRegistry_InvalidModel(t *testing.T) {
	profiles := []ContextProfile{
		NewChatV1Profile(),
	}

	models := map[string]ModelCapabilities{
		"invalid/model": {
			ContextWindow: 0, // 无效
		},
	}

	_, err := NewRegistry(profiles, models)
	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeInvalidConfig))
}

func TestRegistry_ResolveProfile(t *testing.T) {
	profiles := []ContextProfile{
		NewChatV1Profile(),
		NewSearchV1Profile(),
	}

	registry, err := NewRegistry(profiles, nil)
	require.NoError(t, err)

	// 测试存在的 Profile
	p, ok := registry.ResolveProfile(ChatV1)
	assert.True(t, ok)
	assert.Equal(t, AgentIDChat, p.AgentID)

	// 测试不存在的 Profile
	_, ok = registry.ResolveProfile(ProfileKey{Name: "unknown", Version: "v1"})
	assert.False(t, ok)
}

func TestRegistry_ResolveModel(t *testing.T) {
	models := map[string]ModelCapabilities{
		"openai/gpt-4o": {
			ContextWindow:   128000,
			MaxOutputTokens: 4096,
		},
	}

	registry, err := NewRegistry(nil, models)
	require.NoError(t, err)

	// 测试存在的模型
	cap, ok := registry.ResolveModel(ModelRef{Provider: "openai", ModelID: "gpt-4o"})
	assert.True(t, ok)
	assert.Equal(t, 128000, cap.ContextWindow)

	// 测试不存在的模型
	_, ok = registry.ResolveModel(ModelRef{Provider: "unknown", ModelID: "model"})
	assert.False(t, ok)
}

func TestRegistry_Profiles(t *testing.T) {
	profiles := []ContextProfile{
		NewChatV1Profile(),
		NewMainV1Profile(),
		NewSearchV1Profile(),
	}

	registry, err := NewRegistry(profiles, nil)
	require.NoError(t, err)

	all := registry.Profiles()
	assert.Len(t, all, 3)
}

func TestRegistry_Immutability(t *testing.T) {
	// 测试注册表构造后不可变
	// 这是一个行为测试，验证没有暴露修改方法

	profiles := []ContextProfile{
		NewChatV1Profile(),
	}

	registry, err := NewRegistry(profiles, nil)
	require.NoError(t, err)

	// 获取 Profile 后修改不会影响注册表
	p, _ := registry.ResolveProfile(ChatV1)
	p.LoadMemory = false

	// 再次获取，应该是原始值
	p2, _ := registry.ResolveProfile(ChatV1)
	assert.True(t, p2.LoadMemory, "注册表应该是不可变的")
}
