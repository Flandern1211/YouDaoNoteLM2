# 核心契约与数据模型

## 1. 包边界

以下路径均为目标态，当前仓库尚不存在 `internal/agentcontext`。W1 才会新增共享核心；当前实现仍是 `internal/agent/chat/context_builder.go` 和 `internal/service/chat_agent_service.go`。

```text
internal/agentcontext/
  核心类型、ContextCompiler、Registry、Profile、预算与解析策略

internal/agentcontext/adapter/
  现有 Prompt、ChatCache、MessageRepository 和 Token 能力适配

internal/agentcontext/eino/
  MessagePlan 渲染与 ChatModelAgentMiddleware

internal/agentcontext/writeback/
  依赖未来 Harness 的生命周期与 Writer 协调契约
```

共享核心不依赖具体 Redis、MySQL、向量库、记忆实现或 Eino Agent Builder。Provider 端口由核心使用方定义，当前基础设施的实现放在 `adapter`；Eino 细节只进入 `eino`；Writer 接口由 `writeback` 使用方拥有，具体持久化实现通过 `internal/app` 注入。首期不因文件数量预先拆出更多子包。

## 2. 编译核心与生命周期端口

```go
type ContextCompiler interface {
    PrepareTurn(
        ctx context.Context,
        turn *TurnSession,
        req PrepareTurnRequest,
    ) (*PreparedTurn, error)

    CompileModelInput(
        ctx context.Context,
        req CompileRequest,
    ) (*CompiledContext, error)
}

type TurnLifecycleCoordinator interface {
    BeginTurn(
        ctx context.Context,
        req BeginTurnRequest,
    ) (*TurnSession, error)

    FinalizeTurn(
        ctx context.Context,
        req FinalizeRequest,
    ) (*FinalizeResult, error)
}

type Manager interface {
    ContextCompiler
    TurnLifecycleCoordinator
}
```

生命周期语义：

- `ContextCompiler` 是 W0–W4 的可独立交付边界：`PrepareTurn` 每个 Agent invocation 执行一次，`CompileModelInput` 由 Eino Middleware 在每次模型调用前执行。
- `TurnLifecycleCoordinator` 是 W5–W7 的 Harness 集成边界：`BeginTurn` 验证持久化 Handle 和执行权，`FinalizeTurn` 覆盖成功、失败、取消等终态。
- 聚合 `Manager` 只是目标态应用门面，不要求第一批实现同时具备尚不存在的 Harness 能力。

当前 `chatAgentService` 没有 Run、Accepted Handle、Worker claim 或 fencing token，不能伪造 `TurnLifecycleCoordinator`。W0–W4 使用明确标记为 Legacy/Shadow 的 `TurnSession` 适配或测试夹具接入编译核心；只有 Harness 提供事实源后才能启用生产 `BeginTurn` / `FinalizeTurn`。

实现应保持无跨 Run 的可变会话状态。生命周期对象通过参数显式传递，便于重试、恢复和并发测试。

## 3. 请求模型

```go
type AcceptedTurnHandle struct {
    RunID           string
    StepID          string
    AgentID         AgentID
    UserID          uint
    ConversationID  uint
    Input           TurnInput
    CurrentInputRef *MessageRef
    ContextMode     ContextModeSnapshot
}

type ActiveExecutionAuthority struct {
    AttemptID      string
    FencingToken  uint64
    RunStateVersion uint64
}

type BeginTurnRequest struct {
    Handle    AcceptedTurnHandle
    Authority ActiveExecutionAuthority
}

type ContextModeSnapshot struct {
    Mode             string
    WritebackOwner   string
    ContractVersion  string
}

type MessageRef struct {
    MessageID uint
    Sequence  uint64
    Hash      string
}

type TurnSession struct {
    Handle  AcceptedTurnHandle
    Profile ContextProfileSnapshot
}

type PrepareTurnRequest struct {
    Model ModelRef
}
```

