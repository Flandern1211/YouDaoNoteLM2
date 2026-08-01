# H8：MySQL Dispatcher、Worker 与背压实施设计

## 状态、范围与前置

- 状态：草案；依赖 H1–H6；H6 的持久化 Cancel 命令是 claim 前检查的硬前置，H7 的 Pause/Resume 可随后接入同一调度模型。
- 分支：`feature/harness-worker-dispatcher`。

本工作流将 Run 生命周期从 HTTP 进程解耦。首期不强制 NATS：采用 MySQL 可恢复队列和轮询 Dispatcher，代码仍为模块化单体，可按 `all`、`api`、`worker` 三种角色启动。

## 队列与领取模型

`agent_runs.state=queued` 与 `next_retry_at` 是唯一待执行事实。Dispatcher 以短轮询扫描到期 queued Run，并调用 RunStore `Claim`；若 MySQL 版本/隔离级别支持，使用 `SELECT ... FOR UPDATE SKIP LOCKED`，否则使用带 state/version 的条件更新。无论实现手段如何，只有一个 worker 可以把同一 Run 变为 running 并创建 Attempt。

初期不另建重复的 queue 表。可选 `agent_dispatch_wakeups` 只能是加速通知，丢失后必须由轮询发现；不得把可靠性放在 channel、Redis 或内存队列。Dispatcher 领取后把 Run 交给 H3 Supervisor，Supervisor 返回后负责 Finalization 或按状态重新排队。

## 运行角色与配置

- `all`：API、Dispatcher、Worker 同进程；开发和渐进迁移默认值。
- `api`：只执行 Admission、Run 查询与 SSE，不领取 Run。
- `worker`：只扫描/领取/执行/Repair，不暴露用户 API。

启动配置必须校验至少一个可执行 worker 角色，且同一进程的 worker 数、claim batch、poll interval、global/user/conversation 并发上限均有明确默认值。API 请求的 context 只用于 Admission；Worker 为每个 Attempt 新建可控 run context。

## 背压与顺序

Dispatcher 必须同时限制：全局 active Attempt、每用户并发、每 conversation 一条 active 顶层 Run、每 provider 的并发槽。未取得槽位的 Run 留在 queued，不能因为短暂拥塞标为失败。重试 Run 用 `next_retry_at` 退避，避免热循环；每次 claim 前必须读取 H6 的 Cancel intent/`desired_state=cancelled` 并拒绝执行。H7 启用后，再以同一检查点处理 Pause/Resume intent；H8 基线不声称支持尚未上线的 Pause。

conversation 内依据 Admission sequence 领取：队首活动/非终态 Run 存在时，后续 Run 不得越过执行。不同 conversation 可并发，Search 子 Step 使用父 Run 预算，不与顶层 conversation 排序混淆。

## 崩溃语义、迁移与回滚

本阶段只承诺“API 或连接断开不丢 queued Run”；单 worker 硬崩溃的 running Run 在 H9 Lease/Recovery 完成前不能假称自动接管。启动时扫描遗留 running Run 应保守标记为 `suspended` 或按已验证 checkpoint 规则处理，而非直接重新执行。

迁移顺序：先在 `all` 启动 Dispatcher，确认新 Admission 不再同步执行；再启用独立 worker；最后让 API 不持有 Agent builder。回滚停止 Dispatcher 新 claim，但不删除 queued Run；恢复后可继续领取。H9 之前禁止扩容为多个无 Lease 的 worker 实例。

## 验证与退出条件

- MySQL 集成：多个 worker 并发 claim、进程重启后 queued Run 继续、retry 到期、Cancel 队列项不执行、conversation FIFO；H7 启用后补充 Pause/Resume 队列项测试。
- 角色测试：api 不执行 Agent；worker 不暴露 HTTP；all 兼容当前本地运行。
- 背压测试：每种上限达到时 Run 留 queued，释放槽后执行；数据库连接池不被轮询耗尽。
- SSE 回归：API 重启和 SSE 断线不改变已接受/执行的 Run。

退出条件：Run 可在不依附原 HTTP 请求的情况下被可靠领取，API 与 Worker 可独立部署为不同角色，且没有多 worker 无 fencing 的错误扩容路径。
