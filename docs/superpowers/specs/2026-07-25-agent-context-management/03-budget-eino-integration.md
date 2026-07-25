# Token 预算与 Eino 集成

## 1. Eino 接入点

项目当前使用 `github.com/cloudwego/eino v0.9.4`。该版本的 `GenModelInput` 在进入 ReAct 前把 Agent Instruction 和初始消息组合成模型消息；`BeforeModelRewriteState` 在每次模型调用前获得：

```text
state.Messages
state.ToolInfos
state.DeferredToolInfos
```

因此接入流程为：

```text
PrepareTurn
→ TurnContext.MessagePlan
→ AgentInputBuilder 生成初始消息
→ Eino GenModelInput 注入唯一系统 Instruction
→ ReAct
→ ContextMiddleware.BeforeModelRewriteState
→ Manager.CompileModelInput
→ 返回修改后的 state.Messages
```

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
  → 尝试精确计数

精确值或保守估算 < 80%
  → 调用模型

达到 80%
  → 分阶段治理，直到不高于 60%
```

水位是 Profile 版本的一部分。调整必须通过 Shadow 数据验证。

## 7. Token 计数性能

- Prompt、工具 Schema 和稳定 ContextItem 缓存 Token 数。
- 动态消息只计算新增部分。
- Provider 返回真实 usage 时可作为后续估算基线。
- 热路径优先本地近似计数。
- 只有接近阈值时才调用远程精确计数接口。
- 精确计数失败按自己的解析链回退到保守估算；所有计数方式均不可用时中止。

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
- ContextManager 不负责保存完整工具结果。
- 没有可恢复引用且无法安全缩减时返回 `ToolContextOverflow`。

RAG 模块的完整检索数据继续由 RAG/引用系统管理；发送给模型的格式化结果应保持有界。

## 10. 运行内摘要与会话摘要

两类摘要不能混淆：

- 会话摘要：由会话模块持久化，带 `ThroughMessageID` 和版本，下轮读取。
- 运行内摘要：Eino Middleware 为当前 ReAct Run 控制窗口，只修改当前 Agent 状态。

ContextManager 不把运行内摘要直接写回会话摘要。是否将其作为会话模块的摘要输入由后续写回流程决定。

## 11. Prompt Cache

- 工具定义顺序稳定。
- 系统 Prompt 内容由版本控制，运行中不变化。
- 动态记忆、历史和工具结果位于稳定前缀之后。
- Profile 和工具集变化必须改变对应版本和派生缓存键。
- ContextManager 不为了节省少量 Token 而频繁重排稳定工具集合。
