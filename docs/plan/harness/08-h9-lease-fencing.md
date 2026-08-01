# H9：Lease、Fencing、故障接管与 NATS 条件接入实施设计

## 状态、范围与前置

- 状态：草案；依赖 H7、H8。仅在需要多实例 Worker 或运行数据证明 MySQL dispatcher 不足时实施。
- 分支：`feature/harness-lease-fencing`。

本工作流把“能被多个 worker 运行”变成“旧执行者不能覆盖新执行者”。NATS JetStream 不是前置条件：先完成 MySQL lease/fencing；只有明确的吞吐、跨进程唤醒或可用性门槛触发后，才增加 Outbox/NATS adapter。

## Lease 与 fencing

Run claim 分配单调递增 FencingToken，持久化为 `agent_runs.fencing_token` 与 Attempt 记录。Worker 在 MySQL 保存 lease mirror（owner、expires_at、heartbeat），Redis 只可做低延迟 heartbeat/通知。所有 Run、Attempt、Step、Checkpoint、Finalization、Event 和 Journal 的写入都携带当前 token 与 StateVersion；条件更新不匹配即拒绝。

Heartbeat 以租期的固定分数续约，并在连续失败时主动停止执行；不能因为本地仍活着就继续写。Recovery Scanner 只处理 lease 已过期的 Run：有兼容 checkpoint 则新 Attempt Resume；无 checkpoint且所有已完成 Step 可证明幂等时按输入重跑并复用 artifact；存在 `side_effect_unknown` 时转 suspended，要求人工决策。

## Outbox 与 NATS 的启用门槛

只有满足以下至少一项且 MySQL dispatcher 已观测为瓶颈，才引入 NATS：跨实例唤醒延迟无法满足 SLO、轮询数据库成本不可接受、需要 durable fan-out 事件消费者，或 Worker 数量/队列吞吐超过既定数据库容量。引入时新增：

- `agent_outbox`：稳定 event ID、aggregate、类型、payload/version、trace context、发布状态与重试次数；与 Run 状态事务同写。
- `consumer_inbox`：`UNIQUE(event_id, consumer_name)`，保证 consumer 幂等。
- JetStream versioned stream、durable consumer、DLQ 和明确保留期。

Outbox Relay 可重复发布；NATS 可重复投递；消费者必须通过 Inbox、Run CAS 和 fencing 去重。NATS 不替代 MySQL 的 Run、Command 或审计事实。SSE 可以订阅 NATS 加速，但断线重放仍读取 MySQL EventStore。

## 部署、回滚与演练

滚动发布前 worker 进入 drain：停止新 claim，持续 heartbeat，完成当前 Attempt 或触发 H7 安全暂停。硬终止依赖 Lease 过期接管。回滚先停止新版本 claim，再等旧 lease 接管策略生效；绝不手工降低 fencing token 或把 running Run 直接改 queued。

Redis 故障时回退 MySQL lease mirror；NATS 故障时 Outbox 累积并由 Relay 重试；MySQL 不可用时拒绝 claim/状态写入，Worker 必须停而不是猜测成功。

## 验证与退出条件

- 故障注入：worker kill -9、zombie worker 复活、网络分区、heartbeat 丢失、Redis/NATS 短暂不可用、Relay 在发布前后崩溃、重复消息。
- 断言：每个有效状态/结果只由当前 token 写入；旧 worker 所有迟到 Step、Checkpoint、Finalization、Event 均失败；有未知副作用的 Run 不自动重跑。
- 容量测试：多 worker claim 无重复执行，lease/recovery 在目标 SLO 内，Outbox 不无限积压。

退出条件：多实例故障接管具备可审计证据，重复派发不会产生重复有效执行，且引入 NATS 后正确性仍只依赖 MySQL 事实与 CAS。
