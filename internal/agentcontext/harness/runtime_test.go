package harness

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/internal/agentcontext/writeback"
	"YoudaoNoteLm/pkg/config"
)

type runtimeCompiler struct {
	prepared *agentcontext.PreparedTurn
}

func (c *runtimeCompiler) PrepareTurn(
	_ context.Context,
	session *agentcontext.TurnSession,
	_ agentcontext.PrepareTurnRequest,
) (*agentcontext.PreparedTurn, error) {
	turn := *c.prepared
	turn.Session = session
	turn.Profile = session.Profile
	return &turn, nil
}

func (c *runtimeCompiler) CompileModelInput(
	_ context.Context,
	req agentcontext.CompileRequest,
) (*agentcontext.CompiledContext, error) {
	return &agentcontext.CompiledContext{
		Messages: []*schema.Message{schema.UserMessage("compiled")},
		Record: agentcontext.CompileRecord{
			ModelCallID: "call-1",
			Manifest: agentcontext.ContextManifest{
				ProfileID:      req.Turn.Profile.Key.Name,
				ProfileVersion: req.Turn.Profile.Key.Version,
				ContextHMAC:    "hmac",
			},
		},
	}, nil
}

type runtimeManifestWriter struct {
	calls int
	last  writeback.ManifestWriteRequest
}

func (w *runtimeManifestWriter) StoreManifest(
	_ context.Context,
	req writeback.ManifestWriteRequest,
) error {
	w.calls++
	w.last = req
	return nil
}

type runtimeAssistantWriter struct {
	calls int
}

func (w *runtimeAssistantWriter) CommitAssistant(
	_ context.Context,
	req writeback.AssistantWriteRequest,
) (writeback.CommittedMessage, error) {
	w.calls++
	return writeback.CommittedMessage{MessageID: 1}, nil
}

func newTestRuntime(
	t *testing.T,
	cfg config.ContextManagementConfig,
	assistant *runtimeAssistantWriter,
	manifest *runtimeManifestWriter,
) (*Runtime, *MemoryStore) {
	t.Helper()
	registry, err := agentcontext.NewRegistry(
		[]agentcontext.ContextProfile{agentcontext.NewChatV1Profile()},
		nil,
	)
	require.NoError(t, err)
	store := NewMemoryStore()
	runtime, err := NewRuntime(RuntimeConfig{
		ContextConfig: cfg,
		Registry:      registry,
		Compiler: &runtimeCompiler{
			prepared: &agentcontext.PreparedTurn{
				BaseManifest: agentcontext.ContextManifest{
					ProfileID:      agentcontext.ChatV1.Name,
					ProfileVersion: agentcontext.ChatV1.Version,
				},
			},
		},
		Store: store,
		Writers: writeback.WriterRegistry{
			Assistant: assistant,
			Manifest:  manifest,
		},
	})
	require.NoError(t, err)
	return runtime, store
}

func TestRuntime_EnabledCompletesWithSingleWritebackOwner(t *testing.T) {
	assistant := &runtimeAssistantWriter{}
	manifest := &runtimeManifestWriter{}
	runtime, store := newTestRuntime(t, config.ContextManagementConfig{
		Mode:             "enabled",
		WritebackEnabled: true,
	}, assistant, manifest)

	ctx, execution, err := runtime.BeginChat(context.Background(), BeginChatRequest{
		UserID:         42,
		ConversationID: 9,
		Content:        "hello",
		Model:          agentcontext.ModelRef{Provider: "test", ModelID: "model"},
	})
	require.NoError(t, err)
	assert.True(t, runtime.UsesContextWriteback(execution))

	result, err := runtime.FinalizeChat(ctx, execution, FinalizeChatRequest{
		Status:  agentcontext.TurnStatusSuccess,
		Content: "answer",
	})
	require.NoError(t, err)
	assert.NotNil(t, result.Primary)
	assert.Equal(t, 1, assistant.calls)
	assert.Equal(t, 1, manifest.calls)

	record, err := store.GetRun(context.Background(), execution.PreparedTurn.Session.Handle.RunID)
	require.NoError(t, err)
	assert.Equal(t, RunStateCompleted, record.State)
}

func TestRuntime_ShadowCompletesManifestWithoutAssistantWrite(t *testing.T) {
	assistant := &runtimeAssistantWriter{}
	manifest := &runtimeManifestWriter{}
	runtime, _ := newTestRuntime(t, config.ContextManagementConfig{
		Mode:                    "shadow",
		RolloutVersion:          "v1",
		ShadowSampleBasisPoints: 10000,
	}, assistant, manifest)

	ctx, execution, err := runtime.BeginChat(context.Background(), BeginChatRequest{
		UserID:         42,
		ConversationID: 9,
		Content:        "hello",
		Model:          agentcontext.ModelRef{Provider: "test", ModelID: "model"},
	})
	require.NoError(t, err)
	assert.False(t, runtime.UsesContextWriteback(execution))

	_, err = runtime.FinalizeChat(ctx, execution, FinalizeChatRequest{
		Status:  agentcontext.TurnStatusSuccess,
		Content: "answer",
	})
	require.NoError(t, err)
	assert.Zero(t, assistant.calls)
	assert.Equal(t, 1, manifest.calls)
}

func TestRuntime_LegacyDoesNotCreateRun(t *testing.T) {
	runtime, store := newTestRuntime(t, config.ContextManagementConfig{
		Mode: "legacy",
	}, &runtimeAssistantWriter{}, &runtimeManifestWriter{})

	ctx := context.Background()
	returnedCtx, execution, err := runtime.BeginChat(ctx, BeginChatRequest{UserID: 42})
	require.NoError(t, err)
	assert.Equal(t, ctx, returnedCtx)
	assert.Equal(t, "legacy", execution.Mode.Mode)
	assert.Empty(t, store.runs)
}

func TestRuntime_AssignsStablePerRunModelCallSequence(t *testing.T) {
	runtime, _ := newTestRuntime(t, config.ContextManagementConfig{
		Mode: "legacy",
	}, &runtimeAssistantWriter{}, &runtimeManifestWriter{})

	runtime.recordCompile("run-1", agentcontext.CompileRecord{ModelCallID: "compiler-default"})
	runtime.recordCompile("run-1", agentcontext.CompileRecord{ModelCallID: "compiler-default"})

	records := runtime.compileRecords("run-1")
	require.Len(t, records, 2)
	assert.Equal(t, "run-1-call-1", records[0].ModelCallID)
	assert.Equal(t, "run-1-call-2", records[1].ModelCallID)
}
