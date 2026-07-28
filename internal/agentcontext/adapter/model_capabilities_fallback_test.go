package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestConservativeModelCapabilitiesResolver_CustomModel(t *testing.T) {
	registry, err := agentcontext.NewRegistry(nil, nil)
	require.NoError(t, err)
	resolver := NewConservativeModelCapabilitiesResolver(registry)

	caps, err := resolver.ResolveModel(context.Background(), agentcontext.ModelRef{
		Provider: "custom",
		ModelID:  "private-model",
	})

	require.NoError(t, err)
	assert.Equal(t, 128000, caps.ContextWindow)
	assert.Equal(t, agentcontext.TokenizerStrategyConservativeUTF8, caps.TokenizerStrategy)
}
