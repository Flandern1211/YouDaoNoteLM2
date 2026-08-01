package harness

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestService_RunLifecycleAndAuthority(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store)
	handle := agentcontext.AcceptedTurnHandle{
		AgentID:        agentcontext.AgentIDChat,
		UserID:         7,
		ConversationID: 11,
		Input:          agentcontext.UserMessageInput{Content: "hello"},
		ContextMode: agentcontext.ContextModeSnapshot{
			Mode:            "enabled",
			WritebackOwner:  "context",
			ContractVersion: ContextContractVersion,
		},
	}
	chatProfile := agentcontext.NewChatV1Profile()
	profile := chatProfile.ToSnapshot()

	accepted, authority, err := service.AcceptTurn(context.Background(), handle, profile)
	require.NoError(t, err)
	assert.NotEmpty(t, accepted.RunID)
	assert.NotEmpty(t, authority.AttemptID)

	verified, err := service.VerifyAccepted(context.Background(), accepted)
	require.NoError(t, err)
	assert.Equal(t, profile, verified.Profile)
	require.NoError(t, service.VerifyAuthority(context.Background(), accepted.RunID, authority))

	key, finalizingAuthority, err := service.BeginFinalization(
		context.Background(),
		accepted.RunID,
		authority,
		agentcontext.TurnStatusSuccess,
	)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), key.Revision)
	assert.Equal(t, authority.RunStateVersion+1, finalizingAuthority.RunStateVersion)
	assert.ErrorIs(t, service.VerifyAuthority(context.Background(), accepted.RunID, authority), ErrAuthorityStale)
	require.NoError(t, service.VerifyAuthority(context.Background(), accepted.RunID, finalizingAuthority))

	require.NoError(t, service.Complete(
		context.Background(),
		accepted.RunID,
		finalizingAuthority,
		agentcontext.TurnStatusSuccess,
	))
	record, err := store.GetRun(context.Background(), accepted.RunID)
	require.NoError(t, err)
	assert.Equal(t, RunStateCompleted, record.State)

	retryKey, retryAuthority, err := service.BeginFinalization(
		context.Background(),
		accepted.RunID,
		authority,
		agentcontext.TurnStatusSuccess,
	)
	require.NoError(t, err)
	assert.Equal(t, key, retryKey)
	require.NoError(t, service.VerifyAuthority(context.Background(), accepted.RunID, retryAuthority))
	require.NoError(t, service.Complete(
		context.Background(),
		accepted.RunID,
		retryAuthority,
		agentcontext.TurnStatusSuccess,
	))
}

func TestService_RejectsTamperedHandleAndStaleAuthority(t *testing.T) {
	service := NewService(NewMemoryStore())
	accepted, authority, err := service.AcceptTurn(
		context.Background(),
		agentcontext.AcceptedTurnHandle{
			AgentID: agentcontext.AgentIDChat,
			UserID:  7,
			Input:   agentcontext.UserMessageInput{Content: "hello"},
		},
		func() agentcontext.ContextProfileSnapshot {
			profile := agentcontext.NewChatV1Profile()
			return profile.ToSnapshot()
		}(),
	)
	require.NoError(t, err)

	tampered := accepted
	tampered.Input = agentcontext.UserMessageInput{Content: "changed"}
	_, err = service.VerifyAccepted(context.Background(), tampered)
	require.Error(t, err)
	assert.True(t, agentcontext.IsErrorCode(err, agentcontext.ErrCodeInvalidTurnHandle))

	stale := authority
	stale.FencingToken++
	assert.ErrorIs(t, service.VerifyAuthority(context.Background(), accepted.RunID, stale), ErrAuthorityStale)
}

func TestMemoryStore_FinalizationIsIdempotent(t *testing.T) {
	service := NewService(NewMemoryStore())
	accepted, authority, err := service.AcceptTurn(
		context.Background(),
		agentcontext.AcceptedTurnHandle{
			AgentID: agentcontext.AgentIDSearch,
			UserID:  9,
			Input:   agentcontext.SearchTaskInput{Task: agentcontext.SearchTask{Query: "q"}},
		},
		func() agentcontext.ContextProfileSnapshot {
			profile := agentcontext.NewSearchV1Profile()
			return profile.ToSnapshot()
		}(),
	)
	require.NoError(t, err)

	key1, finalizing1, err := service.BeginFinalization(
		context.Background(),
		accepted.RunID,
		authority,
		agentcontext.TurnStatusFailed,
	)
	require.NoError(t, err)
	key2, finalizing2, err := service.BeginFinalization(
		context.Background(),
		accepted.RunID,
		authority,
		agentcontext.TurnStatusFailed,
	)
	require.NoError(t, err)
	assert.Equal(t, key1, key2)
	assert.Equal(t, finalizing1, finalizing2)
}
