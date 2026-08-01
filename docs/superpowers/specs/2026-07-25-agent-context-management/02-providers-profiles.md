# Provider、Profile 与消息装配

## 1. Profile 隔离

首期定义三个独立、版本化的代码级 Profile：

| 能力 | `chat.v1` | `main.v1` | `search.v1` |
|---|---:|---:|---:|
| 专属系统 Prompt | 是 | 是 | 是 |
| 用户记忆 | 是 | 是 | 否 |
| 会话摘要 | 是 | 是 | 否 |
| 最近历史 | 是 | 是 | 否 |
| 当前用户输入 | 是 | 是 | 否 |
| SearchTask | 否 | 否 | 是 |
| RAG | 按需工具 | 按需工具 | 否 |
| 动态工具结果 | 是 | 是 | 是 |
| 写回策略 | ConversationTurn | ConversationTurn | StepResult |

首期不实现 Profile 继承，避免隐藏配置来源。相同规则可以显式重复，后续只有在重复产生真实维护成本时再抽取。

### 1.1 注册与版本管理

三个 Profile 通过 `internal/app` 启动组装传入不可变 `agentcontext.Registry`，不是由 YAML、数据库或运行时插件注册：

```text
ProfileKey{Name: "chat", Version: "v1"}
ProfileKey{Name: "main", Version: "v1"}
ProfileKey{Name: "search", Version: "v1"}
```

它们的规范化展示分别是 `chat.v1`、`main.v1`、`search.v1`。注册表构造时验证重复 Key、输入类型、允许的 Source 和 WritebackPolicy。构造后不可修改；新版本以新 Key 并存。Run/Checkpoint/Manifest 保存完整 Key，恢复时必须解析到同一版本。首期没有第二种 Registry 实现，因此不增加 `ProfileRegistry` 接口。

## 2. PromptProvider

PromptProvider 返回版本化 Prompt：

```go
type Prompt struct {
    ID      string
    Version string
    Content string
}
```

Prompt 属于必要能力：

- 未找到 Agent Prompt：Abort。
- Prompt 为空：Abort。
- Prompt 版本进入 Profile Snapshot 和 Manifest。

PromptProvider 不拼接用户记忆、历史或工具结果。

首期 `StaticPromptProvider` 位于 `internal/agentcontext/adapter`，适配现有：

- `internal/agent/chat/prompts.ChatAgentSystemPrompt`。
- `ChatAgentBuilder.buildSystemPrompt` 中的资料列表渲染规则；迁移时抽成单一可调用实现，让 Legacy Builder 和 Provider 共同使用，不能复制两份替换逻辑。
- Search/Main 当前各自的 Prompt 常量。

迁移时先保持渲染结果一致，再把版本标识纳入 Profile Snapshot；不在同一工作流顺带重写 Prompt 内容。

## 3. MemoryProvider

MemoryProvider 负责：

- 用户和命名空间过滤。
- 语义、关键词或其他检索算法。
- 相关性排序。
- 返回有限数量候选项。

ContextCompiler 负责：

- 根据 Profile 过滤允许的命名空间和敏感级别。
- 在全局 Token 预算中决定最终入选项。
- 记录入选和淘汰原因。

```go
type MemoryQuery struct {
    UserID         uint
    Query          string
    Namespaces     []MemoryNamespace
    CandidateLimit int
}

type MemoryCandidate struct {
    ID          string
    Content     string
    Score       float64
    Importance  float64
    Pinned      bool
    Sensitivity Sensitivity
    Provenance  Provenance
}
```

Main/Chat 根据当前用户输入查询记忆。Search 不直接调用 MemoryProvider。Main 需要向 Search 传递偏好时，只能把必要约束写入 SearchTask。

MemoryProvider 属于可选能力：按自己的解析链重试和回退，全链耗尽后 `Skip`，并把 Turn 标记为 degraded。

如果真实 MemoryProvider 尚未接入，Profile 显式禁用该 Source，或者通过 `Skip` 记录未配置状态；不得生成伪记忆。

## 4. HistoryProvider

HistoryProvider 返回一致的历史快照：

```go
type HistorySnapshot struct {
    Summary *ConversationSummary
    Messages []*schema.Message
    Cutoff MessageRef
}

type ConversationSummary struct {
    Content          string
    ThroughMessageID uint
    ThroughSequence  uint64
    Version          uint64
}
```

