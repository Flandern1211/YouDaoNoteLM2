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
W5 生命周期与写回协调
         ↓
W6 Manifest 与 Shadow
         ↓
W7 灰度启用与旧路径收敛
```

MemoryProvider/MemoryWriter 的真实记忆算法不属于本项目上下文模块首期交付；W2/W5 只完成接口、调度能力和未配置时的明确行为。

### 2026-07-28 最小 Harness 实施决定

- W0–W4、W5a 和 W6a 已完成。
- 用户批准 W5b/W6b 采用单进程同步最小 Harness 完成 Chat 闭环。
- 最小 Harness 使用 MySQL 固化 ContextMode、WritebackOwner、Profile、Authority、revision 和终态，并持久化无正文 Manifest。
- Enabled 的 Assistant 写入与幂等 Journal 在同一数据库事务中完成；Legacy/Shadow 继续由旧写回路径拥有主结果。
- 本阶段不实现 Queue/Worker、Lease、Checkpoint Resume、NATS、后台 Repair Scanner、多实例接管或 Search StepResult 持久化；这些能力不得被最小 Harness 的完成状态替代。

## 3. W0：现有行为基线与 Eino 契约

**当前状态：进行中。**

已完成的只读核对：

- `go.mod` 和模块缓存确认 Eino 为 v0.9.4。
- `GenModelInput` 默认把唯一 Agent Instruction 放在初始消息之前。
- `BeforeModelRewriteState` 能读取并返回修改后的 Messages、ToolInfos、DeferredToolInfos，返回 State 会持久到后续迭代。
- `AfterAgent` 只覆盖成功终止。

尚未完成的 W0 交付：

- Legacy Chat/Main/Search 行为测试。
- Handler 注册顺序与实际调用顺序契约测试。
- 多轮 ReAct State 持久化测试。
- ToolCall/ToolResult 完整性测试。

因此实现必须从 W0 开始，不能直接把静态源码核对当成 W0 退出。

### 范围

- 固化当前 Chat/Main 的摘要、最近历史和 RAG Tool 行为。
- 固化当前 Search 的直接入口和 Main 委托入口。
- 验证 Eino v0.9.4 的 `GenModelInput`、Handler 顺序和状态持久语义。
- 确认 ToolCall/ToolResult 的合法消息结构。
- 固化当前同一 Conversation 单活动 Turn 的锁语义。

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
- 定义 ContextCompiler、TurnInput、MessagePlan 和 ContextItem。
- 定义 `TurnSession`、`PreparedTurn`、`CompileRecord` 和序列化 Snapshot。
- 定义不可变 `Registry`、ProfileKey 和构造期校验。
- 定义强类型 Provider 接口。
- 定义 `chat.v1`、`main.v1`、`search.v1`。
- 定义解析链、重试和耗尽动作。
- 为未来 W5 保留 `TurnLifecycleCoordinator` 边界，但不在 W1 伪造 Run/Harness 实现。

### 产出

- 无基础设施依赖的领域类型和接口。
- Prepare/Compile 编译契约。
- Profile 注册和输入类型校验。
- 确定性 Profile Snapshot。

### 验证

- Chat/Main 拒绝 SearchTaskInput。
- Search 拒绝 UserMessageInput。
- Search Profile 不包含 Memory 和 History。
- 同一版本 Profile 编译结果稳定。
- Registry 拒绝重复 Key，构造后不可变，新旧版本可并存。

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
- 适配实现放在 `internal/agentcontext/adapter`，并由 `internal/app` 注入。

### 产出

- Redis → MySQL History 回退链。
- `StaticPromptProvider` 保持现有 Chat/Main/Search Prompt 渲染结果。
- `LegacyHistoryProvider` 保持现有 Redis → MySQL 行为和 Provider 自有缓存回填。
- 带 `ThroughMessageID` 和版本的 HistorySnapshot 契约。
- 只读 PreparedTurn。
- Provider 状态和延迟元数据。

### 验证

- Redis 失败时 MySQL 回退。
- 必要 Prompt 或 ModelCapabilities 失败时中止。
- Memory 未配置或失败时按 Profile 降级。
- 必要链失败会取消其他仍在运行的任务。
- 执行相关 race 测试。

### 退出条件

PrepareTurn 能为三个 Profile 产生确定性的 MessagePlan，且不写回任何领域数据；Chat/Main 历史使用当前输入的排他 Sequence 边界。

## 6. W3：Token 计数与 BudgetCompiler

### 范围

- 实现输入预算公式和安全余量。
- 计入 System、Messages、ToolInfos 和 DeferredToolInfos。
- 实现 70% 精确计数阈值、80% 高水位和 60% 低水位。
- 实现 Summary 10% 和 Memory 5% 上限。
- 实现候选优先级、时间顺序恢复和硬预算错误。
- 实现 ToolCall/ToolResult 完整性检查。
- Anthropic 接近阈值时适配现有 SDK `Messages.CountTokens`。
- 已知 OpenAI 模型通过版本化注册表映射 `o200k_base` / `cl100k_base`。
- 自定义 OpenAI-compatible 模型要求显式 TokenizerStrategy。
- 实现包含消息与工具结构开销的 `conservative_utf8_bytes` 回退。

### 产出

- Provider-aware TokenCounter 适配。
- CounterMode：`exact_provider`、`compatible_local`、`conservative_utf8_bytes`。
- 快速路径。
- 分阶段治理结果和裁剪原因。
- 类型化预算错误。

### 验证

- 低水位请求不调用远程精确计数。
- 接近阈值时才精确计数。
- 用户输入本身超限时不调用模型。
- 任意治理结果保持工具消息合法。
- 相同输入和版本产生确定结果。
- 中文、特殊 Token、消息封装和工具 Schema 契约样例通过。
- OpenAI 本地兼容计数不误标为 exact。

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
- 实现版本化 PreparedTurnSnapshot codec 和 Eino 内存契约测试；持久化 checkpoint restore 由 W5 Harness 接入。
- 在当前 `chatAgentService` 旁接入 Legacy/Shadow Turn Adapter；不伪造 Run、Authority 或跨进程恢复。
- `ContextBuilder` 在 Legacy 保留，Shadow 只比较无正文 Manifest，不发起第二次模型调用。

### 产出

- Chat/Main Agent 接入路径。
- Search Agent 接入路径。
- 每轮模型调用前的动态治理。
- 运行内摘要与持久会话摘要的明确隔离。
- 可序列化、可验证的 PreparedTurnSnapshot、CompileRecord 和运行内摘要候选。
- 当前代码到 `StaticPromptProvider`、`LegacyHistoryProvider`、ContextCompiler 和 Eino Handler 的组装路径。

### 验证

- 多轮 ReAct 不重复注入记忆、摘要或当前输入。
- RAG 只在工具调用后进入状态。
- Tool Result 增长能触发治理。
- Search 无法获得 Chat/Main 历史和记忆。
- 取消和截止时间完整传播。
- Snapshot 不保存客户端、函数、锁、流、Runtime 指针或明文凭据。
- Snapshot codec 能拒绝 schema、Profile、Prompt、模型或工具契约不兼容。
- Shadow 失败不改变当前 Legacy 模型输入、响应或写回。

### 退出条件

新编译路径在单元和集成环境中可以独立完成 Chat/Main 与 Search 输入编译；当前运行时可以执行 Shadow。没有 Harness 时不得宣称具备 durable Begin/Finalize、fencing 或 Resume。

## 8. W5：生命周期与写回协调

**实施状态：W5a 协调核心已完成；W5b 按 2026-07-28 用户批准实现最小单进程 Harness。完整 Worker claim、Outbox/Repair、暂停恢复和多实例能力仍保留为后续前置条件。**

### 范围

- 接入 RunService 已持久化的 `AcceptedTurnHandle`。
- 适配 TurnVerifier，并在 Agent Worker claim 后传播 ActiveExecutionAuthority。
- 把 W4 的 PreparedTurnSnapshot codec 接入持久化 checkpoint restore；不兼容快照进入 suspended。
- 实现 `BeginTurn` 验证和当前输入排他历史边界。
- 实现 `FinalizeTurn` 与 WritebackCoordinator。
- 在 `internal/agentcontext/writeback` 定义使用方拥有的 Writer 接口。
- 适配 AssistantMessageWriter、StepResultWriter、SummaryWriter、MemoryWriter 和 ManifestWriter。
- 定义稳定幂等键、部分成功和独立重试语义。
- 主结果 Writer 原子提交结果、Finalization Journal 和 Outbox intents。
- 实现未完成 Journal/Outbox 的 Repair Scanner 接入。
- 派生/Repair Worker claim Ticket 后使用独立 FinalizationAuthority。
- 实现 `running → finalizing → completed` 成功状态流。
- 失败/取消的 Harness 终态事务原子创建 FinalizationTicket 和 Manifest intent。
- 使用继承 trace 的独立有界 finalization context。

### 产出

- Assistant → Summary/Memory → Manifest 的可测试依赖图。
- Search StepResult → Manifest 的独立依赖图。
- 成功、失败、取消和无最终回答的终态策略。
- Provider 注入内容过滤和记忆反馈循环防护。
- 写回状态与错误分类。

### 验证

- 重复 Finalize 不生成重复领域数据。
- 过期 FencingToken 不能提交主结果或创建 intent。
- Assistant 失败时不推进 Summary 或 Memory。
- Summary/Memory 任一失败不回滚 Assistant。
- Eino 非成功退出仍记录终态 Manifest。
- Chat/Main 与 Search 的 PrimaryOutput 变体不匹配时拒绝写回。
- 没有依赖无人接管的后台 goroutine。
- 主结果提交后任意故障点都能由 Journal/Outbox 修复。

### 退出条件

任意终态都能产生确定、可重试、可观测的 FinalizeResult，且 ContextCompiler 不依赖 Repository、Outbox、Lease 或 Fencing 实现；TurnLifecycleCoordinator 只依赖窄验证与 Writer 端口。

## 9. W6：Manifest、Shadow 和可观测性

**实施状态：W6a 配置、分桶、指纹和指标端口已完成；W6b 已接入 MySQL Manifest、请求级模式快照和 Enabled/Legacy 写回互斥。完整指标后端和跨进程 Repair 仍待后续 Harness。**

### 范围

- 生成无正文 ContextManifest。
- 生成带用途派生密钥的 Context HMAC。
- 接入低基数指标。
- 实现 `legacy`、`shadow`、`enabled` 模式。
- 实现按 userID 稳定采样的 Shadow。
- 在 Run 配置快照中固定 ContextMode、WritebackOwner 和契约版本。
- 复用 `pkg/config` + Viper，在 AgentConfig 下增加 ContextManagementConfig。
- 使用 SHA-256、rollout version 和 0–10000 基点稳定分桶。

### 产出

- Provider、Token、裁剪和降级观测。
- Legacy 与新装配结果的无正文对比。
- Shadow 异常隔离。
- 配置启动校验和请求级/Run 级快照边界。

### 验证

- Manifest 扫描不含用户正文和凭据。
- Shadow 不发起第二次模型调用。
- Shadow 失败不改变 Legacy 响应。
- 长 Run、Resume 和 Repair 不因全局 Flag 变化切换写回所有者。
- 相同规范化输入的 HMAC 稳定。
- 相同 userID/rollout version 分桶稳定，提高采样率保持样本单调包含。

### 退出条件

能够用数据判断新路径的历史覆盖、Token 利用率、延迟和降级情况。

## 10. W7：灰度启用与旧路径收敛

### 范围

- 从 Shadow 数据选择灰度用户。
- 按稳定用户分桶切换 `enabled`。
- 保留一键 `legacy` 回滚。
- 达到稳定条件后停止 Shadow 双装配。
- Enabled 模式确保同一 Run 只有新旧写回路径之一生效。
- 旧 ContextBuilder 的删除另行审批，不在首次启用时删除。

### 启用条件

- 相关单元、集成和 race 测试通过。
- 没有 Context Window 超限。
- Search 上下文隔离测试通过。
- 必要 Provider 失败不会调用模型。
- Shadow 的关键历史约束召回不低于 Legacy。
- Manifest 未发现正文泄露。
- 写回幂等、失败恢复和 fencing 集成测试通过。

### 回滚条件

出现以下任一情况立即切回 Legacy：

- Context Window 超限。
- Search 获得未授权历史或记忆。
- ToolCall/ToolResult 协议错误。
- 必要上下文缺失但仍调用模型。
- 新路径错误率或延迟显著恶化。
- Assistant 已生成但无法可靠提交，或派生写回任务持续丢失。

### 退出条件

新路径稳定运行，Shadow 已关闭，Legacy 只作为短期回滚路径保留。是否删除 Legacy 由独立设计和用户批准决定。

## 11. 跨工作流约束

- 不新增第三方依赖，除非现有 Eino 和标准库无法合理满足需求。
- 修改 Go 文件后运行 `gofmt`。
- Context 和 goroutine 必须有明确退出条件。
- 不使用后台 Context 绕过已有取消或超时。
- 不为通过测试而删除或弱化目标行为测试。
- 每个工作流只修改直接相关文件，不顺带重构无关代码。
- 任何实现开始前都必须基于本设计生成并批准代码级实施计划。