Chat/Main 的 `CurrentInputRef` 必须指向 RunService 已可靠持久化的入口用户消息。HistoryProvider 只加载该会话序号之前的历史，当前输入由 Handle 单独注入一次。

SearchTask 不产生伪造的用户会话消息，因此 Search 的 `CurrentInputRef` 可以为空；它依靠 Run/Step 标识和强类型 `SearchTaskInput` 建立边界。

`BeginTurn` 不直接查询 Repository，而是依赖使用方定义的窄验证端口：

```go
type TurnVerifier interface {
    VerifyAccepted(
        context.Context,
        AcceptedTurnHandle,
    ) (VerifiedTurn, error)

    VerifyAuthority(
        context.Context,
        string,
        ActiveExecutionAuthority,
    ) error
}
```

Verifier 由未来 RunService/Harness 适配器实现。当前仓库没有对应事实源或实现。`VerifyAccepted` 确认 Run、Step、User、Conversation、输入 Hash、当前消息和 ContextMode 与事实源一致；`VerifyAuthority` 确认当前 Attempt、FencingToken 和 RunStateVersion 仍有执行权。生命周期协调器只消费验证结果，不了解查询或签名实现。

Accepted Handle 在 Run 接纳后保持不变。Active Execution Authority 只在 Worker claim 后产生，Resume/接管会更换；`FinalizeTurn` 的主结果提交必须接收并验证调用时最新的 Authority，不能复用 Begin 时的旧值。

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

## 4. PreparedTurn 与 MessagePlan

```go
type PreparedTurn struct {
    Session           *TurnSession
    Profile           ContextProfileSnapshot
    Instruction       string
    MessagePlan       MessagePlan
    ModelCapabilities ModelCapabilities
    BaseManifest      ContextManifest
}

type PreparedTurnSnapshot struct {
    SchemaVersion      string
    RunID              string
    StepID             string
    Profile            ContextProfileSnapshot
    Prompt             PromptSnapshot
    MessagePlan        MessagePlanSnapshot
    ModelCapabilities  ModelCapabilities
    BaseManifest       ContextManifest
}

type MessagePlan struct {
    Summary      *ContextItem
    Memories     []ContextItem
    History      []*schema.Message
    CurrentInput TurnInput
}
```

`PreparedTurn` 创建后视为只读。Eino Adapter 和 ContextCompiler 可以基于 `MessagePlan` 重新渲染消息，但不得修改 Provider 原始记录。ReAct 多轮复用同一份稳定候选，动态工具结果来自 Eino Agent State。

Snapshot 使用框架无关、可序列化 DTO 表示消息和 ContextItem，不保存 `context.Context`、Provider/Writer/Repository 客户端、函数、锁、流对象、明文凭据或 Eino Runtime 指针。Checkpoint 依照 Harness 设计加密并做完整性校验。

```go
type PreparedTurnSnapshotCodec interface {
    Snapshot(*PreparedTurn) (PreparedTurnSnapshot, error)
    Restore(
        PreparedTurnSnapshot,
        *TurnSession,
    ) (*PreparedTurn, error)
}
```

Restore 接收通过 `BeginTurn` 重新验证的 TurnSession，并校验 schema、Run/Step、User、Conversation、ContextMode、CurrentInputRef、Profile、Prompt、模型和工具契约版本。版本或身份不兼容时进入 `checkpoint_incompatible/suspended`，不能静默重新召回 Provider 并继续。

## 5. 模型调用编译

```go
type CompileRequest struct {
    Turn              *PreparedTurn
    Messages          []*schema.Message
    ToolInfos         []*schema.ToolInfo
    DeferredToolInfos []*schema.ToolInfo
}

type CompiledContext struct {
    Messages []*schema.Message
    Record   CompileRecord
}

type CompileRecord struct {
    ModelCallID            string
    Manifest               ContextManifest
    ContextHMAC            string
    RuntimeSummaryCandidate *SummaryCandidate
    AppliedStateUpdate     *AgentStateUpdateRef
}
```