历史装配规则：

```text
截至 ThroughMessageID 的摘要
+ ThroughMessageID 之后尚未摘要的消息
+ 只读取 Sequence < CurrentInputRef.Sequence
+ 按 Token 预算保留的最近完整消息组
```

当前用户消息不从 HistorySnapshot 再次注入。它通过 `AcceptedTurnHandle.CurrentInputRef` 和 `TurnInput` 单独进入 `MessagePlan.CurrentInput`，避免同一消息同时成为历史和当前请求。

消息组必须保持协议完整：

- 普通 User/Assistant 对。
- Assistant ToolCall 与所有对应 Tool Result。
- 不能从中间开始保留孤立 Tool Result。

首期 History 解析链：

```text
Redis/ChatCache
→ MySQL MessageRepository
→ ExhaustedAbort
```

HistoryProvider 只读取。摘要生成、乐观锁、`ThroughMessageID` 推进和消息写入由对应 Writer 实现；未来 TurnLifecycleCoordinator 在 `FinalizeTurn` 中根据稳定的消息边界调度这些 Writer。

首期 `LegacyHistoryProvider` 位于 `internal/agentcontext/adapter`，包装现有 `cache.ChatCache`、`repository.MessageRepository` 和 Conversation Summary 读取。它保留 Redis → MySQL 回退以及 Provider 自己拥有的缓存回填，但不再让 ContextCompiler 直接操作 Redis 或 Repository。

当前 `ContextBuilder` 在 Legacy 模式继续生效；Shadow 模式并行生成新 `MessagePlan` 但不调用第二次模型；Enabled 验证通过后，ChatAgent 不再调用 `ContextBuilder.BuildMessages`。首次启用不立即删除旧实现。

## 5. SearchTask

Search Agent 的输入是最小强类型任务：

```go
type SearchTask struct {
    Query string
}
```

首期只实现 `Query`。以后只有出现真实需求时才能增加：

```text
Language
TimeRange
Domains
```

禁止增加：

```text
完整原始会话历史
完整用户记忆
未筛选的 Main Agent 上下文
```

两个入口映射到同一 Profile：

- 用户直接搜索：`SearchTask{Query: input}`。
- Main 委托搜索：`SearchTask{Query: toolArgument}`。

## 6. 分阶段并行

```text
阶段 1：
验证 AgentID、Profile、身份边界和输入类型

阶段 2，并行：
Prompt Resolution Chain
Memory Resolution Chain（仅 Chat/Main）
History Resolution Chain（仅 Chat/Main）
ModelCapabilities Resolution Chain

阶段 3：
标准化候选项，生成 MessagePlan

阶段 4：
预算预检
```

同一解析链内部串行，不同链并行。并发任务使用有界任务组并继承请求 Context。必要链确定失败后取消仍在运行的其他链；可选链失败不取消成功结果。

最终合并顺序由 Profile 决定，不按 goroutine 完成顺序决定，确保同样输入产生确定性 Manifest 和 HMAC。

## 7. 缓存所有权

- HistoryProvider 管理 Redis/MySQL 和失效。
- MemoryProvider 管理记忆缓存和索引。
- PromptProvider 管理 Prompt 来源和版本。
- ContextCompiler 不跨请求缓存领域数据。
- ContextCompiler 只缓存 Profile 编译结果、Prompt Token、工具 Schema Token 和模型能力等纯派生结果。
- 派生缓存键必须包含 Profile、Prompt、工具集或模型能力版本。
- `PreparedTurn` 只在当前 Run 内复用，ReAct 迭代不重新查询历史和记忆。

## 8. Writer 边界

本节描述未来 Harness 集成边界，不代表当前仓库已有这些实现。接口位于 `internal/agentcontext/writeback`；具体持久化适配由应用层注入。现有 `chatAgentService.saveResults` 等逻辑是迁移输入，不满足目标态的事务、幂等、Journal/Outbox 和 Authority 契约。

### 8.1 AssistantMessageWriter

