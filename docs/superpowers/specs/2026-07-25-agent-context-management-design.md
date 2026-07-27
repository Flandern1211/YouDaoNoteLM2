# Agent 上下文管理模块设计

## 1. 文档状态

- 状态：已批准；2026-07-27 根据现有代码复审补充落地边界
- 日期：2026-07-25
- 首期范围：Chat Agent、Main Agent、Search Agent
- 方案：轻量 ContextCompiler + 可选 Harness 生命周期协调器 + Eino `ChatModelAgentMiddleware`

本设计细化 `2026-07-16-agent-harness-design.md` 中的上下文管理部分，不替代其中的 Run、Checkpoint、Lease、NATS 或持久化 Harness 设计。该 Harness 文档当前仍是待审设计，仓库中也不存在 RunService、Worker 或持久化 Harness；因此本文把可立即落地的上下文编译能力与依赖未来 Harness 的生命周期协调明确拆开。

首期先建立可替换的 Provider、按 Agent 隔离的装配策略、Token 预算和 Eino 接入点。执行后 Writer 协调属于后续 Harness 集成边界，只有 Harness 设计获批且具备持久化 Run/Authority/Outbox 后才能进入生产实现。

## 2. 目标

- 使用依赖倒置定义 Prompt、用户记忆、会话历史、模型能力和 Token 计数接口。
- 允许同一上下文能力的不同实现通过应用组装层替换。
- 为 Chat、Main 和 Search Agent 提供隔离的 `ContextProfile`。
- 请求开始时生成只读的本轮上下文快照。
- 每次模型调用前治理动态增长的工具结果和消息历史。
- 模型执行完成后，按确定的依赖关系调度助手消息、持久化摘要、长期记忆和 Manifest 写回。
- 让各领域 Writer 只理解自己的数据模型，不要求外层 Service 重复理解完整上下文结构。
- 在不破坏 Eino ToolCall/ToolResult 协议的前提下控制上下文窗口。
- 默认只记录无正文的上下文 Manifest。
- 通过 `legacy`、`shadow`、`enabled` 三种模式渐进迁移。

## 3. 非目标

- 不负责首次接受用户命令、创建 Run 或持久化入口用户消息。
- 不在 ContextCompiler 或 TurnLifecycleCoordinator 内实现摘要生成、记忆提取、冲突合并、Repository 或缓存一致性；这些能力由独立 Provider/Writer 实现。
- 不把 `FinalizeTurn` 变成 Run、Outbox、Attempt、Lease、Fencing 或 Checkpoint 的所有者。
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

### 4.3 编译核心与生命周期协调分离

目标态对外可以提供统一门面，但内部必须拆成两个可独立实现的端口：

- `ContextCompiler`：`PrepareTurn` / `CompileModelInput`，读取稳定候选，并在每次模型调用前生成受预算约束的模型输入；W0–W4 可以基于当前 `chatAgentService` 和 Eino 独立落地。
- `TurnLifecycleCoordinator`：`BeginTurn` / `FinalizeTurn`，验证持久化 Turn Handle、执行权并调度 Writer；W5–W7 依赖未来 RunService/Harness，当前代码不能伪造这些保证。

二者都不实现消息、摘要、记忆、Manifest 的领域规则或存储一致性。当前 Legacy 路径继续由 `chatAgentService` 接受用户消息和保存结果；只有进入 Harness 迁移阶段后，生命周期协调器才接管这部分协调。

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
上下文编译核心（W1-W4）
  ContextCompiler
  TurnPreparer
  Registry（不可变代码级 Profile）
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

未来 Harness 集成（W5-W7）
  TurnLifecycleCoordinator
  WritebackCoordinator
  AssistantMessageWriter / StepResultWriter
  SummaryWriter / MemoryWriter / ManifestWriter
