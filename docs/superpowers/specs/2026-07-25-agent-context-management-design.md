# Agent 上下文管理模块设计

## 1. 文档状态

- 状态：已批准
- 日期：2026-07-25
- 首期范围：Chat Agent、Main Agent、Search Agent
- 方案：薄 `ContextManager` + Eino `ChatModelAgentMiddleware`

本设计细化 `2026-07-16-agent-harness-design.md` 中的上下文管理部分，不替代其中的 Run、Checkpoint、Lease、NATS 或持久化 Harness 设计。首期只建立可替换的上下文能力接口、按 Agent 隔离的装配策略、Token 预算和 Eino 接入点。

## 2. 目标

- 使用依赖倒置定义 Prompt、用户记忆、会话历史、模型能力和 Token 计数接口。
- 允许同一上下文能力的不同实现通过应用组装层替换。
- 为 Chat、Main 和 Search Agent 提供隔离的 `ContextProfile`。
- 请求开始时生成只读的本轮上下文快照。
- 每次模型调用前治理动态增长的工具结果和消息历史。
- 在不破坏 Eino ToolCall/ToolResult 协议的前提下控制上下文窗口。
- 默认只记录无正文的上下文 Manifest。
- 通过 `legacy`、`shadow`、`enabled` 三种模式渐进迁移。

## 3. 非目标

- 不实现用户记忆的写入、提取、冲突合并或存储。
- 不实现会话消息和摘要的持久化写回。
- 不把 RAG 改造成自动预检索的 Context Provider。
- 不接管 Eino 的 ReAct 循环、工具执行和事件流。
- 不在首期迁移 Youdao Agent 和 Generation Agent。
- 不引入运行时 YAML/数据库 Profile 编辑器。
- 不保存完整 Prompt 或候选上下文正文。

## 4. 核心原则

### 4.1 一个引擎，多个隔离 Profile

共享的是 Provider 解析、失败治理、Token 预算和 Manifest 机制，不共享实际上下文内容。

- `chat.v1`：Chat Prompt、用户记忆、会话摘要、最近历史、当前输入。
- `main.v1`：Main Prompt、用户记忆、会话摘要、最近历史、当前输入。
- `search.v1`：Search Prompt、结构化 SearchTask；不加载用户记忆、摘要或会话历史。

### 4.2 双上下文平面

- 运行时上下文：Go `context.Context` 中的取消、截止时间和 Trace 传播；业务 ID 通过强类型请求传递。
- 模型可见上下文：系统提示词、历史、记忆、当前任务、工具定义和工具结果。

运行时数据不会因为存在于 `context.Context` 中而自动进入模型消息。

### 4.3 ContextManager 只读

ContextManager 读取各能力的结果并完成装配，不负责消息、摘要、记忆和工具结果的写回。缓存和一致性由对应数据所有者管理。

### 4.4 RAG 保持工具驱动

用户选择的 `sourceIDs` 限定检索范围。Agent 判断是否需要调用 RAG；RAG 结果作为动态 Tool Result 进入下一次模型调用，不在每轮开始时自动检索。

### 4.5 信任等级不随消息拼接提升

只有系统提示词属于最高可信指令。用户记忆、历史、RAG 和工具结果只能提供数据、偏好或证据，不能覆盖系统规则。

## 5. 总体架构

```text
能力接口层
  PromptProvider
  MemoryProvider
  HistoryProvider
  ModelCapabilitiesResolver
  TokenCounter
          |
          v
核心编排层
  Manager
  ContextProfileRegistry
  ResolutionPolicy
  BudgetCompiler
  ManifestBuilder
          |
          v
Eino 适配层
  AgentInputBuilder
  ContextMiddleware
  Tool-message integrity checks
          |
          v
Eino ChatModelAgent / ReAct
```

请求开始时，Manager 验证 Profile 和输入类型，按依赖阶段并行解析 Provider，形成只读 `TurnContext`。Eino Adapter 把 `TurnContext.MessagePlan` 转为初始消息，Eino `GenModelInput` 注入唯一系统 Instruction。

每次模型调用前，`ContextMiddleware.BeforeModelRewriteState` 读取 `state.Messages`、`state.ToolInfos` 和 `state.DeferredToolInfos`，调用 `BudgetCompiler`。Compiler 仅在达到阈值时治理上下文，并把合法的新状态交回 Eino。

## 6. 模块文档

- [讨论决策记录](2026-07-25-agent-context-management/00-decision-log.md)
- [核心契约与数据模型](2026-07-25-agent-context-management/01-core-contracts.md)
- [Provider、Profile 与消息装配](2026-07-25-agent-context-management/02-providers-profiles.md)
- [Token 预算与 Eino 集成](2026-07-25-agent-context-management/03-budget-eino-integration.md)
- [可观测性、迁移和测试](2026-07-25-agent-context-management/04-observability-migration-testing.md)
- [模块化交付工作流](2026-07-25-agent-context-management/05-delivery-workstreams.md)

## 7. 已确认的关键决策

| 主题 | 决策 |
|---|---|
| 整体方案 | 薄 ContextManager + Eino Middleware |
| Provider 接口 | 按能力定义强类型接口 |
| 内部候选 | 可裁剪数据标准化为 `ContextItem` |
| RAG | 仅作为按需工具 |
| 用户记忆 | Provider 负责召回排名，ContextManager 负责最终注入 |
| 子 Agent 记忆 | Main 最小披露，Search 不直接读取记忆库 |
| 会话历史 | 带游标摘要 + 最近完整消息组 + Token 窗口 |
| 写回 | ContextManager 不负责 |
| 预算 | 硬保留 + 分类上限 + 优先级填充 + 高低水位 |
| 计数 | 快速近似；接近阈值时精确计数 |
| 错误策略 | 每阶段重试，阶段间回退，耗尽后 Abort 或 Skip |
| 并发 | 按依赖阶段并行；回退链内部串行 |
| 缓存 | 数据缓存归 Provider；Manager 只缓存版本化纯计算结果 |
| 可观测 | 默认无正文 Manifest；受控诊断显式开启 |
| 迁移 | `legacy` → `shadow` → `enabled` |

## 8. 验收定义

- Chat/Main 能加载正确的 Prompt、记忆、摘要、历史和当前输入。
- Search 只接收 Search Prompt、SearchTask、工具定义和动态工具结果。
- Search 不读取 Main 的完整输入、历史或记忆。
- Redis 历史失败能按策略回退 MySQL。
- 可选 MemoryProvider 失败不会阻断 Agent，且 Manifest 标记降级。
- 必要 Provider 耗尽会在模型调用前中止。
- 用户输入本身超限时返回明确错误，不自动改写。
- 任意裁剪后 ToolCall/ToolResult 序列仍合法。
- 每次模型调用均包含工具 Schema Token 和输出预留。
- 快速路径不发起额外远程精确计数或摘要调用。
- Manifest 默认不包含用户输入、记忆、历史或工具结果正文。
- Shadow 模式失败不影响 Legacy 正式响应。
- 相关单元测试、集成测试和 race 检测通过后才可启用 `enabled`。
