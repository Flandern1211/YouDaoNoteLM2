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
- 新模块失败不影响旧请求。
- 比较消息 ID、摘要游标、Token、裁剪原因、Provider 状态和 HMAC。
- 不比较或记录正文。

采样按 userID 做稳定分桶，保证同一用户不会在请求间随机切换。

### 5.3 Enabled

新 ContextManager 生成正式输入。旧实现保留为快速回滚路径，直到灰度和稳定期结束。

## 6. Feature Flags

```text
context_manager_mode = legacy | shadow | enabled
context_shadow_sample_rate
context_memory_enabled
context_exact_count_enabled
```

Feature Flag 不允许改变安全边界。关闭精确计数时必须使用保守估算和更大安全余量。

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

### 7.6 Manifest

- 不包含候选或最终正文。
- 正确记录 retry、fallback、skip 和裁剪原因。
- 同一规范化输入产生相同 HMAC。
- 内容变化产生不同 HMAC。

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

## 9. 并发与 race 测试

- 并行 Provider 解析使用同一取消树。
- 必要链失败后其他任务退出。
- TurnContext 在 ReAct 多轮中只读复用。
- 派生 Token 缓存并发安全。
- Manifest 不发生并发写竞态。
- 对相关包运行 `go test -race`。

## 10. 质量和性能验收

- 测试模型矩阵中不出现 Context Window 超限请求。
- 快速路径不调用远程精确计数和摘要模型。
- Search 上下文中不存在会话历史或完整用户记忆。
- Manifest 扫描不发现用户正文或凭据。
- 同一输入和版本产生确定性的 Context HMAC。
- Shadow 样本达到稳定后，关键历史约束召回不低于 Legacy。
- Token 使用率、Provider 延迟和降级率可通过指标观察。
- 只有相关包测试、集成测试和 race 测试通过后才能把模式切到 `enabled`。
