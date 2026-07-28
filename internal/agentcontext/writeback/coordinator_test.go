package writeback

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

// ============ Mock 实现 ============

type mockVerifier struct {
	verifyAcceptedErr  error
	verifyAuthorityErr error
	verifiedTurn       VerifiedTurn
}

func (m *mockVerifier) VerifyAccepted(_ context.Context, handle agentcontext.AcceptedTurnHandle) (VerifiedTurn, error) {
	if m.verifyAcceptedErr != nil {
		return VerifiedTurn{}, m.verifyAcceptedErr
	}
	return m.verifiedTurn, nil
}

func (m *mockVerifier) VerifyAuthority(_ context.Context, _ string, _ agentcontext.ActiveExecutionAuthority) error {
	return m.verifyAuthorityErr
}

type mockAssistantWriter struct {
	committed CommittedMessage
	err       error
	calls     int
}

func (m *mockAssistantWriter) CommitAssistant(_ context.Context, req AssistantWriteRequest) (CommittedMessage, error) {
	m.calls++
	return m.committed, m.err
}

type mockSummaryWriter struct {
	result SummaryWriteResult
	err    error
	calls  int
}

func (m *mockSummaryWriter) EvaluateAndUpdate(_ context.Context, req SummaryWriteRequest) (SummaryWriteResult, error) {
	m.calls++
	return m.result, m.err
}

type mockMemoryWriter struct {
	result MemoryWriteResult
	err    error
	calls  int
}

func (m *mockMemoryWriter) EvaluateAndStore(_ context.Context, req MemoryWriteRequest) (MemoryWriteResult, error) {
	m.calls++
	return m.result, m.err
}

type mockManifestWriter struct {
	err   error
	calls int
}

func (m *mockManifestWriter) StoreManifest(_ context.Context, req ManifestWriteRequest) error {
	m.calls++
	return m.err
}

type mockStepResultWriter struct {
	committed CommittedStepResult
	err       error
	calls     int
}

func (m *mockStepResultWriter) CommitStepResult(_ context.Context, req StepResultWriteRequest) (CommittedStepResult, error) {
	m.calls++
	return m.committed, m.err
}

// ============ BeginTurn 测试 ============