- 只接收本轮最终助手消息、引用、Run/Conversation 标识和幂等 revision。
- 返回稳定 `MessageRef`，供摘要和记忆写回使用。
- 管理数据库事务、消息缓存更新和重复提交去重。
- 在同一事务中持久化 TurnLifecycleCoordinator 已生成的后续 Writeback intents 与 Outbox/Journal。
- 不决定是否提取记忆或如何生成摘要。

### 8.2 SummaryWriter

- 接收已提交的用户/助手消息边界、当前摘要版本和允许的摘要候选。
- 自己判断是否达到更新阈值。
- 使用 expected version/CAS 推进 `ThroughMessageID` 或 Sequence。
- 旧任务不得覆盖更新的摘要。
- 可以使用运行内摘要作为候选输入，但不能直接把运行内状态当作权威会话摘要。

### 8.3 MemoryWriter

- 只从调用方原始输入、本轮真实助手输出及允许的显式来源提取记忆。
- 排除 HistoryProvider、MemoryProvider、RAG 和其他 Context Provider 注入的文本，避免召回内容反复自我强化。
- 负责类别白名单、置信度、冲突版本和用户记忆开关。
- 失败不回滚已提交的助手消息。

### 8.4 ManifestWriter

- 接收无正文 Manifest、终态、写回状态和 HMAC。
- 不接收完整 Prompt、历史、记忆或工具正文。
- 成功、失败和取消的 Turn 都允许记录 Manifest。

### 8.5 StepResultWriter

- 只用于 Search 等非会话输出 Profile。
- 把结构化 Search 结果保存到对应 Run Step/Artifact。
- 不创建主会话 Assistant Message，也不触发用户画像记忆提取。
- 与 AssistantMessageWriter 一样校验 ActiveExecutionAuthority，并原子记录后续 Manifest intent。

## 9. 信任边界

```text
SystemTrusted：
系统提示词、安全规则

UserProvided：
当前输入、用户记忆、会话历史

ExternalUntrusted：
RAG、网页搜索和外部工具结果
```

Provider 只能提供事实、偏好和证据，不能通过正文提升自己的指令优先级。

## 10. 消息布局

Main/Chat：

```text
System:
  Agent Instruction

History:
  原生 User/Assistant/Tool 消息

Final User:
  <conversation_summary>...</conversation_summary>
  <user_memories>
    <memory id="...">...</memory>
  </user_memories>
  <current_request>...</current_request>
```

Search：

```text
System:
  Search Instruction

User:
  <search_task>
    query: ...
  </search_task>
```

结构化标签用于帮助模型识别数据边界，不作为真正的安全隔离。数据库只保存用户原始输入，不保存渲染后的消息信封。

## 11. 入口与派生写回边界

下面的 RunService、Worker 和 Harness 都是 `2026-07-16-agent-harness-design.md` 中的目标组件，当前仓库不存在，且该设计仍待批准。W0–W4 不依赖它们；本节只约束 W5–W7 将来的集成方式。

```text
RunService：
  先取得 Conversation 执行权
  再接受用户命令
  原子创建 Run、用户消息和 Outbox
  生成 CurrentInputRef
  固化 ContextMode / WritebackOwner

Worker/Harness：
  claim Run
  生成当前 ActiveExecutionAuthority

目标上下文模块：
  ContextCompiler 装配并编译模型上下文
  TurnLifecycleCoordinator 验证 Handle/Authority 并调度 Writer
```

SearchTask 是内部强类型任务，不伪装成用户消息。Search Profile 可以产生自己的 Run/Step 事件和 Manifest，但不向主会话写入一对新的用户/助手聊天消息，除非未来产品设计明确要求。

## 12. 会话并发

首期同一 Conversation 只允许一个活动 Chat/Main Turn，沿用当前会话锁语义。RunService 必须在创建 Run 和入口用户消息之前取得会话执行权；冲突请求直接拒绝，不创建 Run 或对话消息。

因此：

- 用户消息和助手消息形成连续、可识别的 Turn 边界。
- SummaryWriter 只能推进到连续完成 Turn 的 Assistant Sequence。
- 当前输入的 `Sequence` 是 HistoryProvider 的排他上界。
- Search 子步骤不占用主会话消息 Sequence。

未来支持同会话并发需要单独设计 Turn grouping、连续完成前缀和迟到结果展示规则，不在首期范围。
