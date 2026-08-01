# 可观测性、迁移和测试

## 1. ContextManifest

生产默认 Manifest 不保存正文：

```go
type ContextManifest struct {
    ProfileID       string
    ProfileVersion  string
    PromptVersion   string
    ToolsetVersion  string
    Model           string

    InputBudget     int
    EstimatedTokens int
    ExactTokens     *int
    CounterMode     string

    Sources         []SourceManifest
    Writebacks      []WritebackManifest
    TurnStatus      string
    Degraded        bool
    ContextHMAC     string
}
```

`SourceManifest` 至少记录：

- 能力与 Provider 标识。
- 候选数量、入选数量和 Token。
- 状态：success、retry、fallback、skipped、failed。
- 重试次数和回退阶段。
- 延迟。
- 淘汰、截断或压缩原因。

不得记录：

- 用户输入正文。
- 用户记忆正文。
- 会话历史和摘要正文。
- RAG、搜索和工具结果正文。
- API Key、Cookie 或其他凭据。

## 2. 上下文指纹

使用带密钥的 HMAC 生成最终上下文指纹，不直接保存完整 Prompt 的 SHA-256：

```text
HMAC-SHA256(派生审计密钥, 规范化最终模型输入)
```

审计密钥从应用主密钥按固定用途标签派生，不能直接复用原始加密密钥字节。指纹用于判断装配结果是否相同，不用于恢复正文。

## 3. 受控诊断

首期不实现完整 Prompt 持久化。未来若增加诊断模式，必须同时满足：

- 显式配置开启。
- 请求采样。
- PII 和凭据脱敏。
- 短保留期。
- 管理员访问控制和审计。
- 高敏感或零保留请求强制关闭。

## 4. 指标

建议指标：

```text
context_prepare_duration
context_compile_duration
context_provider_duration
context_provider_retry_total
context_provider_fallback_total
context_provider_skip_total
context_budget_utilization
context_candidate_tokens
context_selected_tokens
context_trimmed_tokens
context_exact_count_total
context_summary_trigger_total
context_hard_budget_error_total
context_shadow_diff_total
context_finalize_duration
context_writeback_total
context_writeback_retry_total
context_writeback_pending
```

标签保持低基数，只使用 Agent/Profile/Provider/状态/原因枚举；不使用 userID、conversationID 或原始 query 作为指标标签。

## 5. 迁移模式

```text
legacy
shadow
enabled
```

### 5.1 Legacy

仅现有 `ContextBuilder` 生效。新模块不参与正式模型输入。

### 5.2 Shadow

旧实现继续调用模型。新模块仅对稳定采样的请求装配并生成 Manifest：

- 不发起第二次模型调用。
- 不执行 Assistant、Summary 或 Memory 领域写回；只生成不会落正文的模拟写回计划。
- 新模块失败不影响旧请求。
- 比较消息 ID、摘要游标、Token、裁剪原因、Provider 状态、计划写回动作和 HMAC。
- 不比较或记录正文。

采样按 userID 做稳定分桶，保证同一用户不会在请求间随机切换。

### 5.3 Enabled

ContextCompiler 生成正式输入；在 Harness 已落地的部署中，TurnLifecycleCoordinator 协调执行后写回。旧实现不能再对同一 Run 重复保存助手消息、触发摘要或提取记忆；旧路径保留为新请求的快速回滚入口，直到灰度和稳定期结束。

## 6. Feature Flags

首期复用项目现有 `pkg/config` + Viper 配置体系，不新增独立 Feature Flag 服务。目标配置挂在 `AgentConfig` 下：

```go
type AgentConfig struct {
    // 现有搜索 Agent 字段保持不变。
    ContextManagement ContextManagementConfig `mapstructure:"context_management"`
}

type ContextManagementConfig struct {
    Mode                    string `mapstructure:"mode"`
    ShadowSampleBasisPoints uint16 `mapstructure:"shadow_sample_basis_points"`
    MemoryEnabled           bool   `mapstructure:"memory_enabled"`
    ExactCountEnabled       bool   `mapstructure:"exact_count_enabled"`
    WritebackEnabled        bool   `mapstructure:"writeback_enabled"`
    RolloutVersion          string `mapstructure:"rollout_version"`
}
```

