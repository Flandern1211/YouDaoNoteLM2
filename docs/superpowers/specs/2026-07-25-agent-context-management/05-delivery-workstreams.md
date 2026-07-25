# 模块化交付工作流

## 1. 说明

本文把已批准设计拆成可独立验证的交付边界。它定义模块顺序、依赖、产出和退出条件，不展开到逐文件代码步骤；用户审阅整套设计后，再由实施计划细化到具体文件和测试命令。

## 2. 依赖关系

```text
W0 基线与 Eino 契约
  ↓
W1 核心契约与 Profile
  ├──────────────┐
  ↓              ↓
W2 Provider 适配  W3 Token 与预算核心
  └──────┬───────┘
         ↓
W4 Eino Adapter 与 Middleware
         ↓
W5 Manifest 与 Shadow
         ↓
W6 灰度启用与旧路径收敛
```

MemoryProvider 的真实记忆实现不属于本项目上下文模块首期交付；W2 只完成接口接入能力和未配置时的明确行为。

## 3. W0：现有行为基线与 Eino 契约

### 范围

- 固化当前 Chat/Main 的摘要、最近历史和 RAG Tool 行为。
- 固化当前 Search 的直接入口和 Main 委托入口。
- 验证 Eino v0.9.4 的 `GenModelInput`、Handler 顺序和状态持久语义。
- 确认 ToolCall/ToolResult 的合法消息结构。

### 产出

- Legacy 行为测试。
- Eino Middleware 契约测试。
- 当前 Prompt、工具集和 Agent 定义版本标识。

### 验证

- 当前相关包测试通过。
- 流式与非流式 Search 行为都有基线。
- Middleware 修改后的 Messages 和 ToolInfos 会进入后续 ReAct 轮次。

### 退出条件

没有未记录的 Legacy 行为依赖；新模块可以通过测试判断是否破坏现有语义。

## 4. W1：核心契约与 ContextProfile

### 范围

- 建立 `internal/agentcontext`。
- 定义 Manager、TurnInput、TurnContext、MessagePlan 和 ContextItem。
- 定义强类型 Provider 接口。
- 定义 `chat.v1`、`main.v1`、`search.v1`。
- 定义解析链、重试和耗尽动作。

### 产出

- 无基础设施依赖的领域类型和接口。
- Profile 注册和输入类型校验。
- 确定性 Profile Snapshot。

### 验证

- Chat/Main 拒绝 SearchTaskInput。
- Search 拒绝 UserMessageInput。
- Search Profile 不包含 Memory 和 History。
- 同一版本 Profile 编译结果稳定。

### 退出条件

核心包不依赖 Redis、MySQL、具体记忆实现或 Eino Agent Builder。

## 5. W2：Provider 适配与并行解析

### 范围

- PromptProvider 适配现有 Prompt。
- HistoryProvider 适配 ChatCache 和 MessageRepository。
- ModelCapabilitiesResolver 实现覆盖配置和内置注册表。
- TokenCounter 建立精确/本地/估算解析链。
- MemoryProvider 保留注入点和明确的 disabled/skip 行为。
- 实现按依赖阶段并行的 PrepareTurn。

### 产出

- Redis → MySQL History 回退链。
- 带 `ThroughMessageID` 和版本的 HistorySnapshot 契约。
- 只读 TurnContext。
- Provider 状态和延迟元数据。

### 验证

- Redis 失败时 MySQL 回退。
- 必要 Prompt 或 ModelCapabilities 失败时中止。
- Memory 未配置或失败时按 Profile 降级。
- 必要链失败会取消其他仍在运行的任务。
- 执行相关 race 测试。

### 退出条件

PrepareTurn 能为三个 Profile 产生确定性的 MessagePlan，且不写回任何领域数据。

## 6. W3：Token 计数与 BudgetCompiler

### 范围

- 实现输入预算公式和安全余量。
- 计入 System、Messages、ToolInfos 和 DeferredToolInfos。
- 实现 70% 精确计数阈值、80% 高水位和 60% 低水位。
- 实现 Summary 10% 和 Memory 5% 上限。
- 实现候选优先级、时间顺序恢复和硬预算错误。
- 实现 ToolCall/ToolResult 完整性检查。

