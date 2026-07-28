# Harness 分支规划

整体分支按 `harness` 长期集成主线来规划：

```text
main / develop
  -> harness
      -> feature/harness-kernel-runstore
      -> feature/harness-admission-api
      -> feature/harness-supervisor-eino
      -> ...
```

`feature/context-management` 作为当前上下文管理与架构设计分支，负责把项目级 harness 架构、模块边界和实施计划沉淀清楚。设计确认后，创建 `harness` 作为后续 harness 工程实现的主集成分支。

后续所有 harness 实现分支都从 `harness` 切出，完成验收后先合并回 `harness`。等 `harness` 达到稳定闭环后，再整体合回主线。

| 顺序 | 分支名 | 作用 | 合并后应该具备的能力 |
| --- | --- | --- | --- |
| 0 | `feature/context-management` | 当前设计分支，承载上下文管理设计与项目级 harness 架构文档 | 明确整体架构、模块边界、实施阶段；作为创建 `harness` 的设计依据 |
| H | `harness` | Harness 长期集成主线 | 汇总所有 harness 子分支，形成可持续验证的工程主线 |
| 1 | `feature/harness-kernel-runstore` | Harness 地基 | 定义 `Run / Attempt / Step / Authority / Revision`，落 MySQL Run 状态机和最小持久化事实源 |
| 2 | `feature/harness-admission-api` | 请求接入和兼容现有 API | 用户消息持久化、Run 接受、幂等、防重复提交、兼容现有 Chat API |
| 3 | `feature/harness-supervisor-eino` | 执行闭环 | 创建 Attempt，Supervisor 调 Eino Runner，接 ContextCompiler，记录关键 Step 和基础错误分类 |
| 4 | `feature/harness-finalization` | 最终写回可靠化 | assistant message 幂等写回、manifest 持久化、Run 终态 revision、finalizing 状态、repair scanner 最小版 |
| 5 | `feature/harness-events-sse` | 事件日志和 SSE 重放 | semantic event log、事件序号、SSE 断线后按 `after_seq` replay，SSE 断线只 detach |
| 6 | `feature/harness-interrupt-cancel` | 持久化取消机制 | cancel command 入库，worker 响应取消，Run 进入 `cancelled`，逐步替代进程内 `sync.Map` cancel |
| 7 | `feature/harness-checkpoint-resume` | Checkpoint 和恢复 | Eino CheckPointStore adapter、checkpoint 元数据、`Runner.Resume / ResumeWithParams`、新 Attempt 从 checkpoint 继续 |
| 8 | `feature/harness-worker-dispatcher` | 后台 worker 化 | API 创建 Run，MySQL dispatcher 派发，worker 领取并执行 Run，Run 生命周期脱离 HTTP 请求 |
| 9 | `feature/harness-lease-fencing` | 多实例安全 | lease、fencing token、旧 worker 迟到写入拒绝，多 worker 接管时不乱写 |
| 10 | `feature/harness-resource-observability-tests` | 工程质量补强 | 并发/费用/token 预算、核心 metrics/log/audit、契约测试、故障注入测试夹具 |

除 `feature/context-management` 外，表中的 `feature/harness-*` 分支都应从 `harness` 切出，并在完成后合并回 `harness`。

我会把实现分支分成三批做。

第一批，先让单进程 Run 闭环成立：

```text
feature/harness-kernel-runstore
feature/harness-admission-api
feature/harness-supervisor-eino
feature/harness-finalization
feature/harness-events-sse
```

这批完成后，系统应该能做到：

```text
用户发消息
  -> 创建 Run
  -> 执行 Eino Agent
  -> 使用上下文模块
  -> 写回 assistant 结果
  -> 写 manifest
  -> 写事件
  -> 前端能查状态和看 SSE
  -> Run 进入终态
```

第二批，增强生命周期控制：

```text
feature/harness-interrupt-cancel
feature/harness-checkpoint-resume
```

这批解决：

```text
用户明确取消
可恢复中断
服务重启后从 checkpoint 继续
```

第三批，走向多实例和工程成熟：

```text
feature/harness-worker-dispatcher
feature/harness-lease-fencing
feature/harness-resource-observability-tests
```

这批解决：

```text
API 和 worker 分离
多 worker 抢任务不乱
旧执行者不能迟到覆盖
预算、监控、审计、故障测试更完整
```

当前最该做的是：

```text
1. 提交并推送 feature/context-management
2. 基于确认后的设计创建 harness 分支
3. 新建 worktree + feature/harness-kernel-runstore，从 harness 切出
4. 只实现 Kernel + RunStore 最小状态机
```

建议最多同时开 2-3 个 worktree，不要一次性把 10 个分支都建出来。先保留一个 `harness` worktree 作为集成主线，再按当前阶段创建 1-2 个功能 worktree。

每个功能分支合回 `harness` 前至少满足：

- 相关测试通过，或明确说明未运行的原因和影响。
- 没有混入无关改动。
- 状态机、数据表或接口契约变更已同步到设计文档。
- 不破坏 `harness` 上已有的单进程闭环能力。
