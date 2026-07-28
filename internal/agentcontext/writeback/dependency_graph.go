package writeback

import (
	"fmt"

	"YoudaoNoteLm/internal/agentcontext"
)

// WritebackGraph 写回依赖图。
// 定义主结果与派生 Writer 之间的依赖关系和执行顺序。
type WritebackGraph struct {
	nodes []WritebackNode
}

// WritebackNode 写回节点
type WritebackNode struct {
	Operation  WritebackOperation
	DependsOn  []WritebackOperation // 依赖的操作（完成后才能执行）
	Required   bool                 // 是否必需（失败则中止）
	MaxRetries int                  // 最大重试次数
}

// NewManifestOnlyGraph 创建只记录终态 Manifest 的依赖图。
// Shadow 或 Legacy 写回所有者使用该图，避免重复提交主结果。
func NewManifestOnlyGraph() *WritebackGraph {
	return &WritebackGraph{
		nodes: []WritebackNode{{
			Operation: WritebackOperationManifest,
			Required:  true,
		}},
	}
}

// NewWritebackGraph 创建写回依赖图。
// 根据 Profile 的 WritebackPolicy 和终态决定图结构。
func NewWritebackGraph(
	policy agentcontext.WritebackPolicy,
	status agentcontext.TurnStatus,
) *WritebackGraph {
	g := &WritebackGraph{}

	switch policy {
	case agentcontext.WritebackPolicyConversationTurn:
		g.buildConversationTurnGraph(status)
	case agentcontext.WritebackPolicyStepResult:
		g.buildStepResultGraph(status)
	}

	return g
}

// buildConversationTurnGraph 构建 Chat/Main 的写回图。
func (g *WritebackGraph) buildConversationTurnGraph(status agentcontext.TurnStatus) {
	switch status {
	case agentcontext.TurnStatusSuccess:
		// 成功路径：Assistant → (Summary, Memory, Manifest)
		g.nodes = append(g.nodes, WritebackNode{
			Operation:  WritebackOperationAssistant,
			DependsOn:  nil,
			Required:   true,
			MaxRetries: 2,
		})
		g.nodes = append(g.nodes, WritebackNode{
			Operation:  WritebackOperationSummary,
			DependsOn:  []WritebackOperation{WritebackOperationAssistant},
			Required:   false,
			MaxRetries: 1,
		})
		g.nodes = append(g.nodes, WritebackNode{
			Operation:  WritebackOperationMemory,
			DependsOn:  []WritebackOperation{WritebackOperationAssistant},
			Required:   false,
			MaxRetries: 1,
		})
		g.nodes = append(g.nodes, WritebackNode{
			Operation:  WritebackOperationManifest,
			DependsOn:  []WritebackOperation{WritebackOperationAssistant},
			Required:   true,
			MaxRetries: 1,
		})

	default:
		// 失败/取消路径：只写 Manifest
		g.nodes = append(g.nodes, WritebackNode{
			Operation:  WritebackOperationManifest,
			DependsOn:  nil,
			Required:   true,
			MaxRetries: 1,
		})
	}
}

// buildStepResultGraph 构建 Search 的写回图。
func (g *WritebackGraph) buildStepResultGraph(status agentcontext.TurnStatus) {
	switch status {
	case agentcontext.TurnStatusSuccess:
		// 成功路径：StepResult → Manifest
		g.nodes = append(g.nodes, WritebackNode{
			Operation:  WritebackOperationStepResult,
			DependsOn:  nil,
			Required:   true,
			MaxRetries: 2,
		})
		g.nodes = append(g.nodes, WritebackNode{
			Operation:  WritebackOperationManifest,
			DependsOn:  []WritebackOperation{WritebackOperationStepResult},
			Required:   true,
			MaxRetries: 1,
		})

	default:
		// 失败/取消路径：只写 Manifest
		g.nodes = append(g.nodes, WritebackNode{
			Operation:  WritebackOperationManifest,
			DependsOn:  nil,
			Required:   true,
			MaxRetries: 1,
		})
	}
}

// ExecutionPlan 执行计划
type ExecutionPlan struct {
	// Stages 按依赖顺序排列的执行阶段
	// 同一阶段内的操作可以并行执行
	Stages [][]WritebackOperation
	// Nodes 所有节点的映射
	Nodes map[WritebackOperation]WritebackNode
}

// Plan 生成执行计划。
// 使用拓扑排序，将依赖关系转换为可并行执行的阶段。
func (g *WritebackGraph) Plan() (*ExecutionPlan, error) {
	if len(g.nodes) == 0 {
		return &ExecutionPlan{
			Stages: nil,
			Nodes:  make(map[WritebackOperation]WritebackNode),
		}, nil
	}

	// 构建节点映射
	nodeMap := make(map[WritebackOperation]WritebackNode, len(g.nodes))
	for _, n := range g.nodes {
		nodeMap[n.Operation] = n
	}

	// 拓扑排序
	sorted, err := g.topologicalSort()
	if err != nil {
		return nil, err
	}

	// 分阶段：无依赖的先执行，然后逐层推进
	stageOf := make(map[WritebackOperation]int)
	for _, op := range sorted {
		node := nodeMap[op]
		stage := 0
		for _, dep := range node.DependsOn {
			if s, ok := stageOf[dep]; ok && s+1 > stage {
				stage = s + 1
			}
		}
		stageOf[op] = stage
	}

	// 找出最大阶段数
	maxStage := 0
	for _, s := range stageOf {
		if s > maxStage {
			maxStage = s
		}
	}

	// 构建阶段列表
	stages := make([][]WritebackOperation, maxStage+1)
	for op, stage := range stageOf {
		stages[stage] = append(stages[stage], op)
	}

	return &ExecutionPlan{
		Stages: stages,
		Nodes:  nodeMap,
	}, nil
}

// topologicalSort 拓扑排序
func (g *WritebackGraph) topologicalSort() ([]WritebackOperation, error) {
	// 构建入度表
	inDegree := make(map[WritebackOperation]int)
	dependents := make(map[WritebackOperation][]WritebackOperation)

	for _, n := range g.nodes {
		if _, ok := inDegree[n.Operation]; !ok {
			inDegree[n.Operation] = 0
		}
		for _, dep := range n.DependsOn {
			inDegree[n.Operation]++
			dependents[dep] = append(dependents[dep], n.Operation)
		}
	}

	// BFS 拓扑排序
	var queue []WritebackOperation
	for op, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, op)
		}
	}

	var sorted []WritebackOperation
	for len(queue) > 0 {
		op := queue[0]
		queue = queue[1:]
		sorted = append(sorted, op)

		for _, dep := range dependents[op] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(sorted) != len(g.nodes) {
		return nil, fmt.Errorf("写回依赖图存在循环依赖")
	}

	return sorted, nil
}

// GetRequiredOperations 获取必需的操作列表
func (g *WritebackGraph) GetRequiredOperations() []WritebackOperation {
	var required []WritebackOperation
	for _, n := range g.nodes {
		if n.Required {
			required = append(required, n.Operation)
		}
	}
	return required
}

// CanSkip 检查操作是否可以跳过
func (g *WritebackGraph) CanSkip(op WritebackOperation) bool {
	for _, n := range g.nodes {
		if n.Operation == op {
			return !n.Required
		}
	}
	return true // 未知操作默认可跳过
}

// GetMaxRetries 获取操作的最大重试次数
func (g *WritebackGraph) GetMaxRetries(op WritebackOperation) int {
	for _, n := range g.nodes {
		if n.Operation == op {
			return n.MaxRetries
		}
	}
	return 0
}
