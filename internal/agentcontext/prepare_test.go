package agentcontext

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

type fakeSleeper struct {
	sleepCount int32
}

func (f *fakeSleeper) Sleep(_ time.Duration) {
	atomic.AddInt32(&f.sleepCount, 1)
}

type fakePromptProvider struct {
	prompt Prompt
	err    error
	calls  int32
}

func (f *fakePromptProvider) LoadPrompt(_ context.Context, _ PromptQuery) (Prompt, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.prompt, f.err
}

type fakeHistoryProvider struct {
	snapshot HistorySnapshot
	err      error
	calls    int32
}

func (f *fakeHistoryProvider) LoadHistory(_ context.Context, _ HistoryQuery) (HistorySnapshot, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.snapshot, f.err
}

type fakeMemoryProvider struct {
	candidates []MemoryCandidate
	err        error
	calls      int32
}

func (f *fakeMemoryProvider) SearchMemory(_ context.Context, _ MemoryQuery) ([]MemoryCandidate, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.candidates, f.err
}

type fakeModelResolver struct {
	caps ModelCapabilities
	err  error
}

func (f *fakeModelResolver) ResolveModel(_ context.Context, _ ModelRef) (ModelCapabilities, error) {
	return f.caps, f.err
}

// --- tests ---

func TestPrepareTurn_BasicChatProfile(t *testing.T) {
	registry := newTestRegistry()
	config := PrepareTurnConfig{
		Registry:       registry,
		PromptProvider: &fakePromptProvider{prompt: Prompt{ID: "chat.v1", Version: "v1", Content: "system prompt"}},
		HistoryProvider: &fakeHistoryProvider{snapshot: HistorySnapshot{
			Summary:  &ConversationSummary{Content: "previous summary"},
			Messages: []HistoryMessage{{Role: "user", Content: "hello"}},
		}},
		MemoryProvider: &fakeMemoryProvider{candidates: []MemoryCandidate{{ID: "mem-1", Content: "memory"}}},
		ModelResolver:  &fakeModelResolver{caps: ModelCapabilities{ContextWindow: 128000, MaxOutputTokens: 4096}},
		Sleeper:        &fakeSleeper{},
	}

	chatProfile := NewChatV1Profile()
	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			RunID:          "run-1",
			StepID:         "step-1",
			AgentID:        AgentIDChat,
			UserID:         1,
			ConversationID: 1,
			Input:          UserMessageInput{Content: "test query"},
		},
		Profile: chatProfile.ToSnapshot(),
	}

	turn, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{
		Model: ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	}, config)

	require.NoError(t, err)
	assert.NotNil(t, turn)
	assert.Equal(t, "system prompt", turn.Instruction)
	assert.NotNil(t, turn.MessagePlan.Summary)
	assert.Len(t, turn.MessagePlan.Memories, 1)
	assert.Equal(t, "prepared", turn.BaseManifest.TurnStatus)
	assert.False(t, turn.BaseManifest.Degraded)
}

func TestPrepareTurn_SearchProfile_SkipsHistoryAndMemory(t *testing.T) {
	registry := newTestRegistry()
	historyProvider := &fakeHistoryProvider{}
	memoryProvider := &fakeMemoryProvider{}

	config := PrepareTurnConfig{
		Registry:        registry,
		PromptProvider:  &fakePromptProvider{prompt: Prompt{ID: "search.v1", Version: "v1", Content: "search prompt"}},
		HistoryProvider: historyProvider,
		MemoryProvider:  memoryProvider,
		ModelResolver:   &fakeModelResolver{caps: ModelCapabilities{ContextWindow: 64000}},
		Sleeper:         &fakeSleeper{},
	}

	searchProfile := NewSearchV1Profile()
	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			RunID:   "run-1",
			StepID:  "step-1",
			AgentID: AgentIDSearch,
			Input:   SearchTaskInput{Task: SearchTask{Query: "search query"}},
		},
		Profile: searchProfile.ToSnapshot(),
	}

	turn, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{
		Model: ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	}, config)

	require.NoError(t, err)
	assert.NotNil(t, turn)
	// Search Profile 不调用 History/Memory
	assert.Equal(t, int32(0), atomic.LoadInt32(&historyProvider.calls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&memoryProvider.calls))
	assert.Nil(t, turn.MessagePlan.Summary)
	assert.Nil(t, turn.MessagePlan.Memories)
}

