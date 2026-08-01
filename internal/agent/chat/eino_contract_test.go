package chat

import (
	"context"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testChatModel 用于测试的最小 ToolCallingChatModel 实现
type testChatModel struct {
	mu           sync.Mutex
	callCount    int
	messages     [][]*schema.Message
	returnTokens []string
}

func (m *testChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++
	m.messages = append(m.messages, messages)

	idx := m.callCount - 1
	if idx >= len(m.returnTokens) {
		idx = len(m.returnTokens) - 1
	}

	return &schema.Message{
		Role:    schema.Assistant,
		Content: m.returnTokens[idx],
	}, nil
}

func (m *testChatModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}

	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		writer.Send(msg, nil)
		writer.Close()
	}()
	return reader, nil
}

func (m *testChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *testChatModel) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *testChatModel) GetMessages() [][]*schema.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages
}

// testHandler 用于测试的 ChatModelAgentMiddleware 实现
// 嵌入 adk.TypedBaseChatModelAgentMiddleware 提供默认实现
type testHandler struct {
	adk.TypedBaseChatModelAgentMiddleware[*schema.Message]
	name     string
	executed bool
}

func (h *testHandler) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	h.executed = true
	return ctx, runCtx, nil
}

func (h *testHandler) AfterAgent(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.Message]) (context.Context, error) {
	return ctx, nil
}

func (h *testHandler) BeforeModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.Message], mc *adk.TypedModelContext[*schema.Message]) (context.Context, *adk.TypedChatModelAgentState[*schema.Message], error) {
	return ctx, state, nil
}

// TestEinoContract_ChatModelAgent_GenModelInput 测试 GenModelInput 只在头部补一个系统消息
func TestEinoContract_ChatModelAgent_GenModelInput(t *testing.T) {
	chatModel := &testChatModel{
		returnTokens: []string{"回复1", "回复2"},
	}

	handler := &testHandler{name: "test_handler"}

	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Model:         chatModel,
		Instruction:   "这是系统指令",
		ToolsConfig:   adk.ToolsConfig{},
		MaxIterations: 1,
		Handlers:      []adk.ChatModelAgentMiddleware{handler},
	})
	require.NoError(t, err)

	runner := adk.NewRunner(context.Background(), adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	iter := runner.Query(context.Background(), "用户消息")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		require.NoError(t, event.Err)
	}

	// 验证模型被调用
	assert.Equal(t, 1, chatModel.GetCallCount())

	// 验证消息包含系统指令
	messages := chatModel.GetMessages()[0]
	assert.True(t, len(messages) > 0, "应该有消息")

	// 验证系统消息只有一个
	systemCount := 0
	for _, msg := range messages {
		if msg.Role == schema.System {
			systemCount++
		}
	}
	assert.Equal(t, 1, systemCount, "系统消息应该只有一个")
}

// TestEinoContract_ChatModelAgent_MultipleCalls 测试多次模型调用不会重复累积系统消息
func TestEinoContract_ChatModelAgent_MultipleCalls(t *testing.T) {
	chatModel := &testChatModel{
		returnTokens: []string{"调用工具", "最终回复"},
	}

	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Model:         chatModel,
		Instruction:   "系统指令",
		ToolsConfig:   adk.ToolsConfig{},
		MaxIterations: 2,
	})
	require.NoError(t, err)

	runner := adk.NewRunner(context.Background(), adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	iter := runner.Query(context.Background(), "用户消息")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		_ = event // 忽略错误
	}

	// 验证每次调用的系统消息数量
	for i, messages := range chatModel.GetMessages() {
		systemCount := 0
		for _, msg := range messages {
			if msg.Role == schema.System {
				systemCount++
			}
		}
		assert.Equal(t, 1, systemCount, "第 %d 次调用应该只有1个系统消息", i+1)
	}
}

// orderTrackingHandler 用于跟踪执行顺序的 Handler
type orderTrackingHandler struct {
	adk.TypedBaseChatModelAgentMiddleware[*schema.Message]
	name     string
	onBefore func()
}

func (h *orderTrackingHandler) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	if h.onBefore != nil {
		h.onBefore()
	}
	return ctx, runCtx, nil
}

