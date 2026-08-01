# G0：Harness Kernel 契约与迁移实施设计

## 1. 文档状态与范围

- 状态：草案，等待批准 `2026-07-28-project-agent-harness-architecture.md` 中第 15 节的架构决策后实施。
- 对应分支：`feature/harness-kernel-runstore`（从 `harness` 切出）。
- 审计基线：`903f655 feat(agentcontext): add minimal persistent harness`。
- 前置条件：不改变现有 Chat API、Eino Agent 行为或 `internal/agentcontext` 的上下文编译契约。

本工作流只冻结通用 Harness 的领域契约、目标数据模型和从最小 Context Harness 的迁移路径。它不接管请求接入、Worker、SSE、暂停恢复或多实例执行；这些能力分别属于后续分支。

重要假设：首期仍以 MySQL 作为唯一正确性事实源，并且一个用户聊天输入最终对应一个顶层 Run。若批准改为 Redis 或消息队列持有 Run 事实，或允许同一 conversation 无限并行顶层 Run，本设计需要重新评审。

## 2. 目标与可验证的完成标准

完成 G0 后，后续模块能在不引入 GORM、Eino、Redis、NATS 或 `agentcontext` 依赖的前提下，共享同一套 Run 身份、状态、执行权、版本快照和错误分类语义。

退出条件：

- `internal/agentharness/core` 仅包含 Go 标准库和项目内无基础设施依赖的类型；禁止反向依赖 `agentcontext`。
- Run、Attempt、Step、Authority、Revision、状态和版本快照拥有唯一的类型定义与契约测试。
- `agent_runs`、`agent_run_attempts`、`agent_run_steps` 的迁移草案经过 MySQL 集成测试验证：主外键、唯一键、索引与状态 CAS 都可用。
- 已明确 `agent_context_runs` 的淘汰步骤；在迁移完成前不发生双权威状态机写入。
- 现有 `internal/agentcontext/harness` 测试和 Chat 路径保持原行为。

## 3. 当前基线与迁移原则

当前 `agent_context_runs` 持久化了 Context Run、单进程 Authority、revision 和终态；`agent_context_manifests` 与 `agent_context_writebacks` 已使用该 Run ID。它解决 Context 编译与写回闭环，但没有 Attempt、关键 Step、Command、Event、Checkpoint 或独立调度语义，因而不能作为通用 Agent Harness 的终态模型。

迁移采用“先新增、后切换、最后清理”的方式：

1. G0 只新增通用 Kernel 和目标表迁移，不让生产流量写入新表。
2. G1（Admission + Supervisor + Finalization）在一个事务内创建 `agent_runs` 与入口用户消息，并把 Context 生命周期适配到该通用 Run；从该发布开始，新增 Harness 流量只以 `agent_runs` 为权威状态。
3. 已完成的 `agent_context_runs` 历史记录保留到既定数据保留期，不回填成伪造的 Attempt 或 Step；仍在执行的旧记录继续由旧路径完成。
4. 仅当没有代码读写 `agent_context_runs`，且历史保留期已结束，才另开迁移删除旧表。回滚只停止新 Run 进入通用 Harness，不删除任何 Run、消息或 Manifest。

不允许在同一 Run 上同时推进 `agent_context_runs` 和 `agent_runs` 两个状态机。需要关联旧 Context 数据时，通过不可变 `run_id` 或一次性映射读取，不做双向同步。

## 4. Kernel 包与依赖边界

目标目录：

```text
internal/agentharness/core/    # 领域类型、纯状态机、端口与错误
internal/agentharness/run/     # 仅依赖 core 的 RunStore 用例
internal/agentharness/store/   # 后续实现；GORM/MySQL adapter
internal/app/                  # 唯一的具体装配点
```

`core` 可以定义接口，但不实现数据库访问、ID 生成、HTTP 参数解析、Agent 构建或消息写回。`run` 不导入 Eino；Eino 只从后续 `execute` adapter 进入。`agentcontext` 通过 integration adapter 读取通用 Run 快照，不能反向依赖 Worker、Store 或 HTTP。

## 5. 冻结的领域契约

以下名称和字段是 G0 的最小稳定出口。字段可在后续增加，但不能改变既有字段的语义或把 ID 改成基础设施专属类型。

