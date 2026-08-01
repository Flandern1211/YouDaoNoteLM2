package agentcontext

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"
)

// Sleeper 可注入的 sleep 接口，测试中替换为立即返回。
type Sleeper interface {
	Sleep(d time.Duration)
}

// realSleeper 真实的 sleep 实现
type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) { time.Sleep(d) }

// PrepareTurnConfig PrepareTurn 的配置
type PrepareTurnConfig struct {
	Registry        *Registry
	PromptProvider  PromptProvider
	HistoryProvider HistoryProvider
	MemoryProvider  MemoryProvider
	ModelResolver   ModelCapabilitiesResolver
	Sleeper         Sleeper
}

// PrepareTurn 执行分阶段并行的上下文准备。
//
// 阶段 1（串行）：解析 Registry/Profile/模型能力
// 阶段 2（并行）：Prompt、History、Memory（按 Profile 决定是否启用）
// 阶段 3：标准化候选项，生成 PreparedTurn
//
// 对每个 Provider 执行其 retry/fallback/terminal policy。
// context 取消时立即停止重试并等待已启动 goroutine 退出。
func PrepareTurn(
	ctx context.Context,
	session *TurnSession,
	req PrepareTurnRequest,
	config PrepareTurnConfig,
) (*PreparedTurn, error) {
	if session == nil {
		return nil, NewError(ErrCodeInvalidInput, "session 不能为空")
	}

	sleeper := config.Sleeper
	if sleeper == nil {
		sleeper = realSleeper{}
	}

	// 阶段 1：串行解析 Registry/Profile/模型能力
	profile, ok := config.Registry.ResolveProfile(session.Profile.Key)
	if !ok {
		return nil, NewError(ErrCodeProfileNotFound,
			fmt.Sprintf("未找到 Profile: %s.%s", session.Profile.Key.Name, session.Profile.Key.Version))
	}

	modelCaps, err := config.ModelResolver.ResolveModel(ctx, req.Model)
	if err != nil {
		return nil, fmt.Errorf("解析模型能力失败: %w", err)
	}

	// 阶段 2：并行调用相互独立的 Provider
	type providerResult struct {
		prompt        *Prompt
		summary       *ConversationSummary
		memories      []MemoryCandidate
		historyMsgs   []HistoryMessage
		historyErr    error
		promptErr     error
		memoryErr     error
		memorySkipped bool
	}

	var result providerResult
	g, gctx := errgroup.WithContext(ctx)

	// Prompt Provider（必需）
	g.Go(func() error {
		prompt, err := retryProvider(gctx, sleeper, DefaultRetryPolicy(), func() (Prompt, error) {
			return config.PromptProvider.LoadPrompt(gctx, PromptQuery{
				AgentID:    session.Handle.AgentID,
				ProfileKey: session.Profile.Key,
			})
		})
		if err != nil {
			result.promptErr = err
			return err // Prompt 是必需的，失败则 abort
		}
		result.prompt = &prompt
		return nil
	})

	// History Provider（仅 Chat/Main）
	if (profile.LoadHistory || profile.LoadSummary) && config.HistoryProvider != nil {
		g.Go(func() error {
			history, err := retryProvider(gctx, sleeper, DefaultRetryPolicy(), func() (HistorySnapshot, error) {
				return config.HistoryProvider.LoadHistory(gctx, HistoryQuery{
					ConversationID:  session.Handle.ConversationID,
					CurrentInputRef: session.Handle.CurrentInputRef,
					Limit:           RecentRoundsLimit,
				})
			})
			if err != nil {
				result.historyErr = err
				return err // History 对 Chat/Main 是必需的
			}
			result.summary = history.Summary
			result.historyMsgs = history.Messages
			return nil
		})
	}

	// Memory Provider（仅 Chat/Main，可选）
	if profile.LoadMemory && config.MemoryProvider != nil {
		g.Go(func() error {
			query := extractMemoryQuery(session)
			candidates, err := retryProvider(gctx, sleeper, DefaultRetryPolicy(), func() ([]MemoryCandidate, error) {
				return config.MemoryProvider.SearchMemory(gctx, query)
			})
			if err != nil {
				result.memoryErr = err
				result.memorySkipped = true
				return nil // Memory 是可选的，失败不阻断
			}
			result.memories = candidates
			return nil
		})
	}

	// 等待所有 goroutine 完成
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("Provider 解析失败: %w", err)
	}

	// 阶段 3：标准化候选项，生成 PreparedTurn
	turn := &PreparedTurn{
		Session: session,
		Profile: profile.ToSnapshot(),
		MessagePlan: MessagePlan{
			Summary:      buildSummaryItem(result.summary),
			Memories:     buildMemoryItems(result.memories),
			History:      HistoryMessageToSchema(result.historyMsgs),
			CurrentInput: session.Handle.Input,
		},
		ModelCapabilities: modelCaps,
		BaseManifest: ContextManifest{
			ProfileID:      profile.Key.Name,
			ProfileVersion: profile.Key.Version,
			Model:          fmt.Sprintf("%s/%s", req.Model.Provider, req.Model.ModelID),
			TurnStatus:     "prepared",
		},
	}

	// 设置 Prompt 信息
	if result.prompt != nil {
		turn.Instruction = result.prompt.Content
		turn.BaseManifest.PromptVersion = result.prompt.Version
	}

	// 标记 degraded 状态
	if result.memorySkipped {
		turn.BaseManifest.Degraded = true
		turn.BaseManifest.TurnStatus = "degraded"
	}

	return turn, nil
}

