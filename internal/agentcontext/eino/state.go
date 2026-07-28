// Package eino 将 ContextCompiler 接入 Eino ChatModelAgent。
// 通过 ChatModelAgentMiddleware 在每次模型调用前执行上下文编译。
package eino

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// contextCompilerKey 是 PreparedTurn 在 Eino AgentContext 中的私有强类型 key。
// 使用包内私有类型防止与其他 Handler 冲突。
type contextCompilerKey struct{}

// shadowRecordKey 是 Shadow 比较记录在 context 中的私有强类型 key。
type shadowRecordKey struct{}

// GetToolInfos 从 state 中提取工具定义。
func GetToolInfos(state *adk.ChatModelAgentState) []*schema.ToolInfo {
	if state == nil {
		return nil
	}
	return state.ToolInfos
}

// GetMessages 从 state 中提取消息列表。
func GetMessages(state *adk.ChatModelAgentState) []*schema.Message {
	if state == nil {
		return nil
	}
	return state.Messages
}
