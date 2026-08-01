# Agent Harness 实施文档索引

## 状态

本目录将已批准的 Context 管理设计和待批准的项目级 Harness 架构拆成可独立实施、可独立验收的工作流。除 `00-g0` 以外的文档均不得视为已授权编码；编码前须批准 [项目级架构](../../superpowers/specs/2026-07-28-project-agent-harness-architecture.md) 第 15 节的决策。

## 实施顺序

| 顺序 | 文档 | 实现分支 | 前置 | 交付结果 |
| --- | --- | --- | --- | --- |
| H1 | [G0 Kernel 与迁移](00-g0-kernel-contracts-and-migration.md) | `feature/harness-kernel-runstore` | 架构批准 | 通用 ID、状态、Authority、RunStore 与目标表 |
| H2 | [Admission 与兼容 API](01-h2-admission-api.md) | `feature/harness-admission-api` | H1 | 用户消息与 queued Run 的原子接纳、幂等、会话排队 |
| H3 | [Supervisor 与 Eino](02-h3-supervisor-eino.md) | `feature/harness-supervisor-eino` | H1、H2 | Attempt 执行闭环、Step Gateway、错误/重试预算 |
| H4 | [Finalization 与 Repair](03-h4-finalization-repair.md) | `feature/harness-finalization` | H1–H3 | 主结果幂等写回、Journal/Repair、终态一致性 |
| H5 | [事件日志与 SSE](04-h5-events-sse.md) | `feature/harness-events-sse` | H1–H4 | 有序语义事件、状态查询和断线重放 |
| H6 | [Interrupt 与 Cancel](05-h6-interrupt-cancel.md) | `feature/harness-interrupt-cancel` | H1–H4 | 持久化取消命令，替代进程内 cancel 事实 |
| H7 | [Checkpoint 与 Resume](06-h7-checkpoint-resume.md) | `feature/harness-checkpoint-resume` | H1、H3、H6 | Eino 安全点暂停、版本校验与恢复 |
| H8 | [Dispatcher 与 Worker](07-h8-worker-dispatcher.md) | `feature/harness-worker-dispatcher` | H1–H6 | MySQL durable queue、`all/api/worker` 角色 |
| H9 | [Lease、Fencing 与 NATS](08-h9-lease-fencing.md) | `feature/harness-lease-fencing` | H7、H8 | 多实例接管；达到门槛后引入 NATS |
| H10 | [资源、可观测性与硬化](09-h10-resource-observability-tests.md) | `feature/harness-resource-observability-tests` | H1–H9 | 预算、OTel、SLO、故障注入和灰度 |

H2–H5 构成单进程正确性闭环：H2 定义并首次写入事件信封，H3 通过 Finalization 端口交付 Outcome，H4 完成幂等终态写回，H5 再提供同一事件事实源的查询与 SSE 投影。H6–H7 增强生命周期；H8–H10 才引入进程解耦、多实例与生产硬化。H8 以 H6 的持久化 Cancel 命令为硬前置；Pause/Resume 的 claim 前处理在 H7 启用后接入，不阻塞 H8 的基础调度。每个分支完成后必须同步该文档的实际偏差，相关测试通过，并保持已有闭环不退化。

## 共用约束

- MySQL 是 Run、Command、事件和审计的唯一事实源；Redis、NATS 仅做加速或投递。
- `agent_context_runs` 不是通用 Run 的长期权威状态；迁移期间禁止双状态机同步推进。
- Run 创建时冻结 Agent、Prompt、工具、模型、Context Profile 与策略版本；运行中不得热切换协议。
- 常规日志、Event、Manifest 和 Trace 不保存完整 Prompt、网页正文、checkpoint 或凭据。
- 测试从纯契约到 MySQL 集成逐层覆盖；未运行的集成或故障测试必须明确报告，不能默认为通过。
