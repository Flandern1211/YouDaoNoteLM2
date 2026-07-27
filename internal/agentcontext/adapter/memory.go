package adapter

import (
	"context"

	"YoudaoNoteLm/internal/agentcontext"
)

// DisabledMemoryProvider 返回明确的 skipped/not_configured 状态和空候选。
// 用于 chat.v1/main.v1 在未配置记忆实现时的可观测跳过。
type DisabledMemoryProvider struct{}

// NewDisabledMemoryProvider 创建禁用的记忆提供者
func NewDisabledMemoryProvider() *DisabledMemoryProvider {
	return &DisabledMemoryProvider{}
}

// SearchMemory 返回空候选和 nil 错误，表示记忆功能未启用。
// 调用方应将此视为 skipped 状态，记录到 Manifest 中。
func (p *DisabledMemoryProvider) SearchMemory(_ context.Context, _ agentcontext.MemoryQuery) ([]agentcontext.MemoryCandidate, error) {
	return nil, nil
}

// ExternalMemorySearcher 外部记忆搜索接口。
// 真实记忆算法由外部可替换模块实现，不在本仓库内。
type ExternalMemorySearcher interface {
	Search(ctx context.Context, userID uint, query string, limit int) ([]MemoryResult, error)
}

// MemoryResult 外部记忆搜索结果
type MemoryResult struct {
	ID         string
	Content    string
	Score      float64
	Importance float64
	Pinned     bool
}

// DelegatingMemoryProvider 包装注入的外部记忆接口，映射成 ContextItem。
// 不在本仓库实现新的记忆算法。
type DelegatingMemoryProvider struct {
	searcher ExternalMemorySearcher
}

// NewDelegatingMemoryProvider 创建委托的记忆提供者
func NewDelegatingMemoryProvider(searcher ExternalMemorySearcher) *DelegatingMemoryProvider {
	return &DelegatingMemoryProvider{searcher: searcher}
}

// SearchMemory 委托给外部记忆搜索接口，映射结果为 MemoryCandidate。
func (p *DelegatingMemoryProvider) SearchMemory(ctx context.Context, query agentcontext.MemoryQuery) ([]agentcontext.MemoryCandidate, error) {
	if p.searcher == nil {
		return nil, nil
	}

	results, err := p.searcher.Search(ctx, query.UserID, query.Query, query.CandidateLimit)
	if err != nil {
		return nil, err
	}

	candidates := make([]agentcontext.MemoryCandidate, 0, len(results))
	for _, r := range results {
		candidates = append(candidates, agentcontext.MemoryCandidate{
			ID:          r.ID,
			Content:     r.Content,
			Score:       r.Score,
			Importance:  r.Importance,
			Pinned:      r.Pinned,
			Sensitivity: agentcontext.SensitivityLow, // 首期默认低敏感度
			Provenance: agentcontext.Provenance{
				Provider: "delegating_memory",
				Stage:    "search",
			},
		})
	}

	return candidates, nil
}
