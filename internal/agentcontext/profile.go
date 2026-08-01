package agentcontext

// ContextProfile 定义 Agent 的上下文装配策略
type ContextProfile struct {
	Key             ProfileKey
	AgentID         AgentID
	Description     string
	AllowedSources  []ContextKind
	WritebackPolicy WritebackPolicy
	Budget          BudgetConfig
	// 是否加载用户记忆
	LoadMemory bool
	// 是否加载会话历史
	LoadHistory bool
	// 是否加载会话摘要
	LoadSummary bool
}

// Validate 验证 Profile 配置
func (p *ContextProfile) Validate() error {
	if p.Key.Name == "" {
		return NewError(ErrCodeInvalidConfig, "profile name is required")
	}
	if p.Key.Version == "" {
		return NewError(ErrCodeInvalidConfig, "profile version is required")
	}
	if p.AgentID == "" {
		return NewError(ErrCodeInvalidConfig, "agent ID is required")
	}
	return nil
}

// ToSnapshot 将 Profile 转换为快照
func (p *ContextProfile) ToSnapshot() ContextProfileSnapshot {
	return ContextProfileSnapshot{
		Key:             p.Key,
		AgentID:         p.AgentID,
		AllowedSources:  p.AllowedSources,
		WritebackPolicy: p.WritebackPolicy,
		Budget:          p.Budget,
	}
}