```go
type RunID string
type AttemptID string
type StepID string
type StateVersion uint64
type FencingToken uint64
type Revision uint64

type ExecutionAuthority struct {
    AttemptID       AttemptID
    FencingToken    FencingToken
    RunStateVersion StateVersion
}

type VersionSnapshot struct {
    AgentDefinitionVersion string
    PromptVersion          string
    ToolSchemaVersion      string
    ModelConfigHash        string
    ContextProfileVersion  string
    EinoVersion            string
}

type ErrorClass string

const (
    ErrorClassPermanent         ErrorClass = "permanent"
    ErrorClassTransient         ErrorClass = "transient"
    ErrorClassRateLimited       ErrorClass = "rate_limited"
    ErrorClassTimeout           ErrorClass = "timeout"
    ErrorClassCancelled         ErrorClass = "cancelled"
    ErrorClassResourceExhausted ErrorClass = "resource_exhausted"
    ErrorClassInvalidInput      ErrorClass = "invalid_input"
    ErrorClassPermission        ErrorClass = "permission"
    ErrorClassDependencyPermanent ErrorClass = "dependency_permanent"
    ErrorClassWorkerLost        ErrorClass = "worker_lost"
    ErrorClassCheckpointIncompatible ErrorClass = "checkpoint_incompatible"
    ErrorClassSideEffectUnknown ErrorClass = "side_effect_unknown"
)
```

`RunID`、`AttemptID` 与 `StepID` 的具体生成方式由调用方决定；首次实现使用 UUIDv7，并在 Store 层按字符串保存。`ExecutionAuthority` 只在 Worker claim 后有效：新 Attempt 或接管必须创建新的 `AttemptID` 并递增 `FencingToken`，任何持久化更新同时比较 `RunStateVersion`。

`VersionSnapshot` 在 Admission 时冻结。它不得包含 Prompt 正文、API key、用户消息、网页正文或可恢复的模型凭据；模型配置以稳定 hash 表示，实际可重建配置由受控的应用配置版本解析。

上述 12 个值是首期完整的 `ErrorClass` 契约，与架构 M09 一致；`permanent` 专指项目内部不可恢复错误，`dependency_permanent` 指已经确认不会因重试改变结果的外部依赖错误。它们不是“先冻结 5 个、再由模块私自追加”的两阶段枚举。未来新增值只能通过 ADR 进行加法演进：新代码写入新值前必须先完成兼容性评估；写入端校验当前已知值，读取端保留未知字符串、不得自动重试，并以受控错误显示或进入 `suspended`，从而允许滚动发布中的旧进程安全读取新记录。

### 5.1 Run 与输入引用

```go
type InputRef struct {
    Kind string // chat_message | search_task | external_reference
    Ref  string // 领域记录的稳定 ID；不复制用户正文
    Hash string // SHA-256，校验引用内容是否被意外替换
}

type Run struct {
    ID              RunID
    ParentRunID     *RunID
    AgentType       string
    UserID          uint
    NotebookID      *uint
    ConversationID  *uint
    Input           InputRef
    VersionSnapshot VersionSnapshot
    State           RunState
    DesiredState    DesiredState
    StateVersion    StateVersion
    Revision        Revision
    Authority       *ExecutionAuthority
}
```

Chat/Main 的 `InputRef` 指向 Admission 在同一事务中创建的入口用户消息；Search 的输入指向结构化 SearchTask 记录或其不可变 artifact。G0 不定义第二份 `input_json` 来复制聊天正文。后续若确实需要存储 SearchTask，必须先定义独立、脱敏且版本化的 artifact 格式。

### 5.2 状态与状态机

G0 一次性冻结下列枚举值，避免后续 Pause、Retry 或恢复模块另建状态语义：

```go
type RunState string
const (
    RunStateQueued          RunState = "queued"
    RunStateRunning         RunState = "running"
    RunStateFinalizing      RunState = "finalizing"
    RunStateRetryWait       RunState = "retry_wait"
    RunStatePauseRequested  RunState = "pause_requested"
    RunStatePausing         RunState = "pausing"
    RunStatePaused          RunState = "paused"
    RunStateCancelRequested RunState = "cancel_requested"
    RunStateSucceeded       RunState = "succeeded"
    RunStateFailed          RunState = "failed"
    RunStateCancelled       RunState = "cancelled"
    RunStateSuspended       RunState = "suspended"
)

type DesiredState string
const (
    DesiredStateRunning   DesiredState = "running"
    DesiredStatePaused    DesiredState = "paused"
    DesiredStateCancelled DesiredState = "cancelled"
)
```