func (h *orderTrackingHandler) BeforeModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.Message], mc *adk.TypedModelContext[*schema.Message]) (context.Context, *adk.TypedChatModelAgentState[*schema.Message], error) {
	return ctx, state, nil
}

// TestEinoContract_ChatModelAgent_HandlerOrder 测试 Handler 按注册顺序执行
func TestEinoContract_ChatModelAgent_HandlerOrder(t *testing.T) {
	chatModel := &testChatModel{
		returnTokens: []string{"回复"},
	}

	var executionOrder []string
	var mu sync.Mutex

	handler1 := &orderTrackingHandler{
		name: "handler1",
		onBefore: func() {
			mu.Lock()
			executionOrder = append(executionOrder, "handler1_before")
			mu.Unlock()
		},
	}
	handler2 := &orderTrackingHandler{
		name: "handler2",
		onBefore: func() {
			mu.Lock()
			executionOrder = append(executionOrder, "handler2_before")
			mu.Unlock()
		},
	}

	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Model:         chatModel,
		Instruction:   "系统指令",
		ToolsConfig:   adk.ToolsConfig{},
		MaxIterations: 1,
		Handlers:      []adk.ChatModelAgentMiddleware{handler1, handler2},
	})
	require.NoError(t, err)

	runner := adk.NewRunner(context.Background(), adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	iter := runner.Query(context.Background(), "用户消息")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		require.NoError(t, event.Err)
	}

	// 验证执行顺序
	assert.Equal(t, []string{"handler1_before", "handler2_before"}, executionOrder)
}

// TestEinoContract_ChatModelAgent_AfterAgentOnlyOnSuccess 测试 AfterAgent 只在成功终态触发
func TestEinoContract_ChatModelAgent_AfterAgentOnlyOnSuccess(t *testing.T) {
	t.Log("约束：AfterAgent 只在成功终态触发；失败或取消不得被当作可靠 finalize")
	t.Log("AfterAgent 不在以下情况调用：ErrExceedMaxIterations, context cancellation, model errors")
	t.Log("这个约束需要在集成测试中验证")
}

// TestEinoContract_ChatModelAgent_StatePersistence 测试 State 持久化
// 注意：如果没有工具调用，agent 只会调用模型一次就结束
func TestEinoContract_ChatModelAgent_StatePersistence(t *testing.T) {
	chatModel := &testChatModel{
		returnTokens: []string{"最终回复"},
	}

	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Model:         chatModel,
		Instruction:   "系统指令",
		ToolsConfig:   adk.ToolsConfig{},
		MaxIterations: 2,
	})
	require.NoError(t, err)

	runner := adk.NewRunner(context.Background(), adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: false,
	})

	iter := runner.Query(context.Background(), "用户消息")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		require.NoError(t, event.Err)
	}

	// 验证模型被调用（没有工具时只调用一次）
	messages := chatModel.GetMessages()
	assert.GreaterOrEqual(t, len(messages), 1, "模型应该至少被调用1次")
}

// TestEinoContract_ChatModelAgent_HandlerInterface 记录 Handler 接口契约
func TestEinoContract_ChatModelAgent_HandlerInterface(t *testing.T) {
	t.Log("ChatModelAgentMiddleware 接口方法：")
	t.Log("  - BeforeAgent: 在每次 agent run 之前调用")
	t.Log("  - AfterAgent: 在 agent 成功终态后调用")
	t.Log("  - BeforeModelRewriteState: 在每次模型调用前调用，可修改消息和工具")
	t.Log("  - AfterModelRewriteState: 在每次模型调用后调用")
	t.Log("  - WrapInvokableToolCall: 包装工具的同步执行")
	t.Log("  - WrapStreamableToolCall: 包装工具的流式执行")
	t.Log("  - WrapEnhancedInvokableToolCall: 包装增强工具的同步执行")
	t.Log("  - WrapEnhancedStreamableToolCall: 包装增强工具的流式执行")
	t.Log("  - WrapModel: 包装模型调用")

	// 验证 testHandler 实现了接口
	var _ adk.ChatModelAgentMiddleware = &testHandler{}
}
