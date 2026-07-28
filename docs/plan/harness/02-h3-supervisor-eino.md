# H3：Execution Supervisor、Eino 与 Step Gateway 实施设计

## 状态、范围与前置

- 状态：草案；依赖 H1、H2。
- 分支：`feature/harness-supervisor-eino`。

本工作流实现一个 Attempt 的唯一执行所有者。它从 queued Run 取得执行权，按冻结快照构建 Chat/Main/Search Agent，调用 Eino，并将关键边界翻译为 Run、Attempt、Step 与结构化 Outcome。它不实现后台 Worker 进程拆分、Checkpoint Resume 或多实例 Lease。

## 核心语义

`ExecutionSupervisor.Execute(ctx, runID, workerID)` 只接受已经 Claim 的 Run/Attempt；它创建独立 `runCtx`，不使用 HTTP request context 作为父取消源。所有该 Attempt 创建的 goroutine、迭代器与 channel 都必须在结果 drain、取消或有界 deadline 后退出。

执行顺序：

1. RunStore CAS 将 `queued → running`，创建 Attempt、Authority 和 `run.started` 事件。
2. Supervisor 加载 Run、输入引用、VersionSnapshot 与预算，校验 Agent/Prompt/工具/模型版本可解析。
3. 通过 Context integration adapter 生成 PreparedTurn；ContextCompiler 仍只负责模型可见上下文，不能读写 RunStore。
4. 使用 Eino Runner 或当前受控适配器执行，完整消费事件流；panic 在最外层恢复为 `permanent` 错误，再进入 Finalization。
5. 将 Search、带副作用工具、可独立计费/重试工具转换为 Step；普通模型调用仅产生 OTel span。
6. Outcome 成功、取消或失败后，先以 CAS 转入 `finalizing`，再调用下方已冻结的 FinalizationPort；禁止 Service 直接写 assistant message。

## Finalization 端口契约

H3 拥有“何时结束执行、并交付什么 Outcome”的调用契约；H4 拥有 Journal、写回事务和终态 CAS 的实现。这使 H3 可先用 fake Finalizer 验证执行闭环，而 H4 只需实现既定端口，不反向改变 Supervisor 语义：

```go
type FinalizationPort interface {
    Finalize(ctx context.Context, req FinalizationRequest) (FinalizeResult, error)
}

type FinalizationRequest struct {
    RunID                  RunID
    Authority              ExecutionAuthority
    FinalizingStateVersion StateVersion // running -> finalizing 成功后的版本
    Revision               Revision
    Outcome                Outcome
}
```

调用前提是 H3 已成功写入 `running → finalizing`，且 `Outcome` 已冻结为成功、失败或取消。`FinalizationPort` 必须使用 `RunID`、Authority 和 `FinalizingStateVersion` 检查该事实；它在必需 Journal/主写回完成后负责唯一的 `finalizing → terminal` CAS。H3 不假定事务边界、writer 顺序或 Repair 策略；H4 不得重新执行 Eino 或改变已冻结 Outcome。

## Step Gateway

Gateway 是 Search、Youdao 单次操作和外部领域任务引用的统一边界，输入包含 Run/Attempt/Parent Step、稳定 input hash、权限、子预算与 deadline。它在调用前持久化 running Step；调用成功写 artifact reference 和 succeeded；失败写 ErrorClass/Code。Generation/Importer/ASR 不迁入 Harness，只记录其既有 job ID 为 `external_job_ref`。

Main → Search 只传结构化 `SearchTask`，不传完整 Chat 历史或用户记忆。父 Agent 必须消费可恢复子 Agent 结果时才使用 Eino AgentTool；现有异步搜索面板可先由 Step adapter 包装。

## 错误与统一重试预算

Eino 保有单个模型调用的 `ModelRetryConfig`；Harness 负责 Step、Attempt 与 Run 级总预算。每次模型重试、Step retry 或重新排队都扣减同一 `retry_units`，不得形成模型重试 × Step 重试 × Run 重试的乘法放大。

错误必须映射到 G0 的完整 `ErrorClass`：`permanent`、`transient`、`rate_limited`、`timeout`、`cancelled`、`resource_exhausted`、`invalid_input`、`permission`、`dependency_permanent`、`worker_lost`、`checkpoint_incompatible`、`side_effect_unknown`。只有 `transient`、`rate_limited` 或 `timeout`，且步骤已证明无副作用/可重试并仍有预算时，才可进入 `retry_wait`；`side_effect_unknown` 必须进入 `suspended`，`worker_lost` 交由 H9 的恢复规则裁定，其他类别默认不自动重跑。ErrorCode 面向客户端稳定，原始 provider 错误只写受控日志。

## 数据、配置与回滚

新增 Run Budget snapshot（wall time、迭代、模型/工具/Search 调用、token、费用、retry units 的上限与初值），实际用量由 Supervisor/Step Gateway 扣减。预算耗尽生成 `resource_exhausted` Outcome。

本分支只在单进程 `all` 模式被 Admission 的本地 dispatcher 调用；H8 会替换为 durable dispatcher，不改变 Supervisor 接口。Feature flag 回滚只停止新 Run 调用 Supervisor；正在运行的 Attempt 使用其冻结快照完成或经 H6 取消。

## 验证与退出条件

- fake Eino Agent 测试：成功、流式、ToolCall、Search Step、panic、deadline、取消、错误映射和完整 drain。
- 预算测试：模型/Step/Run 重试共享计数；超过任一资源上限不再调用 provider。
- Store 集成：claim 只产生一个 Attempt；旧 Authority 不能写 Step/Outcome；失败/取消也进入 Finalization。
- 回归：Chat/Main/Search 使用正确 ContextProfile；Search 无法读取 Chat 历史。

退出条件：任何 Attempt 只有一个执行 owner，关键副作用都可定位到 Step，且 Eino 失败路径不会绕过终态处理。
