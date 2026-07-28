package agentcontext

import (
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// GovernanceAction 治理动作
type GovernanceAction string

const (
	GovernanceActionNone      GovernanceAction = "none"
	GovernanceActionTrimOld   GovernanceAction = "trim_old"
	GovernanceActionDropLow   GovernanceAction = "drop_low_priority"
	GovernanceActionSummarize GovernanceAction = "summarize"
)

// CompileGovernanceResult 治理结果
type CompileGovernanceResult struct {
	Action        GovernanceAction
	BeforeTokens  int
	AfterTokens   int
	DroppedItems  []DroppedItem
	SelectedItems []SelectedItem
	CounterMode   TokenizerStrategy
}

// DroppedItem 被淘汰的项
type DroppedItem struct {
	ID     string
	Kind   ContextKind
	Reason string
	Tokens int
}

// SelectedItem 被选中的项
type SelectedItem struct {
	ID     string
	Kind   ContextKind
	Tokens int
}

// BudgetCompiler 预算编译器。
// 负责硬保留、软分类、快速路径和水位治理。
type BudgetCompiler struct {
	counter TokenCounter
	model   ModelRef
}

// NewBudgetCompiler 创建预算编译器
func NewBudgetCompiler(counter TokenCounter, model ModelRef) *BudgetCompiler {
	return &BudgetCompiler{
		counter: counter,
		model:   model,
	}
}

// Compile 执行预算编译。
// 返回治理后的消息列表和治理结果。
func (bc *BudgetCompiler) Compile(
	profile ContextProfileSnapshot,
	instruction string,
	plan MessagePlan,
	toolInfos []*schema.ToolInfo,
) (*CompileGovernanceResult, []*schema.Message, error) {
	budget := profile.Budget

	// 计算输入预算
	inputBudget := calculateInputBudget(budget, bc.model)

	// 分组消息
	messages := buildMessagesFromPlan(instruction, plan, toolInfos)
	groups := BuildMessageGroups(messages)

	// 估算总 token 数
	totalTokens := bc.estimateGroupsTokens(groups)

	// 快速路径判断
	fastPathThreshold := int(float64(inputBudget) * budget.FastPathThreshold)
	if totalTokens < fastPathThreshold {
		return &CompileGovernanceResult{
			Action:       GovernanceActionNone,
			BeforeTokens: totalTokens,
			AfterTokens:  totalTokens,
			CounterMode:  bc.getCounterMode(),
		}, messages, nil
	}

	// 需要治理：尝试更精确计数
	preciseTokens := bc.preciseCountGroupsTokens(groups)

	fullGovernanceThreshold := int(float64(inputBudget) * budget.FullGovernanceThreshold)
	if preciseTokens < fullGovernanceThreshold {
		return &CompileGovernanceResult{
			Action:       GovernanceActionNone,
			BeforeTokens: preciseTokens,
			AfterTokens:  preciseTokens,
			CounterMode:  bc.getCounterMode(),
		}, messages, nil
	}

	// 执行完整治理
	governanceTarget := int(float64(inputBudget) * budget.GovernanceTarget)
	result, governedGroups := bc.govern(groups, governanceTarget, budget)

	// 展平为消息列表
	governedMessages := FlattenGroups(governedGroups)

	return result, governedMessages, nil
}

// calculateInputBudget 计算输入预算
func calculateInputBudget(budget BudgetConfig, model ModelRef) int {
	contextWindow := budget.ContextWindow
	if contextWindow == 0 {
		contextWindow = 128000 // 默认值
	}

	maxOutput := budget.MaxOutputTokens
	if maxOutput == 0 {
		maxOutput = 4096 // 默认值
	}

	safetyMargin := int(float64(contextWindow) * budget.SafetyMarginRatio)
	if safetyMargin < budget.SafetyMarginMin {
		safetyMargin = budget.SafetyMarginMin
	}

	return contextWindow - maxOutput - safetyMargin
}

// buildMessagesFromPlan 从 MessagePlan 构建消息列表
func buildMessagesFromPlan(instruction string, plan MessagePlan, toolInfos []*schema.ToolInfo) []*schema.Message {
	var messages []*schema.Message

	// 系统消息
	if instruction != "" {
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: instruction,
		})
	}

	// 摘要
	if plan.Summary != nil {
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: fmt.Sprintf("<conversation_summary>%s</conversation_summary>", plan.Summary.Content),
		})
	}

	// 记忆
	if len(plan.Memories) > 0 {
		var memContent string
		for _, mem := range plan.Memories {
			memContent += fmt.Sprintf("<memory id=\"%s\">%s</memory>\n", mem.ID, mem.Content)
		}
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: fmt.Sprintf("<user_memories>\n%s</user_memories>", memContent),
		})
	}

	// 历史消息
	messages = append(messages, plan.History...)

	// 当前输入
	if plan.CurrentInput != nil {
		switch input := plan.CurrentInput.(type) {
		case UserMessageInput:
			messages = append(messages, schema.UserMessage(input.Content))
		case SearchTaskInput:
			messages = append(messages, &schema.Message{
				Role:    schema.User,
				Content: fmt.Sprintf("<search_task>\nquery: %s\n</search_task>", input.Task.Query),
			})
		}
	}

	// 运行中消息必须位于当前输入之后，保持 ToolCall/ToolResult 协议顺序。
	messages = append(messages, plan.RuntimeMessages...)

	return messages
}

