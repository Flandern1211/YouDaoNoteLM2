package adapter

import (
	"context"
	"fmt"

	"YoudaoNoteLm/internal/agent/chat/prompts"
	"YoudaoNoteLm/internal/agentcontext"
)

// PromptSourceLookup 根据 AgentID 和 ProfileKey 返回该 Agent 的资料列表。
// 首期由调用方注入；未来可由应用层统一提供。
type PromptSourceLookup interface {
	GetSources(ctx context.Context, agentID agentcontext.AgentID) ([]prompts.SourceInfo, error)
}

// StaticPromptProvider 适配现有 prompts 包，实现 PromptProvider 接口。
// 返回受信任 system 内容，但 source 元数据只能作为模板数据。
type StaticPromptProvider struct {
	lookup PromptSourceLookup
}

// NewStaticPromptProvider 创建静态 Prompt 提供者
func NewStaticPromptProvider(lookup PromptSourceLookup) *StaticPromptProvider {
	return &StaticPromptProvider{lookup: lookup}
}

// LoadPrompt 加载 Prompt，使用统一的 prompts.RenderSystemPrompt 渲染。
// Prompt 属于必要能力：未找到或为空时返回错误。
func (p *StaticPromptProvider) LoadPrompt(ctx context.Context, query agentcontext.PromptQuery) (agentcontext.Prompt, error) {
	var sources []prompts.SourceInfo

	if p.lookup != nil {
		var err error
		sources, err = p.lookup.GetSources(ctx, query.AgentID)
		if err != nil {
			return agentcontext.Prompt{}, fmt.Errorf("获取资料列表失败: %w", err)
		}
	}

	content := prompts.RenderSystemPrompt(sources)
	if content == "" {
		return agentcontext.Prompt{}, agentcontext.NewError(
			agentcontext.ErrCodeProviderExhausted,
			"渲染后的 Prompt 为空",
		)
	}

	return agentcontext.Prompt{
		ID:      fmt.Sprintf("%s.%s", query.ProfileKey.Name, query.ProfileKey.Version),
		Version: "v1",
		Content: content,
	}, nil
}

// StaticSourceLookup 静态资料列表查找（用于测试和简单场景）
type StaticSourceLookup struct {
	Sources []prompts.SourceInfo
}

// GetSources 返回预设的资料列表
func (l *StaticSourceLookup) GetSources(_ context.Context, _ agentcontext.AgentID) ([]prompts.SourceInfo, error) {
	return l.Sources, nil
}