首期不由 ContextCompiler 增删工具。工具集合由 Agent Builder 和 Eino Runtime 决定，Compiler 负责计入 Token 和检测硬预算。

`CompiledContext` 是当前模型调用的瞬时视图，不等于持久化会话历史。运行内摘要、临时裁剪或工具结果缩减是否更新 Eino State，由 Middleware 契约明确表达，不能隐式覆盖领域存储。

Middleware/Harness 必须按模型调用顺序收集 `CompileRecord`。这些记录随 Eino checkpoint 保存或写入受控的 Run-local 状态，并在终态显式传给 `FinalizeTurn`；ContextCompiler 和生命周期协调器都不依赖进程内隐藏集合。

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

### 6.1 不可变 Profile Registry

首期不定义可变注册接口或运行时 Profile 编辑器。W1 在核心包中提供一个构造后只读的具体注册表：

```go
type ProfileKey struct {
    Name    string
    Version string
}

type Registry struct {
    profiles map[ProfileKey]ContextProfile
}

func NewRegistry(profiles ...ContextProfile) (*Registry, error)
func (r *Registry) Resolve(key ProfileKey) (ContextProfile, bool)
```

`internal/app` 启动时显式传入三个 Key，其规范化展示分别为 `chat.v1`、`main.v1`、`search.v1`。构造阶段校验空 Key、重复 Key、输入类型和 Source/WritebackPolicy 组合；构造完成后不提供 `Register` 或修改方法。新版本与旧版本并存，Run、Checkpoint 和 Manifest 固化完整 `ProfileKey`，不能原地覆盖旧版本。

只有出现第二种真实注册表实现时才抽取 `ProfileRegistry` 接口，避免为首期不存在的动态来源增加抽象。

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

接口由 ContextCompiler 使用方定义。实现可以来自当前仓库、未来记忆模块或第三方适配器。

## 8. Writer 接口与写回请求

Writer 接口位于目标包 `internal/agentcontext/writeback`，由 `WritebackCoordinator` 这一使用方拥有；具体数据库/缓存实现位于适配层并在 `internal/app` 注入。Writer 只理解自己的领域输入，不读取完整编译器或生命周期协调器内部状态：

```go
type AssistantMessageWriter interface {
    CommitAssistant(
        context.Context,
        AssistantWriteRequest,
    ) (CommittedMessage, error)
}

type SummaryWriter interface {
    EvaluateAndUpdate(
        context.Context,
        SummaryWriteRequest,
    ) (SummaryWriteResult, error)
}

type MemoryWriter interface {
    EvaluateAndStore(
        context.Context,
        MemoryWriteRequest,
    ) (MemoryWriteResult, error)
}

type ManifestWriter interface {
    StoreManifest(
        context.Context,
        ManifestWriteRequest,
    ) error
}

type StepResultWriter interface {
    CommitStepResult(
        context.Context,
        StepResultWriteRequest,
    ) (CommittedStepResult, error)
}
```

Writer 名称描述的是领域能力，不要求每次都实际产生新数据。例如 `SummaryWriter.EvaluateAndUpdate` 可以返回 `not_needed`。

现有 `repository.MessageRepository.Create` 没有 `context.Context`、事务、幂等 revision、Outbox/Journal 或 fencing 校验，不能直接宣称实现了 `AssistantMessageWriter`。`chatAgentService.saveResults`、`saveMessages` 和 `updateSummary` 只作为迁移行为来源；W5 必须在 Harness 的事务与 Authority 契约确定后实现新的持久化适配。

