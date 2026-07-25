# 核心契约与数据模型

## 1. 包边界

共享核心放在 `internal/agentcontext`。该包定义使用方所需的最小接口、Profile、解析策略、预算输入输出和 Manifest，不依赖具体 Redis、MySQL、向量库或记忆实现。

Eino 相关转换放在 `internal/agentcontext/einoadapter`，避免 Provider 和预算策略直接散落 Eino 细节。

具体 Provider 实现放在其领域模块或适配包中，并在 `internal/app` 组装。

## 2. Manager

```go
type Manager interface {
    PrepareTurn(
        ctx context.Context,
        req PrepareTurnRequest,
    ) (*TurnContext, error)

    CompileModelInput(
        ctx context.Context,
        req CompileRequest,
    ) (*CompiledContext, error)
}
```

`PrepareTurn` 每次用户请求执行一次。`CompileModelInput` 由 Eino Middleware 在每次模型调用前执行。

## 3. 请求模型

```go
type PrepareTurnRequest struct {
    AgentID        AgentID
    UserID         uint
    ConversationID uint
    Model          ModelRef
    Input          TurnInput
}
```

`TurnInput` 使用封闭的强类型表示，不使用 `map[string]any`：

```go
type TurnInput interface {
    isTurnInput()
}

type UserMessageInput struct {
    Content string
}

type SearchTaskInput struct {
    Task SearchTask
}
```

调用方必须保证 `AgentID` 与输入类型匹配：

- `chat.v1`、`main.v1` 只接受 `UserMessageInput`。
- `search.v1` 只接受 `SearchTaskInput`。

## 4. TurnContext 与 MessagePlan

```go
type TurnContext struct {
    Profile           ContextProfileSnapshot
    Instruction       string
    MessagePlan       MessagePlan
    ModelCapabilities ModelCapabilities
    BaseManifest      ContextManifest
}

type MessagePlan struct {
    Summary      *ContextItem
    Memories     []ContextItem
    History      []*schema.Message
    CurrentInput TurnInput
}
```

`TurnContext` 创建后视为只读。Eino Adapter 和 BudgetCompiler 可以基于 `MessagePlan` 重新渲染消息，但不得修改 Provider 原始记录。

## 5. 模型调用编译

```go
type CompileRequest struct {
    Turn              *TurnContext
    Messages          []*schema.Message
    ToolInfos         []*schema.ToolInfo
    DeferredToolInfos []*schema.ToolInfo
}

type CompiledContext struct {
    Messages []*schema.Message
    Manifest ContextManifest
}
```

首期不由 ContextManager 增删工具。工具集合由 Agent Builder 和 Eino Runtime 决定，Compiler 负责计入 Token 和检测硬预算。

## 6. ContextItem

只有能独立选择或淘汰的数据块转换为 `ContextItem`：

```go
type ContextItem struct {
    ID         string
    Kind       ContextKind
    Content    string
    Priority   int
    Trust      TrustLevel
    TokenCount int
    Pinned     bool
    Provenance Provenance
}
```

建议的 `ContextKind`：

```text
conversation_summary
user_memory
delegated_preference
```

原生会话消息、Assistant ToolCall 和 Tool Result 保持 `schema.Message`，不压平成 `ContextItem`。

`Pinned` 只提升记忆候选优先级，仍受 Memory 上限和总预算约束。

## 7. Provider 接口

```go
type PromptProvider interface {
    LoadPrompt(context.Context, PromptQuery) (Prompt, error)
}

type MemoryProvider interface {
    SearchMemory(context.Context, MemoryQuery) ([]MemoryCandidate, error)
}

type HistoryProvider interface {
    LoadHistory(context.Context, HistoryQuery) (HistorySnapshot, error)
}

type ModelCapabilitiesResolver interface {
    ResolveModel(context.Context, ModelRef) (ModelCapabilities, error)
}

type TokenCounter interface {
    CountTokens(context.Context, TokenCountRequest) (TokenCount, error)
}
```

接口由 ContextManager 使用方定义。实现可以来自当前仓库、未来记忆模块或第三方适配器。

## 8. 解析链

一个上下文能力由多个按顺序执行的 Stage 组成：

```go
type RetryPolicy struct {
    MaxAttempts int
    Backoff     BackoffPolicy
    Retryable   ErrorClassifier
}

type ExhaustedAction string

const (
    ExhaustedAbort ExhaustedAction = "abort"
    ExhaustedSkip  ExhaustedAction = "skip"
)
```

概念执行模型：

```text
Stage 1 Provider
  → RetryPolicy
  → 成功则结束
  → 耗尽则进入 Stage 2

Stage 2 Provider
  → 自己的 RetryPolicy
  → 成功则结束
  → 全链耗尽后执行 ExhaustedAction
```

“回退”表示切换 Stage；“降级”表示最终跳过某个可选能力后继续运行。两者不是同义词。

## 9. 模型能力与 Token 计数

```go
type ModelCapabilities struct {
    ContextWindow      int
    MaxOutputTokens    int
    TokenizerStrategy  TokenizerStrategy
    SupportsToolCalls  bool
}
```

模型能力解析顺序：

```text
部署或用户显式覆盖
→ 内置版本化模型注册表
→ 未知模型
```

未知模型在生产模式中拒绝装配；开发模式可以通过显式配置启用保守回退，但不能静默猜测。

TokenCounter 的实现优先级：

```text
Provider 官方计数接口
→ 官方或兼容的本地 tokenizer
→ 保守近似计数器
```

`TokenCount` 必须标记结果是精确值还是估算值，供安全余量和 Manifest 使用。

## 10. 错误类型

核心错误类别：

```text
InvalidTurnInput
ProfileNotFound
ProviderExhausted
ModelCapabilitiesUnknown
TokenCountUnavailable
HardBudgetExceeded
ToolContextOverflow
InvalidMessageSequence
```

错误需要携带 Agent、Profile、Provider 能力和阶段等定位上下文，并使用 `%w` 保留错误链。API 文案转换属于上层职责。
