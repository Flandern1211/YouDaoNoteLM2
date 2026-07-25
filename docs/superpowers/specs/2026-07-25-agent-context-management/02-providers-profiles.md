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

首期不实现 Profile 继承，避免隐藏配置来源。相同规则可以显式重复，后续只有在重复产生真实维护成本时再抽取。

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

## 3. MemoryProvider

MemoryProvider 负责：

- 用户和命名空间过滤。
- 语义、关键词或其他检索算法。
- 相关性排序。
- 返回有限数量候选项。

ContextManager 负责：

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
}

type ConversationSummary struct {
    Content          string
    ThroughMessageID uint
    Version          uint64
}
```

历史装配规则：

```text
截至 ThroughMessageID 的摘要
+ ThroughMessageID 之后尚未摘要的消息
+ 按 Token 预算保留的最近完整消息组
```

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

HistoryProvider 只读取。摘要生成、乐观锁、`ThroughMessageID` 推进和消息写入属于会话模块。

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
- ContextManager 不跨请求缓存领域数据。
- ContextManager只缓存 Profile 编译结果、Prompt Token、工具 Schema Token 和模型能力等纯派生结果。
- 派生缓存键必须包含 Profile、Prompt、工具集或模型能力版本。
- `TurnContext` 只在当前 Run 内复用，ReAct 迭代不重新查询历史和记忆。

## 8. 信任边界

```text
SystemTrusted：
系统提示词、安全规则

UserProvided：
当前输入、用户记忆、会话历史

ExternalUntrusted：
RAG、网页搜索和外部工具结果
```

Provider 只能提供事实、偏好和证据，不能通过正文提升自己的指令优先级。

## 9. 消息布局

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