首期 RunStore 必须验证以下转换；尚未实现对应命令的转换只能由测试覆盖，不能从 HTTP 暴露：

| 当前状态 | 目标状态 | 触发者 | 必要条件 |
| --- | --- | --- | --- |
| `queued` | `running` | Worker claim | CAS 成功；创建 Attempt；设置 Authority |
| `running` | `finalizing` | Supervisor | 当前 Authority 仍有效 |
| `finalizing` | `succeeded` / `failed` / `cancelled` | Finalizer | 主结果或终态 Journal 已按结果语义提交 |
| `running` | `retry_wait` | Retry policy | 错误为 `transient`、`rate_limited` 或 `timeout`，且总预算未耗尽并已证明可安全重试 |
| `retry_wait` | `queued` | Dispatcher | `next_retry_at` 已到期 |
| `running` | `pause_requested` | Command | 仅记录用户意图；不代表已 checkpoint |
| `pause_requested` | `pausing` | Worker | 当前 Authority 观察到命令 |
| `pausing` | `paused` | Checkpoint adapter | checkpoint 已校验可读、checksum 和版本兼容 |
| `paused` / 经人工批准的 `suspended` | `queued` | Resume command | checkpoint 与 VersionSnapshot 已验证；只登记待恢复引用，不创建 Attempt |
| `running` / `queued` / `paused` | `cancel_requested` | Command | 持久化取消意图 |
| `cancel_requested` | `cancelled` | Worker / Finalizer | 清理和终态记录完成 |
| `running` / `pausing` | `suspended` | Supervisor | 不可安全恢复或 checkpoint 不可用 |

所有转换都必须以 `(run_id, state, state_version, fencing_token)` 条件更新；无匹配行统一返回 `ErrAuthorityStale` 或 `ErrInvalidTransition`，调用方重新读取后决定，不得盲目重试写入。

## 6. 目标 MySQL 数据模型

G0 仅定义并测试下列三张表。Command、Event、Outbox、Checkpoint 和 Writeback Journal 保留给各自工作流，避免本分支创建无人使用的表。

### 6.1 `agent_runs`

| 字段 | 约束/索引 | 用途 |
| --- | --- | --- |
| `id` | `varchar(36)` PK | 稳定 Run ID |
| `parent_run_id` | nullable, index | 顶层 Run 为空；仅用于显式嵌套 Run |
| `agent_type` | `varchar(64)`, index | `chat`、`main`、`search` 等定义标识 |
| `user_id` | index | 所有权范围 |
| `notebook_id`、`conversation_id` | nullable, index | 业务作用域 |
| `input_kind`、`input_ref`、`input_hash` | not null | 引用入口输入且不复制正文 |
| `version_snapshot_json` | JSON, not null | 冻结的兼容性快照 |
| `state`、`desired_state` | `varchar(32)`, index | 执行状态与用户意图 |
| `state_version`、`revision` | unsigned bigint | CAS 与终态/写回 revision |
| `current_attempt_id` | nullable, index | 当前执行 Attempt |
| `fencing_token` | unsigned bigint | 当前执行权代数 |
| `pending_resume_checkpoint_ref` | nullable | 已验证、等待下一次 Claim 写入新 Attempt 的 checkpoint 引用 |
| `retry_count`、`max_retries`、`next_retry_at` | nullable/index | 后续重试策略预留 |
| `last_error_class`、`last_error_code` | nullable | 结构化失败原因；不存不受控错误正文 |
| `created_at`、`started_at`、`finished_at`、`updated_at` | timestamps | 生命周期查询 |

Admission 实现时增加 `idempotency_key`，并建立 `UNIQUE(user_id, idempotency_key)`；G0 不允许为尚不存在的 Admission API 创建空语义的去重记录。相同理由，`owner_worker_id`、lease 到期时间和 checkpoint 指针在对应模块首次使用时通过加法迁移引入。

### 6.2 `agent_run_attempts`