func TestPrepareTurn_PromptRequired_AbortOnFailure(t *testing.T) {
	registry := newTestRegistry()
	config := PrepareTurnConfig{
		Registry:       registry,
		PromptProvider: &fakePromptProvider{err: errors.New("prompt failed")},
		ModelResolver:  &fakeModelResolver{caps: ModelCapabilities{ContextWindow: 128000}},
		Sleeper:        &fakeSleeper{},
	}

	chatProfile := NewChatV1Profile()
	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			AgentID: AgentIDChat,
			Input:   UserMessageInput{Content: "test"},
		},
		Profile: chatProfile.ToSnapshot(),
	}

	_, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{
		Model: ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	}, config)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt failed")
}

func TestPrepareTurn_MemoryOptional_SkipOnFailure(t *testing.T) {
	registry := newTestRegistry()
	config := PrepareTurnConfig{
		Registry:        registry,
		PromptProvider:  &fakePromptProvider{prompt: Prompt{Content: "prompt"}},
		HistoryProvider: &fakeHistoryProvider{},
		MemoryProvider:  &fakeMemoryProvider{err: errors.New("memory failed")},
		ModelResolver:   &fakeModelResolver{caps: ModelCapabilities{ContextWindow: 128000}},
		Sleeper:         &fakeSleeper{},
	}

	chatProfile := NewChatV1Profile()
	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			AgentID:        AgentIDChat,
			UserID:         1,
			ConversationID: 1,
			Input:          UserMessageInput{Content: "test"},
		},
		Profile: chatProfile.ToSnapshot(),
	}

	turn, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{
		Model: ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	}, config)

	require.NoError(t, err)
	assert.True(t, turn.BaseManifest.Degraded)
	assert.Equal(t, "degraded", turn.BaseManifest.TurnStatus)
	assert.Nil(t, turn.MessagePlan.Memories)
}

func TestPrepareTurn_RequiredProvider_AbortOnHistoryFailure(t *testing.T) {
	registry := newTestRegistry()
	config := PrepareTurnConfig{
		Registry:        registry,
		PromptProvider:  &fakePromptProvider{prompt: Prompt{Content: "prompt"}},
		HistoryProvider: &fakeHistoryProvider{err: errors.New("history failed")},
		ModelResolver:   &fakeModelResolver{caps: ModelCapabilities{ContextWindow: 128000}},
		Sleeper:         &fakeSleeper{},
	}

	chatProfile := NewChatV1Profile()
	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			AgentID:        AgentIDChat,
			ConversationID: 1,
			Input:          UserMessageInput{Content: "test"},
		},
		Profile: chatProfile.ToSnapshot(),
	}

	_, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{
		Model: ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	}, config)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "history failed")
}

func TestPrepareTurn_ContextCancellation_StopsRetries(t *testing.T) {
	registry := newTestRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	config := PrepareTurnConfig{
		Registry:       registry,
		PromptProvider: &fakePromptProvider{prompt: Prompt{Content: "prompt"}},
		ModelResolver:  &fakeModelResolver{caps: ModelCapabilities{ContextWindow: 128000}},
		Sleeper:        &fakeSleeper{},
	}

	chatProfile := NewChatV1Profile()
	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			AgentID: AgentIDChat,
			Input:   UserMessageInput{Content: "test"},
		},
		Profile: chatProfile.ToSnapshot(),
	}

	_, err := PrepareTurn(ctx, session, PrepareTurnRequest{
		Model: ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	}, config)

	require.Error(t, err)
}

func TestPrepareTurn_NilSession(t *testing.T) {
	registry := newTestRegistry()
	config := PrepareTurnConfig{
		Registry:      registry,
		ModelResolver: &fakeModelResolver{},
		Sleeper:       &fakeSleeper{},
	}

	_, err := PrepareTurn(context.Background(), nil, PrepareTurnRequest{}, config)

	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeInvalidInput))
}

func TestPrepareTurn_ProfileNotFound(t *testing.T) {
	registry := newTestRegistry()
	config := PrepareTurnConfig{
		Registry:      registry,
		ModelResolver: &fakeModelResolver{},
		Sleeper:       &fakeSleeper{},
	}

	session := &TurnSession{
		Handle: AcceptedTurnHandle{AgentID: AgentIDChat},
		Profile: ContextProfileSnapshot{
			Key: ProfileKey{Name: "unknown", Version: "v1"},
		},
	}

	_, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{}, config)

	require.Error(t, err)
	assert.True(t, IsErrorCode(err, ErrCodeProfileNotFound))
}