func TestTurnLifecycleCoordinator_BeginTurn_Success(t *testing.T) {
	verifier := &mockVerifier{
		verifiedTurn: VerifiedTurn{
			Handle: agentcontext.AcceptedTurnHandle{
				RunID:   "run-1",
				AgentID: agentcontext.AgentIDChat,
			},
			Profile: agentcontext.ContextProfileSnapshot{
				Key: agentcontext.ChatV1,
			},
		},
	}

	coordinator := NewTurnLifecycleCoordinator(CoordinatorConfig{
		Verifier: verifier,
	})

	session, err := coordinator.BeginTurn(context.Background(), agentcontext.BeginTurnRequest{
		Handle: agentcontext.AcceptedTurnHandle{
			RunID: "run-1",
		},
		Authority: agentcontext.ActiveExecutionAuthority{
			AttemptID: "attempt-1",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "run-1", session.Handle.RunID)
	assert.Equal(t, agentcontext.ChatV1, session.Profile.Key)
}

func TestTurnLifecycleCoordinator_BeginTurn_HandleVerificationFailed(t *testing.T) {
	verifier := &mockVerifier{
		verifyAcceptedErr: assert.AnError,
	}

	coordinator := NewTurnLifecycleCoordinator(CoordinatorConfig{
		Verifier: verifier,
	})

	_, err := coordinator.BeginTurn(context.Background(), agentcontext.BeginTurnRequest{
		Handle: agentcontext.AcceptedTurnHandle{RunID: "run-1"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Handle 验证失败")
}

func TestTurnLifecycleCoordinator_BeginTurn_AuthorityVerificationFailed(t *testing.T) {
	verifier := &mockVerifier{
		verifyAuthorityErr: assert.AnError,
	}

	coordinator := NewTurnLifecycleCoordinator(CoordinatorConfig{
		Verifier: verifier,
	})

	_, err := coordinator.BeginTurn(context.Background(), agentcontext.BeginTurnRequest{
		Handle:    agentcontext.AcceptedTurnHandle{RunID: "run-1"},
		Authority: agentcontext.ActiveExecutionAuthority{AttemptID: "attempt-1"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Authority 验证失败")
}

// ============ FinalizeTurn 测试 ============

func TestTurnLifecycleCoordinator_FinalizeTurn_Success_Chat(t *testing.T) {
	assistantWriter := &mockAssistantWriter{}
	summaryWriter := &mockSummaryWriter{result: SummaryWriteResult{Status: WritebackStatusSuccess}}
	memoryWriter := &mockMemoryWriter{result: MemoryWriteResult{Status: WritebackStatusSuccess}}
	manifestWriter := &mockManifestWriter{}

	coordinator := NewTurnLifecycleCoordinator(CoordinatorConfig{
		Verifier: &mockVerifier{},
		Writers: WriterRegistry{
			Assistant: assistantWriter,
			Summary:   summaryWriter,
			Memory:    memoryWriter,
			Manifest:  manifestWriter,
		},
	})

	result, err := coordinator.FinalizeTurn(context.Background(), agentcontext.FinalizeRequest{
		Turn: &agentcontext.PreparedTurn{
			Session: &agentcontext.TurnSession{
				Handle: agentcontext.AcceptedTurnHandle{RunID: "run-1"},
			},
			Profile: agentcontext.ContextProfileSnapshot{
				Key:             agentcontext.ChatV1,
				WritebackPolicy: agentcontext.WritebackPolicyConversationTurn,
			},
		},
		Outcome: agentcontext.TurnOutcome{
			Status: agentcontext.TurnStatusSuccess,
			PrimaryOutput: agentcontext.ConversationOutput{
				FinalMessage: &schema.Message{Content: "hello"},
			},
		},
		FinalizeKey: agentcontext.FinalizeKey{RunID: "run-1", Revision: 1},
		Authority:   agentcontext.ActiveExecutionAuthority{AttemptID: "attempt-1"},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, assistantWriter.calls)
	assert.Equal(t, 1, summaryWriter.calls)
	assert.Equal(t, 1, memoryWriter.calls)
	assert.Equal(t, 1, manifestWriter.calls)
	assert.NotNil(t, result)
}

func TestTurnLifecycleCoordinator_FinalizeTurn_Failed_Chat(t *testing.T) {
	manifestWriter := &mockManifestWriter{}

	coordinator := NewTurnLifecycleCoordinator(CoordinatorConfig{
		Verifier: &mockVerifier{},
		Writers: WriterRegistry{
			Manifest: manifestWriter,
		},
	})

	result, err := coordinator.FinalizeTurn(context.Background(), agentcontext.FinalizeRequest{
		Turn: &agentcontext.PreparedTurn{
			Session: &agentcontext.TurnSession{
				Handle: agentcontext.AcceptedTurnHandle{RunID: "run-1"},
			},
			Profile: agentcontext.ContextProfileSnapshot{
				Key:             agentcontext.ChatV1,
				WritebackPolicy: agentcontext.WritebackPolicyConversationTurn,
			},
		},
		Outcome: agentcontext.TurnOutcome{
			Status: agentcontext.TurnStatusFailed,
		},
		FinalizeKey: agentcontext.FinalizeKey{RunID: "run-1", Revision: 1},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, manifestWriter.calls)
	assert.Equal(t, agentcontext.WritebackStatusSuccess, result.Manifest)
}

func TestTurnLifecycleCoordinator_FinalizeTurn_AuthorityRejected(t *testing.T) {
	coordinator := NewTurnLifecycleCoordinator(CoordinatorConfig{
		Verifier: &mockVerifier{
			verifyAuthorityErr: assert.AnError,
		},
	})

	_, err := coordinator.FinalizeTurn(context.Background(), agentcontext.FinalizeRequest{
		Turn: &agentcontext.PreparedTurn{
			Session: &agentcontext.TurnSession{
				Handle: agentcontext.AcceptedTurnHandle{RunID: "run-1"},
			},
		},
		Outcome: agentcontext.TurnOutcome{
			Status: agentcontext.TurnStatusSuccess,
		},
		Authority: agentcontext.ActiveExecutionAuthority{AttemptID: "attempt-1"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Authority 验证失败")
}

func TestTurnLifecycleCoordinator_FinalizeTurn_NilWriters(t *testing.T) {
	coordinator := NewTurnLifecycleCoordinator(CoordinatorConfig{
		Verifier: &mockVerifier{},
		Writers:  WriterRegistry{}, // 所有 Writer 为 nil
	})

	result, err := coordinator.FinalizeTurn(context.Background(), agentcontext.FinalizeRequest{
		Turn: &agentcontext.PreparedTurn{
			Session: &agentcontext.TurnSession{
				Handle: agentcontext.AcceptedTurnHandle{RunID: "run-1"},
			},
			Profile: agentcontext.ContextProfileSnapshot{
				Key:             agentcontext.ChatV1,
				WritebackPolicy: agentcontext.WritebackPolicyConversationTurn,
			},
		},
		Outcome: agentcontext.TurnOutcome{
			Status: agentcontext.TurnStatusSuccess,
		},
		FinalizeKey: agentcontext.FinalizeKey{RunID: "run-1", Revision: 1},
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestTurnLifecycleCoordinator_FinalizeTurn_Search_Success(t *testing.T) {
	stepWriter := &mockStepResultWriter{}
	manifestWriter := &mockManifestWriter{}

	coordinator := NewTurnLifecycleCoordinator(CoordinatorConfig{
		Verifier: &mockVerifier{},
		Writers: WriterRegistry{
			StepResult: stepWriter,
			Manifest:   manifestWriter,
		},
	})

	result, err := coordinator.FinalizeTurn(context.Background(), agentcontext.FinalizeRequest{
		Turn: &agentcontext.PreparedTurn{
			Session: &agentcontext.TurnSession{
				Handle: agentcontext.AcceptedTurnHandle{RunID: "run-1"},
			},
			Profile: agentcontext.ContextProfileSnapshot{
				Key:             agentcontext.SearchV1,
				WritebackPolicy: agentcontext.WritebackPolicyStepResult,
			},
		},
		Outcome: agentcontext.TurnOutcome{
			Status: agentcontext.TurnStatusSuccess,
		},
		FinalizeKey: agentcontext.FinalizeKey{RunID: "run-1", Revision: 1},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, stepWriter.calls)
	assert.Equal(t, 1, manifestWriter.calls)
	assert.NotNil(t, result)
}