`Mode` 只允许 `legacy | shadow | enabled`；采样使用 0–10000 基点，避免浮点边界。`shadow` 或按用户灰度的 `enabled` 模式要求非空 `RolloutVersion`；纯 `legacy` 不强制，避免未启用新路径的部署被无关配置阻断。

Feature Flag 不允许改变安全边界。关闭精确计数时必须使用保守估算和更大安全余量。

`agent.context_management.writeback_enabled` 只用于迁移期隔离新旧写回路径。`enabled` 模式下如果该 Flag 关闭，必须显式回退到 Legacy 写回协调器，不能静默丢弃最终消息。

Shadow 稳定分桶只使用标准库 SHA-256，不新增依赖：

```text
digest = SHA256("context-shadow:" + rollout_version + ":" + decimal(user_id))
bucket = BigEndianUint64(digest[0:8]) % 10000
selected = bucket < shadow_sample_basis_points
```

相同 userID 和 rollout version 始终落在同一桶；提高采样率会保留原有样本；只有显式变更 rollout version 才重新分桶。测试必须包含边界基点、确定性和采样率单调性。

Run 创建时必须把以下值固化到配置快照：

```text
context_mode
writeback_owner
context_contract_version
profile_version
```

后续 Attempt、Resume、Repair 和 Finalize 只读取 Run 快照，不重新解析全局 Flag。全局 Flag 变化只影响新 Run。

当前尚无持久化 Run 的 Legacy/Shadow 接入只能在请求开始时生成不可变请求级快照；它不具备跨进程 Resume 保证。只有 Harness 可用后才能宣称按 Run 固化。

## 7. 单元测试

### 7.1 解析链

- 不重试直接成功。
- 同一 Provider 重试后成功。
- 主 Provider 耗尽后回退成功。
- 全链耗尽后 Abort。
- 全链耗尽后 Skip 并标记 degraded。
- Context 取消立即停止重试和回退。

### 7.2 Profile

- Chat/Main 只接受 UserMessageInput。
- Search 只接受 SearchTaskInput。
- Search Profile 不注册 Memory 和 History Source。
- Profile Snapshot 和版本保持不变。
- Registry 拒绝空 Key 和重复 Key。
- Registry 构造后不能运行时注册或覆盖 Profile。
- 新旧 Profile 版本可以并存并按完整 ProfileKey 恢复。

### 7.3 Memory

- Provider 返回有序候选。
- Namespace 和 Sensitivity 过滤正确。
- Pinned 在 Memory 上限内优先。
- 低分候选先被淘汰。
- MemoryProvider 失败按 Skip 降级。

### 7.4 History

- 摘要游标之前的消息不重复注入。
- 游标之后的消息不遗漏。
- Redis 失败回退 MySQL。
- User/Assistant 消息保持时间顺序。
- ToolCall/ToolResult 作为完整组处理。

### 7.5 Budget

- 硬保留内容计入预算。
- 工具 Schema 计入预算。
- 低于阈值走快速路径。
- 接近阈值触发精确计数。
- 超过高水位治理到低水位。
- 用户输入本身过长返回明确错误。
- 裁剪后消息序列合法。
- Anthropic 接近阈值时使用官方 CountTokens 适配。
- 已知 OpenAI 编码记录为 compatible_local，不误标 exact。
- 自定义 OpenAI-compatible 模型缺少显式 tokenizer 策略时生产环境拒绝。
- 中文保守回退按 UTF-8 字节和结构开销计数。

### 7.6 Feature Flag

- Viper 能从 YAML 和 `CLOUDQUE_` 环境变量加载 ContextManagementConfig。
- Mode、基点范围和 RolloutVersion 启动校验正确。
- 相同 userID/rollout version 分桶稳定。
- 提高采样率不会移除原有样本。
- 修改 rollout version 会产生新的确定性分桶。

### 7.7 Manifest

- 不包含候选或最终正文。
- 正确记录 retry、fallback、skip 和裁剪原因。
- 同一规范化输入产生相同 HMAC。
- 内容变化产生不同 HMAC。

### 7.8 生命周期与写回

