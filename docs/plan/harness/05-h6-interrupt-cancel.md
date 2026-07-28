# H6：持久化 Interrupt 与 Cancel 实施设计

## 状态、范围与前置

- 状态：草案；依赖 H1–H4。
- 分支：`feature/harness-interrupt-cancel`。

Checkpoint 上线前，“停止生成”定义为永久 Cancel，不承诺恢复。这个工作流先将取消意图持久化，逐步替代 `chatAgentService` 的进程内 `sync.Map` cancel 函数；Pause/Resume 留给 H7。

## Command 与状态语义

新增 `agent_run_commands`：命令 ID、Run ID、类型、actor、idempotency key、请求时 Authority/StateVersion、状态、受控原因 code、创建/确认时间。`UNIQUE(run_id, idempotency_key)` 保证客户端重复点击只产生一条有效 Cancel Command。

`POST /runs/:id/cancel` 校验所有权后，在同一事务写 Command、设置 `desired_state=cancelled`，并将 `queued/running/paused` 转为 `cancel_requested`（具体转换由 H1 状态表校验）。API 返回“取消已接受”，而不是“已取消”。queued Run 可无需调用 Eino 直接由 dispatcher/finalizer 完成 cancelled；running Run 由 Supervisor 观察意图后调用现有 Eino `WithCancel`，并持续 drain 直到可识别 `CancelError` 或执行结束。若 Run 已在 `finalizing`，执行 Outcome 已冻结：Command 以 `superseded` 状态仅作审计，`desired_state` 和 Run state 均不改变，API 返回稳定的 `cancel_too_late`；Finalizer 仍按原 Outcome 推进到对应终态。

Cancel 胜过 Retry：一旦 desired state 为 cancelled，任何 `retry_wait → queued` 或新的 claim 必须拒绝。Cancel 与成功竞争时以首先成功的状态 CAS 为准；若 Finalization 已提交 terminal success，Cancel 返回该 Run 已终态，不撤销用户已收到的回答。

## 运行时传播

Supervisor 在模型调用、工具边界和 heartbeat 前读取持久化 desired state；本地 wakeup 可以更快通知，但丢失通知不影响正确性。取消使用独立的有界 drain deadline；超时后记录受控错误并进入 Finalization，不能永远占用 Attempt。

Search/Youdao 等 Step 必须继承 Attempt 的 cancel context。禁止为属于当前 Run 的工作使用 `context.WithoutCancel`；确需脱离的领域 Job 只能以 external job ref 表达，并由自身取消协议处理。

## 迁移与回滚

先保留旧 `/stop` endpoint，内部转换为 Cancel Command；当所有 Chat/Main 请求已走 Run 后删除 `cancelFuncs` 的事实职责。旧同步客户端在收到“accepted”后通过 SSE/GetRun 观察终态。回滚时旧 endpoint 仍能接受请求，但已存在 Command 必须继续被 Supervisor/Scanner 尊重。

## 验证与退出条件

- queued、running、retry_wait、finalizing、terminal Run 的取消矩阵；重复 Command；无权限；Cancel/成功并发竞态。
- fake Eino 测试 recursive cancel、CancelError、drain timeout 和子 Step context 传播。
- 重启测试：写入 Cancel Command 后进程重启，重新领取 Run 仍被取消，不依赖旧内存函数。

退出条件：取消意图在 MySQL 可审计、可重放；SSE 断开不会取消 Run；进程内 map 不再是取消正确性的唯一来源。
