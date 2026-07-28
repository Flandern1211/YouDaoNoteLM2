# H4：Finalization、幂等写回与 Repair 实施设计

## 状态、范围与前置

- 状态：草案；依赖 H1–H3，并实现 H3 已冻结的 `FinalizationPort`。
- 分支：`feature/harness-finalization`。

本工作流让模型执行结束与领域写回之间形成可靠边界。主结果、终态 Manifest 与可恢复写回意图必须有可检查的持久化证据；不得依赖请求结束后一个孤立 goroutine 来“尽量保存”。

## 状态流与授权

Supervisor 以当前 Authority 将 `running → finalizing` 后调用 H3 的 `FinalizationPort.Finalize`。`FinalizationRequest` 中的 `FinalizingStateVersion` 是该转换成功后的版本；Finalizer 不接受或重新执行 Eino。Store 以 Run ID、Attempt ID、FencingToken、该 StateVersion 与 `finalizing` state 同时校验。成功流程是：

1. 为该 revision 生成稳定 `FinalizeKey` 与 assistant writeback idempotency key。
2. 同一事务提交 assistant message（如成功）、`agent_writeback_journal` 主记录和必要的派生 intent。
3. assistant 提交成功后，才允许 Summary、Memory 等派生写回；Search 只写 StepResult/Artifact，不写主会话 assistant message。
4. 无论成功、失败还是取消，都提交无正文终态 Context Manifest 和 terminal Event intent。
5. 所有必需主写回成功后，CAS `finalizing → succeeded/failed/cancelled`。

Legacy/Shadow 仍由原写回路径拥有主结果；Enabled Harness 写回必须互斥，禁止同一个 Run 由两条路径都创建 assistant message。

## Journal 与 Repair

新增 `agent_writeback_journal`：`idempotency_key` 唯一，保存 Run、revision、操作类型、状态、Authority 快照、依赖关系、受控错误码、尝试次数与时间。它记录事实和待办，不保存消息/Payload 正文。需要异步的 Summary、Memory、Manifest/Event 发布均以 Journal/Outbox intent 表示。

Repair Scanner 首期可以由 `all` 角色的有界定时任务执行：只领取过期且未完成的 Journal item，重新验证 `FinalizationAuthority`，按依赖图重试。它不重新运行模型，不重写已成功 assistant message，也不把失败 Run 误提取为长期记忆。H8/H9 会把领取与多实例排他强化。

## 失败、回滚与迁移

- Assistant 写入失败：Run 保留 `finalizing` 和 Journal，不能推进 Summary/Memory；Repair 可重试。
- 派生写入失败：assistant 不回滚，Journal 记录可重试状态；终态由策略决定是否等待必须的 Manifest。
- Manifest 或 Event 写入失败：不丢失主结果；保留可修复 intent 与受控告警。
- Authority 过期：拒绝所有主/派生写入，新的 Attempt/Repair 重新读取当前事实。

新增表均为加法迁移；旧 `agent_context_writebacks` 在 Context adapter 完成迁移前保留只读/兼容职责。回滚停止新 Finalizer，不删除 Journal；恢复后 Scanner 必须能继续处理。

## 验证与退出条件

- 重复 Finalize、进程在每个事务边界失败、重复 Repair、旧 fencing token、无最终文本、失败/取消 Outcome。
- 断言 assistant 最多一条，Summary/Memory 绝不先于 assistant，终态 Manifest 不含正文，Search 不产生主会话消息。
- MySQL 集成验证 message + journal 原子性、唯一幂等键和 terminal CAS。

退出条件：任一终态都有可重试、可审计的 FinalizeResult；主结果不会重复，且失败不会依赖进程内偶然存活的 goroutine。