- `BeginTurn` 拒绝缺失或不匹配的持久化 Handle。
- TurnVerifier 拒绝 Run、Step、User、Conversation、输入 Hash 或 ActiveExecutionAuthority 不匹配。
- Chat/Main 历史只读取当前输入 Sequence 之前的消息。
- 当前用户消息只注入一次。
- 成功 Turn 先提交 Assistant，再调度 Summary 和 Memory。
- Assistant 提交失败时不推进摘要或提取记忆。
- `FinalizeTurn` 重试不会创建重复消息、摘要版本或记忆事实。
- Summary 和 Memory 其中之一失败不回滚已提交 Assistant。
- 失败、取消和无最终回答不执行成功回答记忆提取。
- Provider 注入的历史、旧记忆和 RAG 结果不会被作为新记忆来源。
- Chat/Main 与 Search 分别选择 ConversationTurn 和 StepResult 写回策略。
- Chat/Main 成功结果拒绝 StepOutput，Search 成功结果拒绝 ConversationOutput。
- 同一逻辑 Finalize 重试复用持久化 revision。
- 暂停不分配 revision，也不执行终态写回。

## 8. 集成测试

- 使用真实 Eino Middleware 顺序验证每次模型调用前执行 Compile。
- 多轮 ToolCall 后上下文不重复注入。
- RAG 只在工具调用后进入消息状态。
- Search 直接入口和 Main 委托入口产生相同 SearchTask 语义。
- Search 无法读取 Main 的历史和记忆。
- Redis 不可用时 History 回退 MySQL。
- 可选 MemoryProvider 超时不阻断 Main。
- 必要 PromptProvider 失败时模型不被调用。
- 未知模型能力时生产模式拒绝调用。
- Shadow 模式异常不改变 Legacy 响应。
- Shadow 不执行新 Writer 的领域写入。
- Eino 成功、模型错误、取消和超迭代都经过 Harness 终态 Finalize 路径。
- 不依赖 Eino `AfterAgent` 覆盖失败和取消写回。
- Assistant/Step Result 与后续 intents 原子提交；提交后进程退出，Repair 可补齐 Summary、Memory 和 Manifest。
- 成功 Run 只能按 `running → finalizing → completed` 转换；主结果未提交时不得进入 completed。
- 重复 Worker 或过期 fencing token 不能重复提交写回。
- 派生/Repair Worker 使用 FinalizationAuthority，不复用旧 Attempt fencing。
- 每次模型调用的 CompileRecord 按顺序进入最终 Manifest。
- Pause 后 Resume 恢复 PreparedTurnSnapshot 和 CompileRecord，不重新召回变化后的历史或记忆。
- PreparedTurnSnapshot schema 或 Profile/Prompt/模型/工具契约不兼容时进入 suspended。
- Run 期间切换全局 Feature Flag 不改变该 Run 的 writeback owner。
- 同一 Conversation 的第二个活动 Turn 在创建 Run 和用户消息前被拒绝。
- Run context 已取消时，Harness 仍能用独立有界 finalization context 创建或处理终态 Manifest intent。
- 失败/取消终态事务后立即崩溃，Repair 仍能补写 Manifest。

## 9. 并发与 race 测试

- 并行 Provider 解析使用同一取消树。
- 必要链失败后其他任务退出。
- PreparedTurn 在 ReAct 多轮中只读复用。
- 派生 Token 缓存并发安全。
- Manifest 不发生并发写竞态。
- 同一 Run 的重复 Finalize 由幂等键收敛。
- 主结果与 durable intents 的事务边界经过故障注入验证。
- Summary CAS 冲突不会覆盖更新版本。
- 并发 Memory 写回不会生成重复 active fact。
- 对相关包运行 `go test -race`。

## 10. 质量和性能验收

- 测试模型矩阵中不出现 Context Window 超限请求。
- 快速路径不调用远程精确计数和摘要模型。
- Search 上下文中不存在会话历史或完整用户记忆。
- Manifest 扫描不发现用户正文或凭据。
- 同一输入和版本产生确定性的 Context HMAC。
- Shadow 样本达到稳定后，关键历史约束召回不低于 Legacy。
- Token 使用率、Provider 延迟和降级率可通过指标观察。
- 可按 Run 定位 Assistant、Summary、Memory 和 Manifest 的写回状态。
- 进程崩溃不会让已提交 Assistant 永久失去可重试的摘要/记忆任务。
- Run 只有在主结果与必要 durable intents 提交后才进入 completed。
- 只有相关包测试、集成测试和 race 测试通过后才能把模式切到 `enabled`。
