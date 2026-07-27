package agentcontext

// 预定义的 Profile Keys
var (
	ChatV1   = ProfileKey{Name: "chat", Version: "v1"}
	MainV1   = ProfileKey{Name: "main", Version: "v1"}
	SearchV1 = ProfileKey{Name: "search", Version: "v1"}
)

// NewChatV1Profile 创建 chat.v1 Profile
func NewChatV1Profile() ContextProfile {
	return ContextProfile{
		Key:         ChatV1,
		AgentID:     AgentIDChat,
		Description: "Chat Agent 上下文配置",
		AllowedSources: []ContextKind{
			ContextKindConversationSummary,
			ContextKindUserMemory,
		},
		WritebackPolicy: WritebackPolicyConversationTurn,
		Budget:          DefaultBudgetConfig(),
		LoadMemory:      true,
		LoadHistory:     true,
		LoadSummary:     true,
	}
}

// NewMainV1Profile 创建 main.v1 Profile
func NewMainV1Profile() ContextProfile {
	return ContextProfile{
		Key:         MainV1,
		AgentID:     AgentIDMain,
		Description: "Main Agent 上下文配置",
		AllowedSources: []ContextKind{
			ContextKindConversationSummary,
			ContextKindUserMemory,
		},
		WritebackPolicy: WritebackPolicyConversationTurn,
		Budget:          DefaultBudgetConfig(),
		LoadMemory:      true,
		LoadHistory:     true,
		LoadSummary:     true,
	}
}

// NewSearchV1Profile 创建 search.v1 Profile
func NewSearchV1Profile() ContextProfile {
	return ContextProfile{
		Key:             SearchV1,
		AgentID:         AgentIDSearch,
		Description:     "Search Agent 上下文配置",
		AllowedSources:  []ContextKind{},
		WritebackPolicy: WritebackPolicyStepResult,
		Budget:          DefaultBudgetConfig(),
		LoadMemory:      false,
		LoadHistory:     false,
		LoadSummary:     false,
	}
}