// extractMemoryQuery 从 session 提取记忆查询
func extractMemoryQuery(session *TurnSession) MemoryQuery {
	query := MemoryQuery{
		UserID:         session.Handle.UserID,
		CandidateLimit: 10,
	}

	// 从输入中提取查询文本
	switch input := session.Handle.Input.(type) {
	case UserMessageInput:
		query.Query = input.Content
	case SearchTaskInput:
		query.Query = input.Task.Query
	}

	return query
}

// buildSummaryItem 从摘要构建 ContextItem
func buildSummaryItem(summary *ConversationSummary) *ContextItem {
	if summary == nil || summary.Content == "" {
		return nil
	}
	return &ContextItem{
		ID:      "summary",
		Kind:    ContextKindConversationSummary,
		Content: summary.Content,
		Trust:   TrustLevelUserProvided,
		Pinned:  true,
		Provenance: Provenance{
			Provider: "history",
			Stage:    "prepare",
		},
	}
}

// buildMemoryItems 从记忆候选构建 ContextItem 列表
func buildMemoryItems(candidates []MemoryCandidate) []ContextItem {
	if len(candidates) == 0 {
		return nil
	}

	items := make([]ContextItem, 0, len(candidates))
	for _, c := range candidates {
		items = append(items, ContextItem{
			ID:         c.ID,
			Kind:       ContextKindUserMemory,
			Content:    c.Content,
			Pinned:     c.Pinned,
			Trust:      TrustLevelUserProvided,
			Provenance: c.Provenance,
		})
	}
	return items
}

// retryProvider 对 Provider 调用执行重试策略。
// 返回值为泛型，适用于任何 Provider 返回类型。
func retryProvider[T any](
	ctx context.Context,
	sleeper Sleeper,
	policy RetryPolicy,
	fn func() (T, error),
) (T, error) {
	var zero T

	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		// 最后一次尝试，不再重试
		if attempt == policy.MaxAttempts-1 {
			return zero, err
		}

		// 检查错误是否可重试
		if policy.Retryable != nil && !policy.Retryable(err) {
			return zero, err
		}

		// 计算退避延迟并使用注入的 sleeper
		delay := calculateBackoff(policy.Backoff, attempt)

		// 等待退避延迟或 context 取消
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
			sleeper.Sleep(0) // 允许测试中控制时间
		}
	}

	return zero, NewError(ErrCodeProviderExhausted, "重试次数耗尽")
}

// calculateBackoff 计算退避延迟
func calculateBackoff(policy BackoffPolicy, attempt int) time.Duration {
	delay := float64(policy.InitialMs)
	for i := 0; i < attempt; i++ {
		delay *= policy.Multiplier
	}
	if delay > float64(policy.MaxMs) {
		delay = float64(policy.MaxMs)
	}
	return time.Duration(delay) * time.Millisecond
}

// NumGoroutine 返回当前 goroutine 数量，便于测试中检测泄漏。
var NumGoroutine = runtime.NumGoroutine