```

当前实现由 `chatAgentService` 接受请求、保存用户消息、创建 ChatAgent、调用 Eino 并保存结果；`chat.ContextBuilder` 直接读取摘要和历史。W0–W4 在这条路径旁建立新编译核心，并先以 Shadow 验证。

未来 RunService 先取得 Conversation 执行权，再以自己的事务接受用户命令，创建 Run、入口用户消息和 Outbox，并生成不可变 `AcceptedTurnHandle`。Worker claim Run 后取得当前 `ActiveExecutionAuthority`，生命周期协调器通过 `BeginTurn` 验证二者，再调用编译核心形成只读 `PreparedTurn`。这些组件不是当前仓库已有实现。

每次模型调用前，`ContextMiddleware.BeforeModelRewriteState` 读取 `state.Messages`、`state.ToolInfos` 和 `state.DeferredToolInfos`，调用 `BudgetCompiler`。Compiler 仅在达到阈值时治理上下文，并把合法的新状态交回 Eino。

Eino 退出后，外层 Harness 在成功、失败、取消等所有终态调用 `FinalizeTurn`。成功结果先幂等持久化助手消息；获得稳定消息边界后，再调度摘要检查、长期记忆提取和最终 Manifest。`AfterAgent` 只作为成功路径扩展点，不是可靠写回的唯一入口。

### 5.1 当前代码到目标组件的迁移

| 目标组件 | 当前代码 | 迁移说明 |
|---|---|---|
| `ContextCompiler` | `internal/agent/chat/context_builder.go` | W2 通过 Provider 适配重用现有数据源；Shadow 验证后由新编译路径替代 `BuildMessages` |
| `PromptProvider` | `prompts.ChatAgentSystemPrompt` 与 `ChatAgentBuilder.buildSystemPrompt` | 抽取现有资料列表渲染为单一实现，`StaticPromptProvider` 与 Legacy Builder 共同调用，避免复制规则 |
| `HistoryProvider` | `cache.ChatCache`、`MessageRepository` 与 `ConversationRepository` | `LegacyHistoryProvider` 保留摘要及历史的 Redis → MySQL 回退；缓存回填由 Provider 自己管理 |
| `Registry` | 无 | W1 新增不可变代码级注册表并在 `internal/app` 启动组装 |
| `TokenCounter` | 无 | W3 新增 Provider-aware 实现 |
| Eino Adapter | `ChatAgentBuilder` / `ChatModelAgent` | W4 注入接口式 Handler，并在 `BeforeModelRewriteState` 调用编译器 |
| `TurnLifecycleCoordinator` | 无；相关职责散落在 `chatAgentService` | W5 在 Harness 可用后新增，当前路径不伪造 Run/Authority |
| Writer 实现 | `chatAgentService.saveResults` 等分散逻辑 | 只能作为迁移行为来源；现有 `MessageRepository.Create` 不具备目标事务、幂等和 fencing 保证 |

迁移期间 `ContextBuilder` 保留为 Legacy 路径；首次启用新路径时也不立即删除，删除需单独批准。

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
| 整体方案 | 轻量 ContextCompiler + 可选 TurnLifecycleCoordinator + Eino Middleware |
| Provider 接口 | 按能力定义强类型接口 |
| 内部候选 | 可裁剪数据标准化为 `ContextItem` |
| RAG | 仅作为按需工具 |
| 用户记忆 | Provider 负责召回排名，ContextCompiler 负责最终注入 |
| 子 Agent 记忆 | Main 最小披露，Search 不直接读取记忆库 |
| 会话历史 | 带游标摘要 + 最近完整消息组 + Token 窗口 |
| 入口用户消息 | RunService 在接受 Run 时可靠持久化 |
| 执行后写回 | 未来 TurnLifecycleCoordinator 生成计划并调度 Writer；主结果与派生 intent 原子提交 |
| 摘要 | 运行内压缩与持久化会话摘要分离 |
| Search 写回 | 保存 Step Result/Artifact，不写主会话 Assistant Message |
| 恢复 | PreparedTurn 与 CompileRecord 随 checkpoint 恢复 |
| 并发会话 | 首期同一 Conversation 只允许一个活动 Turn |
| 成功状态流 | `running → finalizing → completed` |
| 写回授权 | 主结果使用 ActiveExecutionAuthority；派生/Repair 使用 FinalizationAuthority |
| 预算 | 硬保留 + 分类上限 + 优先级填充 + 高低水位 |
| 计数 | Anthropic 官方计数；已知 OpenAI 编码本地兼容计数；未知兼容模型显式配置或保守估算 |
| 错误策略 | 每阶段重试，阶段间回退，耗尽后 Abort 或 Skip |
| 并发 | 按依赖阶段并行；回退链内部串行 |
| 缓存 | 数据缓存归 Provider；Manager 只缓存版本化纯计算结果 |
| 可观测 | 默认无正文 Manifest；受控诊断显式开启 |
| 迁移 | `legacy` → `shadow` → `enabled` |
| 当前阶段 | W0 进行中；静态 Eino 核对完成，基线与契约测试待实现 |
| Harness 依赖 | W0–W4 可独立交付；W5–W7 等待 Harness 设计批准和基础能力 |

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
- `FinalizeTurn` 重试不会重复创建助手消息、摘要版本或记忆事实。
- 过期 fencing token 无法提交主结果或创建派生写回 intent。
- 助手消息成功持久化后，摘要和记忆任务才能使用该稳定消息边界。
- 助手消息或 Search Step Result 与其派生 Outbox/Journal intent 在同一事务中提交。
- 写回失败不会依赖进程内孤立 goroutine；需要异步时通过 Harness 的可靠任务机制调度。
- 失败或取消的 Run 仍生成终态 Manifest，但不会被当作成功回答提取长期记忆。
- 暂停不执行终态 Finalize；Resume 恢复 PreparedTurn 和 CompileRecord 后继续。
- 相关单元测试、集成测试和 race 检测通过后才可启用 `enabled`。

## 9. 外部设计依据

以下官方资料用于校准 2026-07-27 的职责修订：

- [OpenAI Agents SDK Context Management](https://openai.github.io/openai-agents-python/context/)：区分本地运行上下文与模型可见上下文。
- [OpenAI Agents SDK Sessions](https://openai.github.io/openai-agents-python/sessions/)：会话历史在 Run 前加载，并在执行过程中写入本轮新项目。
- [LangChain Context Engineering](https://docs.langchain.com/oss/python/langchain/context-engineering)：通过 State、Store、Runtime Context 和 Middleware 共同管理上下文。
- [LangGraph Persistence](https://docs.langchain.com/oss/python/langgraph/persistence)：Graph State 在 step 边界 checkpoint，长期记忆由独立 Store 管理。
- [Google ADK Sessions](https://adk.dev/sessions/)：区分 Session、State 与跨 Session Memory。
- [Microsoft Agent Framework Context Providers](https://learn.microsoft.com/en-us/agent-framework/agents/conversations/context-providers)：Provider 在执行前提供上下文，并在执行后从请求与响应提取和保存信息。
- [Eino ChatModelAgentMiddleware](https://www.cloudwego.io/docs/eino/core_modules/eino_adk/eino_adk_chatmodelagentmiddleware/)：`BeforeModelRewriteState` 位于每次模型调用前，`AfterAgent` 只覆盖成功终止。
- [Anthropic Effective Context Engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)：将上下文视为每次推理都需要重新策划的有限注意力预算。

这些实现并不存在统一命名的万能 ContextManager。共同模式是 Session/State 持久化、模型调用前上下文编译、领域 Store，以及调用完成后的窄写回 Hook。本设计用 ContextCompiler 和可选 TurnLifecycleCoordinator 表达这些横切点，但不吞并底层领域所有权。
