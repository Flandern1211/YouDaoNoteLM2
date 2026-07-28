# H2：Admission、幂等与 Chat API 兼容实施设计

## 状态、范围与前置

- 状态：草案；仅在 H1 及项目级架构获批后实施。
- 分支：`feature/harness-admission-api`。
- 依赖：H1 Kernel/RunStore；现有 Conversation、Message 所有权校验。

本工作流把现有 Chat API 变成兼容适配器：它仍可保持现有响应格式，但不再自行保存用户消息并直接调用 Agent。它改为调用 `RunService.Accept`，在一个 MySQL 事务中接受用户意图。Worker、SSE 重放和 Pause/Resume 不属于本分支。

## 目标与不变量

成功标准是：同一次客户端意图最多创建一个用户消息和一个 Run；同一 conversation 的顶层 Run 顺序可解释；HTTP/SSE 请求不是 Run 的所有者。

- 先校验 user 对 conversation/notebook/source 的所有权，再创建记录。
- 用户消息、`agent_runs` 的 `queued` 状态、版本快照和首条 `run.accepted` 语义事件在同一事务提交。
- 相同 `(user_id, idempotency_key)` 重试返回同一 Run 与入口消息，绝不创建第二条消息。
- 同一 conversation 默认只允许一个活动顶层 Run；新输入写入持久化队列顺序，不能绕开并发规则。
- 请求取消、SSE 断开或 API 进程退出不得把已提交 Run 误标为取消。

## 接口与数据变更

`run.AdmissionService` 由应用层调用，输入为 `AcceptRequest`：用户/会话/笔记本作用域、Agent 类型、受控 InputRef、Source IDs、幂等键和经过校验的 VersionSnapshot。返回 `AcceptedRun`：Run ID、入口 Message ID、状态、排队位置（如可计算）与是否来自幂等重放。

新增或启用：

- `agent_runs.idempotency_key varchar(191) not null` 与 `UNIQUE(user_id, idempotency_key)`；客户端未提供时由 API 为一次请求生成并回显，不能用消息正文 hash 伪造幂等键。
- `agent_runs.sequence bigint not null`；以 conversation 内单调序号表达提交顺序，建立 `UNIQUE(conversation_id, sequence)`。
- `agent_run_events` 的规范事件信封在 H2 创建并首次使用：`run_id`、`sequence`、`event_id`、可空 `attempt_id`、可空 `step_id`、`type`、`payload_version`、脱敏 `payload_json`、`created_at`；约束为 `UNIQUE(run_id, sequence)`、全局 `UNIQUE(event_id)`，并建立 `(run_id, sequence)` 索引。Admission 在同一事务插入 sequence 为 1 的 `run.accepted`；H5 只能复用此 schema 增加查询、重放和 SSE 投影，不得重定义事件格式或 sequence 语义。

`queued` Run 没有 Authority 或 Attempt。只有 H3/H8 的 claim 才会创建 Authority；Admission 不得预先分配 fencing token。

## 处理流程

1. HTTP adapter 校验认证、请求大小、Conversation/Notebook/Source 所有权与 Agent 输入类型。
2. 开启事务；按 `(user_id, idempotency_key)` 查询既有 Run。命中时验证请求的不可变字段相同，返回既有结果；不同时返回稳定的幂等冲突错误。
3. 锁定 conversation 的提交序号，写入口 `Message(role=user)`，构造 `InputRef(chat_message, message_id, SHA-256)`。
4. 计算并冻结 VersionSnapshot，插入 `queued` Run 与 `run.accepted` Event；存在活动 Run 时保留 queued，不直接启动。
5. 提交后调用本地唤醒接口；唤醒失败不回滚 Run，H8 的轮询 Dispatcher 必须能补领。

首期活动状态的定义为 `queued`、`running`、`finalizing`、`retry_wait`、`pause_requested`、`pausing`、`paused`、`cancel_requested`。H2 只产生 `queued`，并将其余状态视为未来 H3–H7 产生的数据而占用活动槽；H2 不暴露 Pause/Resume/Cancel HTTP 能力。终态不占用 conversation 活动槽。排队 Run 的历史边界由其入口消息序号确定，执行时不得看到后续输入。

## 迁移、回滚与兼容

先以 feature flag 只让受控用户走 Admission；未启用用户保留 Legacy Chat 路径。切换时 Context integration adapter 使用通用 Run ID，禁止同时创建 `agent_context_runs` 作为第二份事实源。旧 API 可以临时同步等待 H3 的结果，但内部仍只接受一个 Run。

回滚关闭新 Admission，只影响尚未提交的新请求；已接受 Run 由 Harness 路径继续完成或由明确的恢复策略处理。不得删除入口消息或 queued Run。前端改造为显式 Run API 不是本分支前置。

## 验证与退出条件

- 单元测试：重复 key、冲突 key、无 key、非法输入、未授权作用域、VersionSnapshot 冻结、conversation 顺序。
- MySQL 集成：并发相同 key 仅一条消息/Run；并发不同 key 的 sequence 无冲突；事务任一点失败无孤儿消息或 Run。
- 兼容测试：既有 Chat 请求和流式请求仍获得可识别的接受/失败响应，SSE 断开不改变已提交状态。

退出条件：重复创建不产生重复有效意图；入口消息与 Run 不再分离提交；没有 HTTP context 被传递为 Run 的根生命周期。