// estimateGroupsTokens 估算消息组的 token 数
func (bc *BudgetCompiler) estimateGroupsTokens(groups []MessageGroup) int {
	total := 0
	for _, g := range groups {
		for _, msg := range g.Messages {
			// 保守估算：content 长度 / 3 + 固定开销
			total += estimateMessageTokens(msg)
		}
	}
	return total
}

// preciseCountGroupsTokens 使用 TokenCounter 精确计算
func (bc *BudgetCompiler) preciseCountGroupsTokens(groups []MessageGroup) int {
	total := 0
	for _, g := range groups {
		count, err := CountGroupTokens(g, bc.counter, bc.model)
		if err != nil {
			// 降级到保守估算
			total += bc.estimateGroupsTokens([]MessageGroup{g})
			continue
		}
		total += count
	}
	return total
}

// estimateMessageTokens 保守估算单条消息的 token 数
func estimateMessageTokens(msg *schema.Message) int {
	// 固定开销：角色、消息结构
	tokens := 4

	// Content token 估算
	tokens += estimateTextTokens(msg.Content)

	// Tool calls 开销
	for _, tc := range msg.ToolCalls {
		tokens += 10 // tool call 结构开销
		tokens += estimateTextTokens(tc.Function.Arguments)
	}

	// Tool call ID 开销
	if msg.ToolCallID != "" {
		tokens += 10
	}

	return tokens
}

// estimateTextTokens 估算文本的 token 数（UTF-8 保守估算）
func estimateTextTokens(text string) int {
	if text == "" {
		return 0
	}

	// UTF-8 字节数 / 3 的上界（中文约 1.5-2 token/字符，英文约 0.75 token/字符）
	// 使用保守上界
	bytes := len([]byte(text))
	tokens := (bytes + 2) / 3 // 向上取整
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// getCounterMode 获取当前计数模式
func (bc *BudgetCompiler) getCounterMode() TokenizerStrategy {
	// 首期默认使用保守估算
	return TokenizerStrategyConservativeUTF8
}

// govern 执行治理
func (bc *BudgetCompiler) govern(
	groups []MessageGroup,
	targetTokens int,
	budget BudgetConfig,
) (*CompileGovernanceResult, []MessageGroup) {
	result := &CompileGovernanceResult{
		BeforeTokens: bc.estimateGroupsTokens(groups),
		CounterMode:  bc.getCounterMode(),
	}

	// 分离硬保留组和可淘汰组
	var hardReserved []MessageGroup
	var droppable []MessageGroup

	for _, g := range groups {
		if g.IsHardReserved || g.Type == MessageGroupTypeToolExchange && !g.IsClosed {
			hardReserved = append(hardReserved, g)
		} else {
			droppable = append(droppable, g)
		}
	}

	// 计算硬保留 token 数
	hardTokens := bc.estimateGroupsTokens(hardReserved)
	remainingBudget := targetTokens - hardTokens

	if remainingBudget <= 0 {
		// 硬保留已超限
		result.Action = GovernanceActionDropLow
		result.AfterTokens = hardTokens
		return result, hardReserved
	}

	// 从后往前淘汰 droppable 组（保留最近的）
	var selected []MessageGroup
	currentTokens := 0

	for i := len(droppable) - 1; i >= 0; i-- {
		groupTokens := bc.estimateGroupsTokens([]MessageGroup{droppable[i]})
		if currentTokens+groupTokens <= remainingBudget {
			selected = append([]MessageGroup{droppable[i]}, selected...)
			currentTokens += groupTokens
		} else {
			// 记录被淘汰的项
			for _, msg := range droppable[i].Messages {
				result.DroppedItems = append(result.DroppedItems, DroppedItem{
					Kind:   classifyMessage(msg),
					Reason: "budget_exceeded",
					Tokens: estimateMessageTokens(msg),
				})
			}
		}
	}

	// 合并硬保留和选中的组
	finalGroups := append(hardReserved, selected...)
	result.Action = GovernanceActionTrimOld
	result.AfterTokens = bc.estimateGroupsTokens(finalGroups)

	return result, finalGroups
}

// classifyMessage 根据消息内容分类
func classifyMessage(msg *schema.Message) ContextKind {
	if msg.Role == schema.System {
		return ContextKindConversationSummary
	}
	return ContextKindConversationSummary // 默认
}
