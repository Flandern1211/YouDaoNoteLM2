# H10：资源预算、可观测性、测试与生产硬化实施设计

## 状态、范围与前置

- 状态：草案；贯穿 H1–H9，但容量门槛、完整故障注入和全量灰度在 H9 后验收。
- 分支：`feature/harness-resource-observability-tests`。

本工作流不增加第二套业务生命周期，而是为现有 Kernel、Supervisor、Store 和 Worker 提供统一预算、遥测、数据治理、测试夹具与运维门禁。

## 资源与费用预算

Admission 冻结 `ExecutionBudget`：max wall time、iterations、model calls、input/output tokens、search/tool calls、retry units 和估算费用。它同时快照价格/模型策略版本；不把易变价格写死到 core。Supervisor 与 Step Gateway 原子或可重放地累计 Usage，父 Run 给子 Step 分配额度并接收实际回传。

在 Admission 做用户/系统配额检查，在 Dispatcher 做并发槽控制，在运行时做每次调用前扣减。预算耗尽写 `resource_exhausted` 与可解释 Event；产品策略可决定 failed 或 suspended，但不得继续无预算调用。Provider semaphore/limiter 防止一个模型或搜索供应商拖垮全部 Worker。

## Telemetry、审计与隐私

使用 OpenTelemetry API：Run、Attempt、Step 建 span，模型/工具调用为子 span；长时间队列/进程边界使用 span link，不维持跨小时 parent span。Run ID/Attempt ID 写 trace 与结构化日志，不作为 metrics label；指标只使用低基数 agent type、state、error class、provider 和结果。

最小指标：队列等待、Run/Attempt/Step 时长、terminal 数、retry、cancel、pause/resume、checkpoint 成功率/尺寸、预算耗尽、Event/Outbox 积压、lease 接管、Context token/降级。审计记录 actor、command、前后状态、Authority、受控 code 与时间；不记录完整 Prompt、用户秘密、网页正文、checkpoint 或长期记忆值。

为 Run、Event、Audit、Checkpoint、Artifact、Journal 各自定义保留期、归档/删除 job 和失败告警。删除前遵守外键和活跃 Run 保护；隐私删除请求需由独立合规流程决定，不能以通用表级级联删除替代。

## TestKit、评测与故障注入

建立共享 Harness TestKit：假时钟、确定性 ID、MemoryStore、故障注入 Store、记录型 EventSink、fake Dispatcher、fake Eino Agent/CheckpointStore 和受控 Runner。每个模块先覆盖纯状态/幂等/预算契约，再运行 MySQL、Redis、MinIO、NATS（启用时）集成测试。

固定 Eino 升级门禁：Context Handler 每次模型调用前执行；CancelError、安全点 checkpoint、Runner.Resume、recursive AgentTool cancel 和 ModelRetryConfig 语义均需通过项目契约。质量评测覆盖关键上下文约束召回、Search 结果完整性、暂停恢复前后引用一致性、token/费用回归；不对非确定模型回答使用脆弱的全文精确断言。

## 灰度、SLO 与 Runbook

Harness flag 独立于 Context `legacy/shadow/enabled`；按稳定 user bucket 以 5%→25%→50%→100% 灰度，Run 创建时冻结结果。回滚只禁止新 Run 进入 Harness，已接受 Run 继续完成、暂停或由恢复策略处理，绝不删除事实表。

上线前定义并监控：接受成功率、排队等待、Run 成功/失败/取消/暂停率、重复写入率（应为零）、恢复成功率、p95/p99 端到端耗时、预算耗尽、数据库连接池和 Outbox 积压。Runbook 覆盖 provider 故障、队列积压、Repair 堵塞、checkpoint 损坏、worker drain、zombie writer、回滚和数据清理。

## 验证与退出条件

- race、单元、集成、故障注入、容量和灰度回滚测试分层执行；每次结果记录环境、版本、样本和未覆盖项。
- 容量目标由真实基线批准后设定；不得直接宣称旧设计的 100–300 并发已满足。达到目标前监测无持续队列积压、数据库池耗尽或成本失控。
- 完成一次生产式 worker 接管和一次回滚演练；Eino/模型/Prompt/工具协议变更均通过兼容门禁。

退出条件：每个 Run 可通过 ID 定位状态、Attempt、关键 Step、写回、事件、trace 和审计；正确性、容量和运维证据满足批准的 SLO，才能扩大至全量用户。