```go
type FinalizeRequest struct {
    Turn           *PreparedTurn
    Outcome        TurnOutcome
    CompileRecords []CompileRecord
    FinalizeKey    FinalizeKey
    Authority      ActiveExecutionAuthority
    Ticket         FinalizationTicket
}

type FinalizeKey struct {
    RunID    string
    Revision uint64
}

type WritebackIntent struct {
    Key               FinalizeKey
    Operation         WritebackOperation
    ProfileID         string
    InputRef          *MessageRef
    PrimaryOutputRef  *OutputRef
    CompileRecordRefs []string
    PayloadVersion    string
}

type TurnOutcome struct {
    Status         TurnStatus
    PrimaryOutput  PrimaryOutput
    Messages       []*schema.Message
    Usage          UsageSummary
    TerminalReason string
}

type PrimaryOutput interface {
    isPrimaryOutput()
}

type ConversationOutput struct {
    FinalMessage *schema.Message
    References   []ArtifactRef
}

type StepOutput struct {
    Result    SearchResult
    Artifacts []ArtifactRef
}

type FinalizationTicket struct {
    ID          string
    Key         FinalizeKey
    IntentKinds []WritebackOperation
}

type FinalizationAuthority struct {
    TicketID      string
    LeaseToken    string
    TicketVersion uint64
}

type FinalizeResult struct {
    Primary  *CommittedOutput
    Summary  WritebackStatus
    Memory   WritebackStatus
    Manifest WritebackStatus
}
```

`TurnOutcome.Messages` 只包含可持久化的显式 Agent 消息和工具事件，不包含模型隐藏思维链。Chat/Main 成功时必须返回 `ConversationOutput`；Search 成功时必须返回 `StepOutput`；非成功终态的 `PrimaryOutput` 为空。MemoryWriter 的输入必须由 Manager 过滤为调用方原始输入、本轮真实输出和允许的来源引用，排除历史、记忆和其他 Provider 注入的消息，避免记忆反馈循环。

Profile 定义强类型 `WritebackPolicy`：

```text
chat/main + success + final message
  → AssistantMessageWriter.CommitAssistant
  → 在同一事务中记录 Summary/Memory/Manifest 的 durable intents
  → SummaryWriter 与 MemoryWriter
  → ManifestWriter

search + success
  → StepResultWriter.CommitStepResult
  → ManifestWriter

failed / cancelled / no final message
  → 跳过成功回答写入和长期记忆提取
  → ManifestWriter 记录终态和部分上下文
```

Chat/Main 的 Assistant 写入和 Search 的 Step Result 写入是成功 Run 的必要提交点。Summary 和 Memory 是可独立重试的派生写回。

为消除“主结果已提交、派生任务尚未创建”的崩溃窗口，主结果 Writer 必须在同一个数据库事务中：

1. 校验 `ActiveExecutionAuthority`。
2. 幂等提交 Assistant Message 或 Step Result。
3. 持久化 TurnLifecycleCoordinator 已计算的后续 `WritebackIntent`。
4. 写入 Outbox 或 Finalization Journal。

成功执行先通过 Harness 状态事务从 `running` 进入 `finalizing`，在该事务中创建 FinalizationTicket、基础 Manifest intent 和 CompileRecord 引用，并返回包含新 RunStateVersion 的 ActiveExecutionAuthority。Run 只有在主结果和所有必要 durable intents 提交成功后，才能 CAS 从 `finalizing` 进入 `completed`。Summary、Memory 和最终 Manifest 可以稍后执行；Repair Scanner 根据未完成 Journal/Outbox 补齐，TurnLifecycleCoordinator 不创建孤立 goroutine。即使 Assistant 提交后进程立即退出，也存在可发现、可重试的持久化意图。

Intent 必须保存足以重建 Writer 请求的稳定引用、CompileRecord 引用和协议版本。Assistant/Step Result 提交事务负责回填 `PrimaryOutputRef`。敏感正文不复制进普通 Outbox；Writer 从已提交消息、加密 Run Result 或受控 checkpoint 按引用读取。

所有 Writer 请求都必须包含相同的 `FinalizeKey` 和 `FinalizationTicket`，但授权类型按阶段区分：