### 产出

- Provider-aware TokenCounter 适配。
- 快速路径。
- 分阶段治理结果和裁剪原因。
- 类型化预算错误。

### 验证

- 低水位请求不调用远程精确计数。
- 接近阈值时才精确计数。
- 用户输入本身超限时不调用模型。
- 任意治理结果保持工具消息合法。
- 相同输入和版本产生确定结果。

### 退出条件

测试模型矩阵中不会生成超过已知模型窗口的请求。

## 7. W4：Eino Adapter 与 Middleware

### 范围

- MessagePlan 渲染为初始 Eino Messages。
- Agent Instruction 只注入一次。
- 为三个 Agent Profile 接入接口式 Middleware。
- 在 `BeforeModelRewriteState` 调用 BudgetCompiler。
- 接入或适配 Eino ToolReduction 和 Summarization。
- 保证上下文注入幂等。

### 产出

- Chat/Main Agent 接入路径。
- Search Agent 接入路径。
- 每轮模型调用前的动态治理。
- 运行内摘要与持久会话摘要的明确隔离。

### 验证

- 多轮 ReAct 不重复注入记忆、摘要或当前输入。
- RAG 只在工具调用后进入状态。
- Tool Result 增长能触发治理。
- Search 无法获得 Chat/Main 历史和记忆。
- 取消和截止时间完整传播。

### 退出条件

新路径在单元和集成环境中可以独立完成 Chat/Main 与 Search 请求。

## 8. W5：Manifest、Shadow 和可观测性

### 范围

- 生成无正文 ContextManifest。
- 生成带用途派生密钥的 Context HMAC。
- 接入低基数指标。
- 实现 `legacy`、`shadow`、`enabled` 模式。
- 实现按 userID 稳定采样的 Shadow。

### 产出

- Provider、Token、裁剪和降级观测。
- Legacy 与新装配结果的无正文对比。
- Shadow 异常隔离。

### 验证

- Manifest 扫描不含用户正文和凭据。
- Shadow 不发起第二次模型调用。
- Shadow 失败不改变 Legacy 响应。
- 相同规范化输入的 HMAC 稳定。

### 退出条件

能够用数据判断新路径的历史覆盖、Token 利用率、延迟和降级情况。

## 9. W6：灰度启用与旧路径收敛

### 范围

- 从 Shadow 数据选择灰度用户。
- 按稳定用户分桶切换 `enabled`。
- 保留一键 `legacy` 回滚。
- 达到稳定条件后停止 Shadow 双装配。
- 旧 ContextBuilder 的删除另行审批，不在首次启用时删除。

### 启用条件

- 相关单元、集成和 race 测试通过。
- 没有 Context Window 超限。
- Search 上下文隔离测试通过。
- 必要 Provider 失败不会调用模型。
- Shadow 的关键历史约束召回不低于 Legacy。
- Manifest 未发现正文泄露。

### 回滚条件

出现以下任一情况立即切回 Legacy：

- Context Window 超限。
- Search 获得未授权历史或记忆。
- ToolCall/ToolResult 协议错误。
- 必要上下文缺失但仍调用模型。
- 新路径错误率或延迟显著恶化。

### 退出条件

新路径稳定运行，Shadow 已关闭，Legacy 只作为短期回滚路径保留。是否删除 Legacy 由独立设计和用户批准决定。

## 10. 跨工作流约束

- 不新增第三方依赖，除非现有 Eino 和标准库无法合理满足需求。
- 修改 Go 文件后运行 `gofmt`。
- Context 和 goroutine 必须有明确退出条件。
- 不使用后台 Context 绕过已有取消或超时。
- 不为通过测试而删除或弱化目标行为测试。
- 每个工作流只修改直接相关文件，不顺带重构无关代码。
- 任何实现开始前都必须基于本设计生成并批准代码级实施计划。
