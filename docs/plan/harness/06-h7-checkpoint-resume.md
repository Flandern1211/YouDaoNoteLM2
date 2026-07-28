# H7：Checkpoint、Pause 与 Resume 实施设计

## 状态、范围与前置

- 状态：草案；依赖 H1 Kernel/RunStore、H3 Supervisor、H6 Command/Cancel；推荐在 H8 durable dispatcher 前先证明单实例恢复。
- 分支：`feature/harness-checkpoint-resume`。

本工作流使用 Eino v0.9.4 的 `CheckPointStore`、`WithCancel` 与 `Runner.Resume`，只在模型/工具安全点实现暂停。Harness 负责 metadata、兼容性、命令与审计，不复制 Eino checkpoint 编码或 ReAct 循环。

## Checkpoint 存储与版本

新增 `agent_checkpoints`，以逻辑 checkpoint key + 不可变 version 表示：Run/Attempt、version、storage kind、blob/MinIO URI、字节数、SHA-256、fencing token、VersionSnapshot hash、创建/过期时间与 current 指针。小于 1 MiB 的字节存 MySQL，大对象存 MinIO；两者的元数据和 current pointer 始终在 MySQL。

自定义 Eino Store 的 `Set(ctx, checkpointID, bytes)` 必须从强类型 Attempt context 取得 Run ID、Attempt ID、Authority。它先写不可变 version，再以 `(run_id, fencing_token, state_version)` CAS 切换 current；旧 Worker 即使写入迟到 blob，也不能覆盖 current pointer。`Get` 验证 checksum 和版本元数据；读不到或校验失败返回受控错误。

## Pause/Resume 流程

Pause API 写 `desired_state=paused` 和 Pause Command，状态仅到 `pause_requested`。Worker 观察命令后转 `pausing`，对 Eino 使用 recursive safe-point cancel，drain iterator，等待 checkpoint Set。只有 checkpoint 可读、checksum 完整、Authority 匹配且 VersionSnapshot 兼容时，才写 `paused`。否则进入 `suspended` 并记录可解释 code；绝不将“请求暂停”展示成“已暂停”。

Resume 只接受 `paused`，或经人工批准的 `suspended`。它比较 Eino major/minor、Agent 定义、Prompt、工具 schema、模型配置 hash 与 Context Profile；精确匹配直接允许，声明为 backward-compatible 的组合通过显式矩阵，其余拒绝并让用户选择从原输入创建新 Run。Resume API 在同一事务写 Resume Command、`desired_state=running`、已验证的 `pending_resume_checkpoint_ref`，并以 CAS 将 `paused`（或获批 `suspended`）转为 `queued`；它不创建 Attempt，也不分配 Authority。随后本地 dispatcher 或 H8 Dispatcher 通过唯一的 `Claim` 事务把 `queued → running`，消费该 pending 引用、创建新 Attempt/Authority，并以冻结快照调用 `Runner.Resume(checkpointKey)`。因此“恢复创建新 Attempt”是 Claim 的效果而非 API 的直接写入；Run ID 保持不变，排队与背压规则也一致适用。

## 取消、清理与回滚

Cancel 与 Pause 竞争时 Cancel 优先，已生成 checkpoint 不自动 Resume。checkpoint 到期只能清理非 current、无活动 Run 且超过保留期的版本；删除前检查 Run/Attempt 引用。feature flag 回滚只停止新 Pause/Resume 请求，已 paused Run 保留并可在重新启用后恢复，不能删 checkpoint 来“回滚”。

## 验证与退出条件

- 固定 Eino v0.9.4 契约：安全点 Cancel 产生可识别 CancelError，同版本 Runner 可 Resume，recursive cancel 覆盖 AgentTool。
- MySQL/MinIO 集成：checksum 损坏、blob 缺失、存储超阈值、旧 token Set、current pointer CAS。
- 端到端：模型边界暂停、工具边界暂停、Resume 的 `paused → queued → running` 与 Attempt 创建时机、进程重启恢复、配置不兼容进入 suspended、取消与暂停竞态。

退出条件：系统只在验证通过后显示 paused；单实例重启可从兼容 checkpoint 恢复；损坏或不兼容 checkpoint 绝不盲目重跑。