- 主结果提交和新增 intent：使用当前 `ActiveExecutionAuthority`，Repository 校验 Attempt、FencingToken、RunStateVersion 和 `finalizing` 状态。
- Summary/Memory/Manifest 派生执行与 Repair：Worker claim FinalizationTicket 后获得 `FinalizationAuthority`，Repository 校验 Ticket ID、Writeback LeaseToken、TicketVersion 和 operation 状态。

Harness 在进入 `finalizing` 或不可恢复失败/取消终态时分配并持久化 revision；同一逻辑结果的网络重试、Worker 重投和 Repair 重试必须复用该 revision。可恢复暂停不分配 revision；只有显式重开并形成新的终态结果时才能递增。领域唯一键至少包含 `run_id + revision + operation`。

拥有执行权及授权校验的是 Harness/Repository 层，TurnLifecycleCoordinator 不自行实现租约协议。过期 Agent Worker 既不能提交主结果，也不能创建新的 durable intents；派生/Repair Worker 使用独立 Finalization Authority，不复用原 Attempt 的 fencing 权限。

成功路径在 `running → finalizing` 状态事务中创建 Ticket 和 Manifest intent，随后由主结果事务补充 Summary/Memory intents，最后进入 `completed`。失败/取消路径在写入对应终态的同一事务中创建 Ticket 和 Manifest intent。主结果提交持续失败时 Run 保持 `finalizing` 并按策略重试或进入 `suspended`；Repair Scanner 始终能从 Ticket 发现并补写 Manifest。

Finalization 使用 Harness 创建的独立有界 context，继承 trace 和审计身份，但不沿用已经取消的 Run context，也不使用无期限的 `context.Background()`。ManifestWriter 对所有终态执行；重试由 `FinalizeKey` 收敛。

## 9. 解析链

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

## 10. 模型能力与 Token 计数

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

首期 TokenCounter 矩阵：

```text
Anthropic：
  快速路径 → 保守本地估算
  接近阈值 → 现有 anthropic-sdk-go Messages.CountTokens

已知 OpenAI 模型：
  模型注册表显式映射 o200k_base / cl100k_base
  → 经过契约测试的 Go tiktoken-compatible 本地实现

自定义 OpenAI-compatible 模型：
  显式配置 TokenizerStrategy 和 ContextWindow
  → 已知编码或 conservative_utf8_bytes

无匹配策略：
  生产环境拒绝装配；开发环境只有显式启用后才允许保守估算
```

Anthropic 官方 CountTokens 结果可以标记为 `exact_provider`。OpenAI 本地 tokenizer 即使编码匹配，聊天消息、工具 Schema 和服务端封装开销仍可能变化，标记为 `compatible_local` 而不是永久精确值。所有结果记录 CounterMode、编码/策略版本和安全余量，供 BudgetCompiler 与 Manifest 使用。

中文不使用“字符数除以四”等英文经验公式：匹配编码时直接 tokenizer 编码；没有编码时以 UTF-8 字节数为正文保守上界，并额外加入角色、消息、ToolInfo 和 DeferredToolInfo 的结构开销。Provider 返回的调用后 usage 只用于校准偏差，不能替代调用前预算。

是否引入具体 Go tokenizer 依赖由 W3 契约测试决定：候选实现必须覆盖所需编码、中文、特殊 Token、消息与工具 Schema 样例，并与官方样例或真实 usage 对比。测试通过前不把第三方实现写成“精确计数器”。

## 11. 错误类型

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
InvalidTurnHandle
AssistantCommitFailed
WritebackDispatchFailed
ActiveExecutionAuthorityRejected
FinalizationAuthorityRejected
FinalizeRevisionConflict
PrimaryResultCommitFailed
```

错误需要携带 Agent、Profile、Provider/Writer 能力、操作阶段和 Run 标识等定位上下文，并使用 `%w` 保留错误链。API 文案转换属于上层职责。
