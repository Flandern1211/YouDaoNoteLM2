package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestRegistryModelCapabilitiesResolver_FromRegistry(t *testing.T) {
	registry := newTestRegistry(t)
	resolver := NewRegistryModelCapabilitiesResolver(registry)

	caps, err := resolver.ResolveModel(context.Background(), agentcontext.ModelRef{
		Provider: "openai",
		ModelID:  "gpt-4o",
	})

	require.NoError(t, err)
	assert.Equal(t, 128000, caps.ContextWindow)
	assert.Equal(t, 4096, caps.MaxOutputTokens)
	assert.True(t, caps.SupportsToolCalls)
}

func TestRegistryModelCapabilitiesResolver_FromOverride(t *testing.T) {
	registry := newTestRegistry(t)
	resolver := NewRegistryModelCapabilitiesResolver(registry).
		WithOverride(agentcontext.ModelRef{Provider: "custom", ModelID: "model-v1"}, agentcontext.ModelCapabilities{
			ContextWindow:   32000,
			MaxOutputTokens: 2048,
		})

	caps, err := resolver.ResolveModel(context.Background(), agentcontext.ModelRef{
		Provider: "custom",
		ModelID:  "model-v1",
	})

	require.NoError(t, err)
	assert.Equal(t, 32000, caps.ContextWindow)
}

func TestRegistryModelCapabilitiesResolver_UnknownModel(t *testing.T) {
	registry := newTestRegistry(t)
	resolver := NewRegistryModelCapabilitiesResolver(registry)

	_, err := resolver.ResolveModel(context.Background(), agentcontext.ModelRef{
		Provider: "unknown",
		ModelID:  "model",
	})

	require.Error(t, err)
	assert.True(t, agentcontext.IsErrorCode(err, agentcontext.ErrCodeModelUnknown))
}

func TestRegistryModelCapabilitiesResolver_OverrideTakesPrecedence(t *testing.T) {
	// 如果 Registry 和 override 都有，Registry 优先
	registry := newTestRegistry(t)
	resolver := NewRegistryModelCapabilitiesResolver(registry).
		WithOverride(agentcontext.ModelRef{Provider: "openai", ModelID: "gpt-4o"}, agentcontext.ModelCapabilities{
			ContextWindow: 999,
		})

	caps, err := resolver.ResolveModel(context.Background(), agentcontext.ModelRef{
		Provider: "openai",
		ModelID:  "gpt-4o",
	})

	require.NoError(t, err)
	// Registry 值优先
	assert.Equal(t, 128000, caps.ContextWindow)
}

func TestDefaultModelFixture(t *testing.T) {
	fixture := DefaultModelFixture()

	assert.Contains(t, fixture, "openai/gpt-4o")
	assert.Contains(t, fixture, "openai/gpt-4o-mini")
	assert.Contains(t, fixture, "deepseek/deepseek-chat")

	for _, caps := range fixture {
		assert.Greater(t, caps.ContextWindow, 0)
		assert.Greater(t, caps.MaxOutputTokens, 0)
	}
}

func newTestRegistry(t *testing.T) *agentcontext.Registry {
	t.Helper()

	profiles := []agentcontext.ContextProfile{
		agentcontext.NewChatV1Profile(),
		agentcontext.NewMainV1Profile(),
		agentcontext.NewSearchV1Profile(),
	}

	models := DefaultModelFixture()

	registry, err := agentcontext.NewRegistry(profiles, models)
	require.NoError(t, err)
	return registry
}