func TestPrepareTurn_UnknownModel(t *testing.T) {
	registry := newTestRegistry()
	config := PrepareTurnConfig{
		Registry:      registry,
		ModelResolver: &fakeModelResolver{err: NewError(ErrCodeModelUnknown, "unknown model")},
		Sleeper:       &fakeSleeper{},
	}

	chatProfile := NewChatV1Profile()
	session := &TurnSession{
		Handle:  AcceptedTurnHandle{AgentID: AgentIDChat},
		Profile: chatProfile.ToSnapshot(),
	}

	_, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{
		Model: ModelRef{Provider: "unknown", ModelID: "model"},
	}, config)

	require.Error(t, err)
}

func TestPrepareTurn_ParallelExecution(t *testing.T) {
	// 验证独立 Provider 确实并行执行
	registry := newTestRegistry()

	// 使用延迟来验证并行性
	slowPrompt := &slowPromptProvider{delay: 50 * time.Millisecond}
	slowHistory := &slowHistoryProvider{delay: 50 * time.Millisecond}
	slowMemory := &slowMemoryProvider{delay: 50 * time.Millisecond}

	config := PrepareTurnConfig{
		Registry:        registry,
		PromptProvider:  slowPrompt,
		HistoryProvider: slowHistory,
		MemoryProvider:  slowMemory,
		ModelResolver:   &fakeModelResolver{caps: ModelCapabilities{ContextWindow: 128000}},
		Sleeper:         &fakeSleeper{},
	}

	chatProfile := NewChatV1Profile()
	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			AgentID:        AgentIDChat,
			UserID:         1,
			ConversationID: 1,
			Input:          UserMessageInput{Content: "test"},
		},
		Profile: chatProfile.ToSnapshot(),
	}

	start := time.Now()
	turn, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{
		Model: ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	}, config)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.NotNil(t, turn)

	// 如果串行执行，需要 ~150ms；并行应该 ~50ms
	// 允许一些误差
	assert.Less(t, elapsed, 120*time.Millisecond, "Provider 应该并行执行")
}

func TestPrepareTurn_StableProviderDedup(t *testing.T) {
	// 验证同一请求内每个稳定 Provider 只解析一次
	registry := newTestRegistry()
	promptProvider := &fakePromptProvider{prompt: Prompt{Content: "prompt"}}
	historyProvider := &fakeHistoryProvider{}
	memoryProvider := &fakeMemoryProvider{}

	config := PrepareTurnConfig{
		Registry:        registry,
		PromptProvider:  promptProvider,
		HistoryProvider: historyProvider,
		MemoryProvider:  memoryProvider,
		ModelResolver:   &fakeModelResolver{caps: ModelCapabilities{ContextWindow: 128000}},
		Sleeper:         &fakeSleeper{},
	}

	chatProfile := NewChatV1Profile()
	session := &TurnSession{
		Handle: AcceptedTurnHandle{
			AgentID:        AgentIDChat,
			UserID:         1,
			ConversationID: 1,
			Input:          UserMessageInput{Content: "test"},
		},
		Profile: chatProfile.ToSnapshot(),
	}

	_, err := PrepareTurn(context.Background(), session, PrepareTurnRequest{
		Model: ModelRef{Provider: "openai", ModelID: "gpt-4o"},
	}, config)

	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&promptProvider.calls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&historyProvider.calls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&memoryProvider.calls))
}

// --- slow providers for parallelism testing ---

type slowPromptProvider struct {
	delay time.Duration
}

func (p *slowPromptProvider) LoadPrompt(_ context.Context, _ PromptQuery) (Prompt, error) {
	time.Sleep(p.delay)
	return Prompt{ID: "test", Version: "v1", Content: "prompt"}, nil
}

type slowHistoryProvider struct {
	delay time.Duration
}

func (p *slowHistoryProvider) LoadHistory(_ context.Context, _ HistoryQuery) (HistorySnapshot, error) {
	time.Sleep(p.delay)
	return HistorySnapshot{}, nil
}

type slowMemoryProvider struct {
	delay time.Duration
}

func (p *slowMemoryProvider) SearchMemory(_ context.Context, _ MemoryQuery) ([]MemoryCandidate, error) {
	time.Sleep(p.delay)
	return nil, nil
}

// --- helper ---

func newTestRegistry() *Registry {
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
	r, _ := NewRegistry(profiles, models)
	return r
}