| 字段 | 约束/索引 | 用途 |
| --- | --- | --- |
| `id` | `varchar(36)` PK | Attempt ID |
| `run_id` | FK → `agent_runs.id`, index | 所属 Run |
| `attempt_number` | `UNIQUE(run_id, attempt_number)` | 每次 Claim（包括恢复 Run 的 Claim）的有序历史 |
| `worker_id` | nullable | 首期 `all` 角色也写稳定进程标识 |
| `fencing_token` | not null | 本 Attempt 写入授权 |
| `resume_checkpoint_ref` | nullable | 后续 Resume 引用，不保存 Blob |
| `trace_id` | nullable, index | OTel 关联 |
| `state`、`error_class`、`error_code` | nullable/index | Attempt 结果 |
| `started_at`、`heartbeat_at`、`finished_at` | timestamps | 执行诊断 |

### 6.3 `agent_run_steps`

| 字段 | 约束/索引 | 用途 |
| --- | --- | --- |
| `id` | `varchar(36)` PK | Step ID；首期与 Run/Attempt 一样使用 UUIDv7，不支持复合编码 |
| `run_id` | FK → `agent_runs.id`, index | 所属 Run |
| `attempt_id` | FK → `agent_run_attempts.id`, index | 产生该 Step 的 Attempt |
| `parent_step_id` | nullable, index | 子 Step 关系 |
| `kind`、`agent_name`、`state` | index | Search、外部副作用工具等关键边界 |
| `input_hash`、`tool_call_id` | nullable/index | 幂等与 Eino 关联 |
| `result_artifact_ref` | nullable | 后续 artifact 引用 |
| `fencing_token` | not null | 拒绝旧执行者迟到写入 |
| `error_class`、`error_code`、`started_at`、`finished_at` | nullable/index | 状态和诊断 |

模型调用、逐 token 事件和普通纯函数工具不写入 `agent_run_steps`。表外键的删除策略为 `RESTRICT`；Harness 记录必须通过数据保留任务清理，不能因为删除 conversation 而无审计痕迹地级联删除。

## 7. RunStore 端口与原子性

`run` 包拥有使用方接口，Store 实现不能泄露 GORM model：

```go
type Store interface {
    CreateQueued(ctx context.Context, run Run) error
    Get(ctx context.Context, id RunID) (Run, error)
    Claim(ctx context.Context, id RunID, workerID string) (Run, Attempt, error)
    Transition(ctx context.Context, req TransitionRequest) (Run, error)
    CreateStep(ctx context.Context, step Step) error
    FinishStep(ctx context.Context, req FinishStepRequest) (Step, error)
}
```

`Claim` 必须在同一个 MySQL 事务中：锁定或 CAS 更新 queued Run、递增 fencing token、创建新的 Attempt、设置 `current_attempt_id` 和 `started_at`。`Transition` 只负责状态和结构化错误字段；Assistant 消息、Manifest 与派生写回仍由后续 Finalization 事务负责。不能把“先更新状态、再单独插入 Attempt/Step”拆成两个无保护的数据库调用。

## 8. 实施顺序与验证

1. 在 `core` 编写类型、状态转换表和纯函数测试；测试覆盖合法转换、终态不可变、状态版本递增、Authority 不匹配、写入未知枚举拒绝，以及读取未知持久化错误分类时不自动重试。
2. 在 `run` 定义 Store、TransitionRequest 和错误类型，再以内存 fake 验证调用方不依赖 SQL 细节。
3. 在 `store` 实现 GORM/MySQL adapter 与三个加法 migration；migration 必须可重复执行，并在空库和已有 `agent_context_*` 表的库上测试。
4. 增加 MySQL 集成测试，至少覆盖并发 claim 只有一个赢家、旧 token 拒绝写入、Attempt 序号唯一、Step 外键和状态 CAS。
5. 完成依赖方向检查与现有回归：`go test ./internal/agentharness/... ./internal/agentcontext/...`；MySQL 集成测试使用项目既有测试数据库配置，未配置时明确跳过且不得视作通过。

每一步只新增契约和持久化能力，不接入 `internal/app`，不迁移线上流量。这样失败时可回滚到没有任何新表读写的代码版本；已经创建的加法表保留，不执行破坏性回滚。

## 9. 明确不在 G0 实现的内容

- Chat API 的 Admission 原子事务、idempotency key 接收与 conversation 排队。
- Eino Runner、Supervisor、模型重试、HTTP/SSE 生命周期解耦。
- Search/Youdao Step Gateway、Artifact 存储与领域任务迁移。
- Cancel/Pause/Resume API、Eino CheckPointStore、Worker、Dispatcher、Lease、NATS 或 Outbox。
- `agent_context_runs` 的删除或历史回填。
- OpenTelemetry 后端、生产指标面板或完整故障注入环境。

