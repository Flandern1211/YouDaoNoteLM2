# H5：语义事件日志与 SSE 重放实施设计

## 状态、范围与前置

- 状态：草案；依赖 H1–H4；H2 已创建并冻结 `agent_run_events` 事件信封，建议在单进程闭环稳定后实施。
- 分支：`feature/harness-events-sse`。

本工作流将“连接上的 token 流”与“可恢复的 Run 事实”分开。持久化的是状态、Attempt、关键 Step、完成和错误等语义事件；逐 token 数据只允许短期传递，不能成为重新构建 Run 状态的依据。

## 事件模型

复用 H2 创建的 `agent_run_events`：`run_id`、`sequence`、`event_id`、可空 `attempt_id`、可空 `step_id`、`type`、`payload_version`、脱敏 `payload_json`、`created_at`。约束为 `UNIQUE(run_id, sequence)` 与全局 `UNIQUE(event_id)`；按 `(run_id, sequence)` 建索引。H5 不修改该持久化信封或 `run.accepted` 的 sequence=1 约定；后续 EventStore 和 SSE 都以它为唯一 schema 契约。

首批 event type：

- `run.accepted`、`run.queued`、`run.claimed`、`run.state_changed`；
- `attempt.started`、`attempt.finished`；
- `step.started`、`step.finished`；
- `run.finalizing`、`run.completed`、`run.failed`、`run.cancelled`；
- `run.error`、`run.suspended`。

payload 仅包含稳定状态、受控错误 code、时间、artifact/message 引用和必要展示摘要；不得保存 Prompt、完整模型输出、网页正文、工具参数中的秘密或原始 provider 错误。最终 assistant 内容继续由现有 Message API 按权限读取。

## 写入与读取语义

状态转换与对应 Event 在同一数据库事务写入；不得先发 SSE 再写事实。每个 Run 在事务内分配递增 sequence，重复操作重试使用稳定 event ID，避免产生重复有效状态。Event 不是 Event Sourcing：Run 当前状态始终从 `agent_runs` 读取，Event 只用于审计和客户端投影。

新增查询端口：

```text
GetRun(user, runID) -> 当前状态、Attempt、终态引用
ListEvents(user, runID, afterSeq, limit) -> 严格递增的语义事件
```

所有读取先校验 Run 所有权。`after_seq` 为排他游标；空游标从保留期内的最早事件开始。若游标早于保留窗口，API 返回明确的 `events_expired`，客户端改为查询当前 Run，而不是伪造完整回放。

## SSE 兼容与生命周期

既有 SSE endpoint 可保留路径，但内部先回放 `after_seq` 之后的持久化事件，再订阅本进程 notifier。连接只负责 detach：客户端断开、网络错误和浏览器 Abort 不调用 Agent cancel。H8 后 notifier 可替换为 MySQL polling/通知；H9 后可加 NATS transport，均不改变 EventStore 或 cursor 语义。

逐 token 事件若继续提供，必须带 `ephemeral=true`、不占持久 sequence，并在重新连接时明确不可回放；客户端不得把它当作完成判定。完成判定只使用 terminal `run.*` Event 或 GetRun。

## 保留、回滚与验证

语义 Event 保留期由配置控制，清理任务按已终态和时间删除；审计与 Run 的保留策略独立。切换期间 Legacy 流保持兼容，但不得为同一 Run 同时生成两套 sequence。回滚仅关闭 SSE 新投影，Event 事实继续保存。

- Store 测试：并发状态转换 sequence 无重复/空洞；失败事务没有孤立 Event；重复尝试不重复 terminal Event。
- HTTP/SSE 测试：断线后以 `after_seq` 精确重放；权限隔离；过期游标；先 replay 再 live 无丢失或逆序。
- 回归：SSE 断开时 Run 继续运行，完成状态可从 GetRun 查询。

退出条件：任何 terminal Run 均能按 Run ID 查询状态并在保留期内重放有序语义事件，且流连接不再拥有 Run 生命周期。
