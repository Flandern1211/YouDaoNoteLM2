package chat

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"

	"YoudaoNoteLm/internal/agent/chat/prompts"
)

func TestChatAgentBuilder_BuildSystemPrompt_NoSources(t *testing.T) {
	// 测试场景：无知识库时的系统提示词
	builder := &ChatAgentBuilder{
		ctx:         context.Background(),
		sourceIDs:   []uint{},
		sourceNames: map[uint]string{},
	}

	prompt := builder.buildSystemPrompt()

	// 验证关键段存在
	assert.Contains(t, prompt, "智能知识问答助手")
	assert.Contains(t, prompt, "用户未选定特定资料")
	assert.Contains(t, prompt, "search_knowledge")
	assert.Contains(t, prompt, "get_sources_summary")

	// 验证不包含源列表占位符
	assert.NotContains(t, prompt, "{{.SourceList}}")
}

func TestChatAgentBuilder_BuildSystemPrompt_WithSources(t *testing.T) {
	// 测试场景：有知识库时的系统提示词
	builder := &ChatAgentBuilder{
		ctx:       context.Background(),
		sourceIDs: []uint{1, 2, 3},
		sourceNames: map[uint]string{
			1: "资料A",
			2: "资料B",
			3: "资料C",
		},
	}

	prompt := builder.buildSystemPrompt()

	// 验证关键段存在
	assert.Contains(t, prompt, "智能知识问答助手")
	assert.Contains(t, prompt, "search_knowledge")
	assert.Contains(t, prompt, "get_sources_summary")

	// 验证源列表被正确渲染
	assert.Contains(t, prompt, "资料A")
	assert.Contains(t, prompt, "资料B")
	assert.Contains(t, prompt, "资料C")
	assert.Contains(t, prompt, "ID: 1")
	assert.Contains(t, prompt, "ID: 2")
	assert.Contains(t, prompt, "ID: 3")

	// 验证源列表的稳定顺序（按 sourceIDs 顺序）
	assert.Contains(t, prompt, "1. 资料A (ID: 1)")
	assert.Contains(t, prompt, "2. 资料B (ID: 2)")
	assert.Contains(t, prompt, "3. 资料C (ID: 3)")
}

func TestChatAgentBuilder_BuildSystemPrompt_SourceWithoutName(t *testing.T) {
	// 测试场景：源没有名称时使用默认名称
	builder := &ChatAgentBuilder{
		ctx:         context.Background(),
		sourceIDs:   []uint{5},
		sourceNames: map[uint]string{},
	}

	prompt := builder.buildSystemPrompt()

	// 验证使用默认名称
	assert.Contains(t, prompt, "资料#5")
}

func TestChatAgentBuilder_BuildSystemPrompt_TemplateIntegrity(t *testing.T) {
	// 测试场景：确保模板渲染不会破坏其他部分
	builder := &ChatAgentBuilder{
		ctx:       context.Background(),
		sourceIDs: []uint{1},
		sourceNames: map[uint]string{
			1: "测试资料",
		},
	}

	prompt := builder.buildSystemPrompt()

	// 验证模板的其他关键部分保持完整
	assert.Contains(t, prompt, "# 角色")
	assert.Contains(t, prompt, "# 能力")
	assert.Contains(t, prompt, "# 工具选择指南")
	assert.Contains(t, prompt, "# 工作流程")
	assert.Contains(t, prompt, "# 回答规范")
	assert.Contains(t, prompt, "# 无资料模式")
	assert.Contains(t, prompt, "# 注意")

	// 验证工作流描述保持完整
	assert.Contains(t, prompt, "两步工作流")
	assert.Contains(t, prompt, "第一步：调用 get_sources_summary")
	assert.Contains(t, prompt, "第二步：调用 search_knowledge")
}

func TestChatAgentBuilder_BuildSystemPrompt_ConsistentWithConstant(t *testing.T) {
	// 测试场景：确保渲染结果与原始常量模板一致
	builder := &ChatAgentBuilder{
		ctx:         context.Background(),
		sourceIDs:   []uint{},
		sourceNames: map[uint]string{},
	}

	prompt := builder.buildSystemPrompt()

	// 验证渲染结果包含原始模板的所有关键部分
	// 这不是全文快照测试，只验证关键段
	assert.Contains(t, prompt, "智能知识问答助手")
	assert.Contains(t, prompt, "search_knowledge")
	assert.Contains(t, prompt, "get_sources_summary")
	assert.Contains(t, prompt, "两步工作流")
	assert.Contains(t, prompt, "引用标注")
}

func TestChatAgentBuilder_Build_DefaultTools(t *testing.T) {
	// 测试场景：验证默认工具集合
	// 注意：这个测试需要 mock LLM 和其他依赖，这里只验证工具构建逻辑
	builder := NewChatAgentBuilder(context.Background())

	// 验证初始状态
	assert.Empty(t, builder.tools)
	assert.NotNil(t, builder.sourceNames)
	assert.Equal(t, 10, builder.maxIterations)

	// 验证 prompts 常量存在
	_ = prompts.ChatAgentSystemPrompt
}

func TestChatAgentBuilder_WithTool(t *testing.T) {
	// 测试场景：验证工具添加
	builder := NewChatAgentBuilder(context.Background())

	// 添加一个 mock 工具
	mockTool := &mockBaseTool{name: "test_tool"}
	builder.WithTool(mockTool)

	assert.Len(t, builder.tools, 1)
}

func TestChatAgentBuilder_WithTools(t *testing.T) {
	// 测试场景：验证批量工具添加
	builder := NewChatAgentBuilder(context.Background())

	// 添加多个 mock 工具
	mockTool1 := &mockBaseTool{name: "tool1"}
	mockTool2 := &mockBaseTool{name: "tool2"}
	builder.WithTools(mockTool1, mockTool2)

	assert.Len(t, builder.tools, 2)
}

// mockBaseTool 用于测试的工具 mock
type mockBaseTool struct {
	name string
}

func (m *mockBaseTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

func (m *mockBaseTool) InvokableRun(_ context.Context, _ string) (string, error) {
	return "mock result", nil
}