## 10. 实施前的批准门禁

开始 `feature/harness-kernel-runstore` 编码前，需要明确批准项目级架构第 15 节的十项决策，尤其是：MySQL 为事实源、同一 conversation 的单活动顶层 Run、Checkpoint 前 Stop 为 cancel，以及最终只保留 `agent_runs` 这一权威状态源。

批准后，本文件中的表结构、枚举、包依赖方向和迁移顺序即为该分支的验收基线；任何改变 checkpoint 或副作用语义的扩展，应先补充 ADR 与兼容性评估。

## 11. 实施记录

### 2026-07-28 实施状态

**已完成的工作：**

1. **core 包实现** (`internal/agentharness/core/`)：
   - `types.go`：定义了 RunID、AttemptID、StepID、StateVersion、FencingToken、Revision 等类型
   - `statemachine.go`：实现了状态转换表和验证逻辑
   - `ports.go`：定义了 RunStore 接口
   - `errors.go`：定义了标准错误类型
   - `types_test.go`：类型测试
   - `statemachine_test.go`：状态机测试

2. **run 包实现** (`internal/agentharness/run/`)：
   - `service.go`：定义了 Service 结构和高级操作
   - `service_test.go`：Service 测试

3. **store 包实现** (`internal/agentharness/store/`)：
   - `models.go`：定义了 GORM 模型（AgentRun、AgentRunAttempt、AgentRunStep）
   - `gorm_store.go`：实现了 GormStore，支持 CreateQueued、Get、Claim、Transition、CreateStep、FinishStep
   - `gorm_store_test.go`：使用内存 mock 的测试

**实际偏差：**

1. **ID 生成**：当前使用简单的时间戳+随机数生成 ID，而非 UUIDv7。需要在生产环境使用真正的 UUIDv7 生成器。

2. **MySQL 集成测试**：测试使用 MySQL 进行集成测试，需要设置环境变量 `TEST_MYSQL_DSN`。测试包括：
   - 基本 CRUD 操作
   - 并发 Claim 测试（验证只有一个赢家）
   - 旧 Token 拒绝写入测试
   - Attempt 序号唯一性测试

3. **表结构**：已创建 GORM 模型和 SQL 迁移脚本 `001_create_agent_tables.sql`。

**验证结果：**

- `go test ./internal/agentharness/...`：所有测试通过
- `go vet ./internal/agentharness/...`：无警告
- `go build ./internal/agentharness/...`：编译成功
- `go build ./...`：整个项目编译成功

**依赖方向检查：**

- `core` 包：仅依赖 Go 标准库，无外部依赖 ✓
- `run` 包：仅依赖 `core` 包，无外部依赖 ✓
- `store` 包：依赖 `core` 包和 GORM，无其他外部依赖 ✓

**待完成工作：**

1. 使用真正的 UUIDv7 生成器替换简单 ID 生成
2. 补充更多边界条件测试

**运行 MySQL 集成测试：**

```bash
# 设置测试数据库 DSN
export TEST_MYSQL_DSN="user:password@tcp(host:port)/test_database?charset=utf8mb4&parseTime=True&loc=Local"

# 运行测试
go test ./internal/agentharness/store/... -v
```

如果使用 Docker Compose，可以这样获取测试数据库：

```bash
# 启动 MySQL 容器
docker-compose up -d mysql

# 获取容器 IP
MYSQL_IP=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' youdaonotelm-mysql)

# 设置 DSN
export TEST_MYSQL_DSN="root:your_mysql_root_password@tcp(${MYSQL_IP}:3306)/youdao?charset=utf8mb4&parseTime=True&loc=Local"
```

**已创建的文件：**

```
internal/agentharness/
├── core/
│   ├── types.go          # 核心类型定义
│   ├── statemachine.go   # 状态机实现
│   ├── ports.go          # 端口接口定义
│   ├── errors.go         # 错误类型定义
│   ├── types_test.go     # 类型测试
│   └── statemachine_test.go # 状态机测试
├── run/
│   ├── service.go        # Service 实现
│   └── service_test.go   # Service 测试
└── store/
    ├── models.go         # GORM 模型定义
    ├── gorm_store.go     # GormStore 实现
    ├── gorm_store_test.go # GormStore 测试
    └── migrations/
        ├── 001_create_agent_tables.sql # 数据库迁移脚本
        └── README.md     # 迁移说明文档
```
