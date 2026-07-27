# Token 预算与 Eino 集成

## 1. Eino 接入点

项目当前使用 `github.com/cloudwego/eino v0.9.4`。该版本的 `GenModelInput` 在进入 ReAct 前把 Agent Instruction 和初始消息组合成模型消息；`BeforeModelRewriteState` 在每次模型调用前获得：

```text
state.Messages
state.ToolInfos
state.DeferredToolInfos
```

W0 静态源码核对已经确认以上字段和改写持久语义，但 Legacy 行为测试、Handler 顺序测试和多轮 ReAct 契约测试尚未实现，因此 W0 状态仍是“进行中”。

当前 W0–W4 接入流程为：

```text
chatAgentService 接受当前请求
→ Legacy/Shadow Turn Adapter
→ ContextCompiler.PrepareTurn
→ PreparedTurn.MessagePlan
→ AgentInputBuilder 生成初始消息
→ Eino GenModelInput 注入唯一系统 Instruction
→ ReAct
→ ContextMiddleware.BeforeModelRewriteState
→ ContextCompiler.CompileModelInput
→ 返回修改后的 state.Messages
→ 收集无正文 CompileRecord/Manifest
```

W5–W7 在未来 Harness 可用后的目标流程为：

```text
RunService.AcceptTurn
→ Worker/Harness ClaimRun
→ TurnLifecycleCoordinator.BeginTurn
→ ContextCompiler.PrepareTurn / CompileModelInput
→ Eino 产生终态
→ Harness 调用 TurnLifecycleCoordinator.FinalizeTurn
```

第一段可以独立实现；第二段依赖当前尚不存在的 RunService、Worker、Authority、Outbox 和持久化 Harness，不能用当前 `chatAgentService` 伪造生产保证。

ContextMiddleware 使用接口式 `ChatModelAgentMiddleware`，不使用已标记为兼容路径的旧 struct Middleware。

## 2. Handler 顺序

建议顺序：

```text
1. 上下文注入幂等检查
2. ToolReduction
3. BudgetCompiler
4. 必要时 Summarization
5. Metrics/Manifest
```

最终实现计划必须通过 Eino 契约测试确认 Handler 注册顺序与实际调用顺序一致。任一 Handler 修改消息后，后续 Handler 必须读取修改后的状态。

上下文注入必须幂等，不能在每次模型调用前重复追加记忆、摘要或当前输入。

`AfterAgent` 只在 Eino Agent 成功终止后调用；模型错误、取消、超出迭代等路径不会调用。它可以做成功路径的轻量后处理或事件采集，但不能作为助手消息、终态 Manifest 或可靠写回调度的唯一入口。

## 3. 输入预算

```text
InputBudget
= ContextWindow
- ReservedOutputTokens
- SafetyMargin
```

初始安全余量：

```text
max(512 tokens, ContextWindow 的 5%)
```

如果 TokenCounter 只能近似计数，Profile 可以提高安全余量，不能降低。

ReservedOutputTokens 来自版本化模型能力或显式模型配置。未知模型在生产环境中拒绝运行。

## 4. 硬保留项

- 系统提示词和安全规则。
- 当前用户输入或 SearchTask。
- 当前活跃的 ToolCall/ToolResult 协议组。
- 本次 Agent 可见工具定义。
- 输出 Token 预留。

硬保留项本身超限时立即返回：

- 用户输入造成：`HardBudgetExceeded`，提示用户缩短输入。
- 工具 Schema 造成：Agent/Profile 配置错误。
- 活跃工具结果造成：`ToolContextOverflow`。

不静默截断系统规则、用户当前请求或工具 Schema。

## 5. 软上下文上限

首期默认值以 InputBudget 为基准：

- 用户记忆总量最多 5%。
- 会话摘要最多 10%。
- 其余可用空间由最近历史和动态工具结果竞争。

Profile 可以设置更小上限。扩大上限需要新 Profile 版本和回归评测。

不设置固定最小比例。某类内容为空时，空间自动由其他内容使用。

## 6. 快速路径和水位

首期默认水位：

```text
精确计数阈值：InputBudget 的 70%
治理高水位：InputBudget 的 80%
治理目标低水位：InputBudget 的 60%
```

执行逻辑：

```text
估算值 < 70%
  → 直接调用模型

估算值 >= 70%
  → 尝试 Provider 支持的最高可信计数

Provider 精确值、本地兼容值或保守估算 < 80%
  → 调用模型

达到 80%
  → 分阶段治理，直到不高于 60%
```

水位是 Profile 版本的一部分。调整必须通过 Shadow 数据验证。

## 7. Token 计数性能

