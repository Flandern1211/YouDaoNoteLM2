package harness

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestPersistentRun_StoresInputHashWithoutPlaintext(t *testing.T) {
	secret := "user-secret-content"
	profile := agentcontext.NewChatV1Profile()
	record := RunRecord{
		Handle: agentcontext.AcceptedTurnHandle{
			RunID:          "run-1",
			StepID:         "step-0",
			AgentID:        agentcontext.AgentIDChat,
			UserID:         7,
			ConversationID: 11,
			Input:          agentcontext.UserMessageInput{Content: secret},
			ContextMode: agentcontext.ContextModeSnapshot{
				Mode:            "enabled",
				WritebackOwner:  "context",
				ContractVersion: ContextContractVersion,
			},
		},
		Profile:   profile.ToSnapshot(),
		InputHash: hashTurnInput(agentcontext.UserMessageInput{Content: secret}),
		State:     RunStateRunning,
		Authority: agentcontext.ActiveExecutionAuthority{
			AttemptID:       "attempt-1",
			FencingToken:    1,
			RunStateVersion: 1,
		},
	}

	persistent, err := encodeRun(record)
	require.NoError(t, err)
	encoded, err := json.Marshal(persistent)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), secret)
	assert.Len(t, persistent.InputHash, 64)

	decoded, err := decodeRun(persistent)
	require.NoError(t, err)
	assert.Equal(t, record.Handle.RunID, decoded.Handle.RunID)
	assert.Equal(t, record.Handle.ContextMode, decoded.Handle.ContextMode)
	assert.Equal(t, record.Profile, decoded.Profile)
}

func TestPersistentManifest_HasNoPlaintextFields(t *testing.T) {
	manifestType := reflect.TypeOf(PersistentManifest{})
	for _, forbidden := range []string{"Content", "Prompt", "Messages", "ToolArguments", "Input"} {
		_, exists := manifestType.FieldByName(forbidden)
		assert.False(t, exists, "Manifest 持久化模型不得包含 %s 字段", forbidden)
	}
}
