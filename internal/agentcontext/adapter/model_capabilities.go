package adapter

import (
	"context"
	"fmt"

	"YoudaoNoteLm/internal/agentcontext"
)

// ModelConfigProvider 从现有模型配置获取能力信息的接口。
// 首期使用代码级 fixture；W6 实现配置化 override。
type ModelConfigProvider interface {
	GetModelCapabilities(ref agentcontext.ModelRef) (agentcontext.ModelCapabilities, bool)
}

// RegistryModelCapabilitiesResolver 包装 Registry 实现 ModelCapabilitiesResolver 接口。
// 未知模型必须通过显式 override 才能启动，不静默猜测容量。
type RegistryModelCapabilitiesResolver struct {
	registry *agentcontext.Registry
	// overrides 允许代码级覆盖未知模型的能力
	overrides map[string]agentcontext.ModelCapabilities
}

// NewRegistryModelCapabilitiesResolver 创建基于 Registry 的模型能力解析器
func NewRegistryModelCapabilitiesResolver(registry *agentcontext.Registry) *RegistryModelCapabilitiesResolver {
	return &RegistryModelCapabilitiesResolver{
		registry:  registry,
		overrides: make(map[string]agentcontext.ModelCapabilities),
	}
}

// WithOverride 添加模型能力覆盖（用于测试和未知模型）
func (r *RegistryModelCapabilitiesResolver) WithOverride(ref agentcontext.ModelRef, caps agentcontext.ModelCapabilities) *RegistryModelCapabilitiesResolver {
	key := modelKey(ref)
	r.overrides[key] = caps
	return r
}

// ResolveModel 解析模型能力。
// 优先从 Registry 查找，其次从 overrides 查找。
// 未知模型返回错误，不静默猜测。
func (r *RegistryModelCapabilitiesResolver) ResolveModel(_ context.Context, ref agentcontext.ModelRef) (agentcontext.ModelCapabilities, error) {
	// 1. 从 Registry 查找
	caps, ok := r.registry.ResolveModel(ref)
	if ok {
		return caps, nil
	}

	// 2. 从 overrides 查找
	key := modelKey(ref)
	caps, ok = r.overrides[key]
	if ok {
		return caps, nil
	}

	// 3. 未知模型
	return agentcontext.ModelCapabilities{}, agentcontext.NewError(
		agentcontext.ErrCodeModelUnknown,
		fmt.Sprintf("未知模型: %s/%s，必须通过显式 override 注册", ref.Provider, ref.ModelID),
	)
}

func modelKey(ref agentcontext.ModelRef) string {
	return ref.Provider + "/" + ref.ModelID
}

// DefaultModelFixture 返回默认的模型能力 fixture（用于测试）
func DefaultModelFixture() map[string]agentcontext.ModelCapabilities {
	return map[string]agentcontext.ModelCapabilities{
		"openai/gpt-4o": {
			ContextWindow:     128000,
			MaxOutputTokens:   4096,
			TokenizerStrategy: agentcontext.TokenizerStrategyCompatibleLocal,
			SupportsToolCalls: true,
		},
		"openai/gpt-4o-mini": {
			ContextWindow:     128000,
			MaxOutputTokens:   4096,
			TokenizerStrategy: agentcontext.TokenizerStrategyCompatibleLocal,
			SupportsToolCalls: true,
		},
		"deepseek/deepseek-chat": {
			ContextWindow:     64000,
			MaxOutputTokens:   4096,
			TokenizerStrategy: agentcontext.TokenizerStrategyCompatibleLocal,
			SupportsToolCalls: true,
		},
	}
}