- Prompt、工具 Schema 和稳定 ContextItem 缓存 Token 数。
- 动态消息只计算新增部分。
- Anthropic 热路径先保守估算，接近阈值时调用现有 SDK 的 `Messages.CountTokens`，结果标记为 `exact_provider`。
- 已知 OpenAI 模型由版本化模型注册表显式选择 `o200k_base` 或 `cl100k_base` 的本地兼容实现，结果标记为 `compatible_local`。
- 自定义 OpenAI-compatible 模型必须显式配置编码和 ContextWindow；未配置时生产环境拒绝，不能按模型名猜测。
- 无匹配 tokenizer 的开发回退使用 `conservative_utf8_bytes`：UTF-8 正文字节数加消息、角色和工具结构开销。
- 中文使用匹配 tokenizer；没有匹配时按 UTF-8 字节保守估算，不使用“字符数除以四”。
- Provider 返回真实 usage 只作为后续偏差监测和校准基线，不能替代调用前预算。
- Provider 精确计数失败按自己的解析链回退到本地兼容或保守估算；所有允许的计数方式均不可用时中止。

具体 Go tokenizer 依赖必须在 W3 通过中文、特殊 Token、消息封装和工具 Schema 契约测试后才能引入。编码匹配不等于完整请求永久精确，Manifest 必须记录 CounterMode、编码和策略版本。

## 8. 治理顺序

```text
1. 清理已完成且过期的旧工具结果
2. 淘汰低相关、低重要性的普通用户记忆
3. 淘汰最旧的未摘要历史消息组
4. 使用已有会话摘要覆盖旧历史
5. 仍超限时触发 Eino 运行内 Summarization
```

每一步完成后重新计数，达到低水位就停止。

候选选择可以按优先级计算，但最终消息必须恢复原始时间顺序。

## 9. 工具结果治理

- ToolCall 与对应结果作为原子组验证。
- 活跃工具调用组不可部分删除。
- 旧工具结果清理必须保留明确占位，说明结果已因上下文治理移除。
- 单个工具结果超限时，优先使用工具 Runtime 提供的原始结果引用和摘要。
- ContextCompiler 不负责保存完整工具结果。
- 没有可恢复引用且无法安全缩减时返回 `ToolContextOverflow`。

RAG 模块的完整检索数据继续由 RAG/引用系统管理；发送给模型的格式化结果应保持有界。

## 10. 运行内摘要与会话摘要

两类摘要不能混淆：

- 会话摘要：由会话模块持久化，带 `ThroughMessageID` 和版本，下轮读取。
- 运行内摘要：Eino Middleware 为当前 ReAct Run 控制窗口，只修改当前 Agent 状态。

ContextCompiler 不把运行内摘要直接覆盖会话摘要。未来 `FinalizeTurn` 可以把它作为带来源和覆盖边界的候选交给 SummaryWriter；SummaryWriter 仍需根据已提交消息边界、当前摘要版本和 CAS 规则决定是否更新。

## 11. 终态与可靠写回

本节属于 W5–W7 的未来 Harness 集成契约，不是当前代码的执行流程。W0–W4 的 Shadow 接入只生成编译记录和模拟写回计划，不调用这些 Writer。

Harness 必须在 Eino 返回成功、错误或取消等终态后统一构造 `TurnOutcome`，附带全部 `CompileRecord` 并调用 `FinalizeTurn`。可恢复暂停只保存 checkpoint 和当前 Agent State，不执行终态 Finalize；Resume 后继续同一逻辑 Turn：

```text
Eino final answer
  → Harness: running → finalizing + FinalizationTicket
  → FinalizeTurn(success)
  → AssistantMessageWriter 幂等提交
  → Summary/Memory 可靠调度
  → Harness: finalizing → completed
  → Manifest

Eino error / cancellation
  → FinalizeTurn(non-success)
  → 不提取成功回答记忆
  → 记录终态 Manifest
```

运行中的 ToolCall/ToolResult、Eino State 和 checkpoint 继续在 Agent Runtime/Harness 的 step 边界管理。`FinalizeTurn` 不是唯一状态持久化点，也不重放工具副作用。

`PreparedTurnSnapshot`、已经产生的 `CompileRecord` 和运行内摘要候选必须注册为 Eino 可 checkpoint 的类型，或存入 Harness 管理的受控 checkpoint payload。Resume 优先恢复该快照，不重新调用 History/Memory Provider；只有首次模型调用前尚未产生 checkpoint 时，才按 Run 中固化的 Profile、Prompt、模型和 ContextMode 版本重新准备。

## 12. Prompt Cache

- 工具定义顺序稳定。
- 系统 Prompt 内容由版本控制，运行中不变化。
- 动态记忆、历史和工具结果位于稳定前缀之后。
- Profile 和工具集变化必须改变对应版本和派生缓存键。
- ContextCompiler 不为了节省少量 Token 而频繁重排稳定工具集合。
