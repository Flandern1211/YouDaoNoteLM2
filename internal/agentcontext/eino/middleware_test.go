package eino

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

// mockCompiler 用于测试的 ContextCompiler 实现
type mockCompiler struct {
	prepareResult *agentcontext.PreparedTurn
	prepareErr    error
	compileResult *agentcontext.CompiledContext
	compileErr    error
	compileCalls  int
}

func (m *mockCompiler) PrepareTurn(
	ctx context.Context,
	session *agentcontext.TurnSession,
	req agentcontext.PrepareTurnRequest,
) (*agentcontext.PreparedTurn, error) {
	return m.prepareResult, m.prepareErr
}

func (m *mockCompiler) CompileModelInput(
	ctx context.Context,
	req agentcontext.CompileRequest,
) (*agentcontext.CompiledContext, error) {
	m.compileCalls++
	return m.compileResult, m.compileErr
}

func TestContextMiddleware_LegacyMode_NoChange(t *testing.T) {
	compiler := &mockCompiler{}
	middleware := NewContextMiddleware(ContextMiddlewareConfig{
		Compiler: compiler,
		Mode:     ContextModeLegacy,
	})

	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.UserMessage("hello"),
		},
	}

	// Legacy 模式没有 PreparedTurn，不修改 state
	ctx := context.Background()
	newCtx, newState, err := middleware.BeforeModelRewriteState(ctx, state, nil)
	require.NoError(t, err)
	assert.Equal(t, ctx, newCtx)
	assert.Equal(t, state, newState)
	assert.Equal(t, 0, compiler.compileCalls)
}

func TestContextMiddleware_ShadowMode_WithPreparedTurn(t *testing.T) {
	compiler := &mockCompiler{
		compileResult: &agentcontext.CompiledContext{
			Messages: []*schema.Message{
				schema.UserMessage("hello"),
			},
			Record: agentcontext.CompileRecord{
				Manifest: agentcontext.ContextManifest{
					EstimatedTokens: 100,
					TurnStatus:      "prepared",
				},
			},
		},
	}

	var sinkRecord *ShadowCompareRecord
	middleware := NewContextMiddleware(ContextMiddlewareConfig{
		Compiler: compiler,
		Mode:     ContextModeShadow,
		ShadowSink: func(r ShadowCompareRecord) {
			sinkRecord = &r
		},
	})

	turn := &agentcontext.PreparedTurn{
		Session: &agentcontext.TurnSession{
			Handle: agentcontext.AcceptedTurnHandle{RunID: "test-run"},
		},
		Profile: agentcontext.ContextProfileSnapshot{
			Key: agentcontext.ChatV1,
		},
	}

	// 注入 PreparedTurn 到 context
	ctx := context.WithValue(context.Background(), contextCompilerKey{}, turn)

	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.UserMessage("hello"),
		},
	}

	newCtx, newState, err := middleware.BeforeModelRewriteState(ctx, state, nil)
	require.NoError(t, err)

	// Shadow 不修改 state
	assert.Equal(t, state, newState)
	assert.NotNil(t, newCtx)

	// 验证 Shadow 编译被调用
	assert.Equal(t, 1, compiler.compileCalls)

	// 验证比较记录
	require.NotNil(t, sinkRecord)
	assert.Equal(t, "test-run", sinkRecord.TurnID)
	assert.Equal(t, 1, sinkRecord.LegacyMsgCount)
	assert.Equal(t, 1, sinkRecord.ShadowMsgCount)
	assert.True(t, sinkRecord.RoleOrderMatch)
}

func TestContextMiddleware_ShadowMode_CompileError(t *testing.T) {
	compiler := &mockCompiler{
		compileErr: assert.AnError,
	}

	var sinkRecord *ShadowCompareRecord
	middleware := NewContextMiddleware(ContextMiddlewareConfig{
		Compiler: compiler,
		Mode:     ContextModeShadow,
		ShadowSink: func(r ShadowCompareRecord) {
			sinkRecord = &r
		},
	})

	turn := &agentcontext.PreparedTurn{
		Session: &agentcontext.TurnSession{
			Handle: agentcontext.AcceptedTurnHandle{RunID: "test-run"},
		},
	}

	ctx := context.WithValue(context.Background(), contextCompilerKey{}, turn)
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.UserMessage("hello"),
		},
	}

	// Shadow 编译失败不改变用户请求结果
	_, newState, err := middleware.BeforeModelRewriteState(ctx, state, nil)
	require.NoError(t, err)
	assert.Equal(t, state, newState)

	// 验证错误被记录
	require.NotNil(t, sinkRecord)
	assert.NotEmpty(t, sinkRecord.Error)
}

func TestPrepareAndInject(t *testing.T) {
	turn := &agentcontext.PreparedTurn{
		Session: &agentcontext.TurnSession{
			Handle: agentcontext.AcceptedTurnHandle{RunID: "test-run"},
		},
		Profile: agentcontext.ContextProfileSnapshot{
			Key: agentcontext.ChatV1,
		},
	}

	compiler := &mockCompiler{
		prepareResult: turn,
	}

	session := &agentcontext.TurnSession{
		Handle: agentcontext.AcceptedTurnHandle{RunID: "test-run"},
	}

	ctx, preparedTurn, err := PrepareAndInject(
		context.Background(),
		compiler,
		session,
		agentcontext.PrepareTurnRequest{},
	)
	require.NoError(t, err)
	assert.NotNil(t, preparedTurn)
	assert.Equal(t, "test-run", preparedTurn.Session.Handle.RunID)

	// 验证可以从 context 中取回
	retrieved := getPreparedTurnFromContext(ctx)
	assert.Equal(t, turn, retrieved)
}

func TestGetPreparedTurnFromContext_Nil(t *testing.T) {
	turn := getPreparedTurnFromContext(context.Background())
	assert.Nil(t, turn)
}

func TestCompareRoleOrder_SameOrder(t *testing.T) {
	a := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}
	b := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}
	assert.True(t, compareRoleOrder(a, b))
}

func TestCompareRoleOrder_DifferentOrder(t *testing.T) {
	a := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}
	b := []*schema.Message{
		schema.AssistantMessage("hi", nil),
		schema.UserMessage("hello"),
	}
	assert.False(t, compareRoleOrder(a, b))
}

func TestCompareRoleOrder_DifferentLength(t *testing.T) {
	a := []*schema.Message{
		schema.UserMessage("hello"),
	}
	b := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}
	assert.False(t, compareRoleOrder(a, b))
}

func TestContextMiddleware_EnabledMode_Rejected(t *testing.T) {
	// W4 不支持 enabled 模式，必须返回 nil
	middleware := NewContextMiddleware(ContextMiddlewareConfig{
		Compiler: &mockCompiler{},
		Mode:     ContextModeEnabled,
	})
	assert.Nil(t, middleware, "enabled 模式在 W4 应该被拒绝")
}
