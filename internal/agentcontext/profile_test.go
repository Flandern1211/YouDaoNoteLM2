package agentcontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextProfile_Validate(t *testing.T) {
	tests := []struct {
		name    string
		profile ContextProfile
		wantErr bool
		errCode ErrorCode
	}{
		{
			name: "valid profile",
			profile: ContextProfile{
				Key:        ProfileKey{Name: "chat", Version: "v1"},
				AgentID:    AgentIDChat,
				LoadMemory: true,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			profile: ContextProfile{
				Key:     ProfileKey{Version: "v1"},
				AgentID: AgentIDChat,
			},
			wantErr: true,
			errCode: ErrCodeInvalidConfig,
		},
		{
			name: "missing version",
			profile: ContextProfile{
				Key:     ProfileKey{Name: "chat"},
				AgentID: AgentIDChat,
			},
			wantErr: true,
			errCode: ErrCodeInvalidConfig,
		},
		{
			name: "missing agent ID",
			profile: ContextProfile{
				Key: ProfileKey{Name: "chat", Version: "v1"},
			},
			wantErr: true,
			errCode: ErrCodeInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, IsErrorCode(err, tt.errCode))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestChatV1Profile(t *testing.T) {
	p := NewChatV1Profile()

	assert.Equal(t, ChatV1, p.Key)
	assert.Equal(t, AgentIDChat, p.AgentID)
	assert.True(t, p.LoadMemory)
	assert.True(t, p.LoadHistory)
	assert.True(t, p.LoadSummary)
	assert.Equal(t, WritebackPolicyConversationTurn, p.WritebackPolicy)
	assert.Contains(t, p.AllowedSources, ContextKindConversationSummary)
	assert.Contains(t, p.AllowedSources, ContextKindUserMemory)
}

func TestMainV1Profile(t *testing.T) {
	p := NewMainV1Profile()

	assert.Equal(t, MainV1, p.Key)
	assert.Equal(t, AgentIDMain, p.AgentID)
	assert.True(t, p.LoadMemory)
	assert.True(t, p.LoadHistory)
	assert.True(t, p.LoadSummary)
	assert.Equal(t, WritebackPolicyConversationTurn, p.WritebackPolicy)
}

func TestSearchV1Profile(t *testing.T) {
	p := NewSearchV1Profile()

	assert.Equal(t, SearchV1, p.Key)
	assert.Equal(t, AgentIDSearch, p.AgentID)
	assert.False(t, p.LoadMemory)
	assert.False(t, p.LoadHistory)
	assert.False(t, p.LoadSummary)
	assert.Equal(t, WritebackPolicyStepResult, p.WritebackPolicy)
	assert.Empty(t, p.AllowedSources)
}
