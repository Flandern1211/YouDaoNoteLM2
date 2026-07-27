# 上下文管理讨论决策记录

## 1. 目的

本文记录本次设计讨论中逐项检验的问题、主要备选方案、最终选择和选择理由。它是设计决策的可追溯记录，不替代模块设计文档。

2026-07-27 根据主流 Agent 框架的 Context/Session/State 生命周期实践和用户复审，修订 D01、D07、D22，并新增 D24–D29。随后根据现有仓库落地审查新增 D30–D33，明确当前代码桥接、Harness 依赖、TokenCounter 和 Feature Flag 机制。原“只读装配”结论保留为被替代的历史决定。

## 2. 决策总表

| 编号 | 决策主题 | 最终选择 |
|---:|---|---|
| D01 | 上下文模块职责范围 | ContextCompiler + 可选 TurnLifecycleCoordinator；外部模块独立且可替换 |
| D02 | 上下文装配时机 | 请求级准备 + 模型调用级编译 |
| D03 | RAG 触发 | 仅工具驱动 |
| D04 | Agent 差异表达 | 统一引擎 + 强类型 ContextProfile |
| D05 | 用户记忆对主/子 Agent 的可见性 | 主 Agent 检索 + 子 Agent 最小披露 |
| D06 | 信任提升 | Provider 内容不能提升指令优先级 |
| D07 | 执行后写回 | TurnLifecycleCoordinator 调度；ContextCompiler 保持只读 |
| D08 | 模型能力和 Token 计数 | 注册表、覆盖配置和 Provider-aware Counter |
| D09 | 用户输入超限 | 直接提示用户缩短 |
| D10 | Provider 故障策略 | 重试、回退和耗尽动作可组合 |
| D11 | Provider 接口形态 | 能力专用强类型接口 + 统一内部候选 |
| D12 | 首期 Agent 范围 | Chat/Main + Search |
| D13 | Search Agent 输入 | 最小强类型 SearchTask |
| D14 | 用户记忆召回边界 | Provider 召回排名，Manager 最终选择 |
| D15 | 会话历史策略 | 带游标摘要 + 最近消息 + Token 窗口 |
| D16 | Token 预算策略 | 硬保留、分类上限、优先级和水位治理 |
| D17 | Manifest 内容 | 默认只记录元数据 |
| D18 | Provider 并发 | 按依赖阶段并行 |
| D19 | 缓存所有权 | 数据缓存归 Provider，Manager 缓存纯计算 |
| D20 | 上下文概念 | 运行时上下文与模型上下文分离 |
| D21 | 迁移方式 | Legacy、Shadow、Enabled |
| D22 | 总体架构方案 | 轻量 ContextCompiler + 可选生命周期协调器 + Eino Middleware |
| D23 | 文档组织 | 总设计 + 按模块拆分文档 + 决策日志 |
| D24 | 入口用户消息 | RunService 接受 Run 时可靠持久化 |
| D25 | 写回依赖图 | 主结果成功后调度 Profile 允许的派生写回 |
| D26 | 摘要语义 | 运行内压缩与持久化会话摘要分离 |
| D27 | 写回可靠性 | 主结果与派生 intent 原子提交，Journal/Outbox 可修复 |
| D28 | 会话并发 | 首期同一 Conversation 串行 Turn |
| D29 | 恢复与模式 | PreparedTurn/CompileRecord 可恢复；写回模式按 Run 固定 |
| D30 | 当前代码迁移与包落位 | 新增目标包并通过显式 Adapter 渐进替代 ContextBuilder |
| D31 | 编译与 Harness 生命周期 | ContextCompiler 可独立交付；TurnLifecycleCoordinator 等待 Harness |
| D32 | 首期 TokenCounter | Anthropic 官方计数 + OpenAI 本地兼容计数 + UTF-8 保守回退 |
| D33 | Feature Flag 与稳定分桶 | 复用 Viper；SHA-256 + rollout version + 基点分桶 |

## 3. 逐项记录

### D01：ContextManager 的职责范围

备选：

- 纯装配器：调用方准备全部材料。
- 编排器：调用独立的历史、记忆、Prompt 等能力后统一装配。
- 完整上下文平台：同时负责记忆、RAG、会话和存储生命周期。

选择：

> 对外可以提供轻量 Manager 门面，内部拆分为可独立交付的 ContextCompiler 与依赖 Harness 的 TurnLifecycleCoordinator。它们分别协调上下文准备/模型调用前编译，以及执行后上下文派生写回；记忆、RAG、会话存储等模块独立实现并可替换。

理由：

- 满足依赖倒置和依赖注入。
- 不把领域存储职责集中到编译器或生命周期协调器。
- 避免外层 Service 重复理解 PreparedTurn、TurnOutcome 和写回依赖。
- 未来替换记忆机制时不修改 Agent 编排核心。

### D02：上下文装配时机

备选：

- 每个用户请求开始时只装配一次。
- 每次模型调用前全量重新装配。
- 两阶段装配。

选择：

> 请求开始时解析稳定的本轮候选；每次模型调用前只处理动态消息、工具结果和 Token 预算。

理由：

- 避免每轮重复查询记忆和历史。
- 能控制 ReAct 工具结果导致的上下文增长。

### D03：RAG 触发方式

备选：

- 每轮自动检索。
- 只作为工具。
- 首次自动检索 + 工具二次检索。

选择：

> 只作为工具，由 Agent 判断是否需要检索。

理由：

- 用户先选择知识库范围。
- 当前项目已经采用 RAG Tool。
- 避免无必要的每轮检索和重复 Token。

### D04：不同 Agent 的上下文组合

备选：

- 每个 Agent 单独实现 Builder。
- 统一 Manager + 强类型 Profile。
- 全部由 YAML/数据库动态配置。

选择：

> 统一 Manager + 代码级强类型 ContextProfile。

理由：

- 共享预算和失败治理，避免复制。
- 首期无需承担动态配置校验和运维复杂度。

### D05：用户记忆的可见性

备选：

- 所有 Agent 注入全部记忆。
- 按当前任务检索相关记忆并加入固定偏好。
- 只有主 Agent 读取，子 Agent 接收最小委托上下文。

选择：

> Main/Chat 检索相关记忆；子 Agent 默认只获得主 Agent 显式传递的必要约束。

理由：

- 减少 Token。
- 避免向子 Agent 泄露不相关或敏感信息。

### D06：Provider 内容的信任等级

选择：

> Provider 只提供事实、偏好和证据，不能通过正文提升自己的指令优先级。

具体含义：

- 系统提示词是唯一最高可信指令。
- 用户记忆和历史属于 UserProvided。
- RAG、网页和外部工具结果属于 ExternalUntrusted。

### D07：上下文模块是否写回

备选：

- 只读装配。
- 统一 Commit。
- 完整管理读写生命周期。

原选择：

> 只读装配。

2026-07-27 修订：

> ContextCompiler 保持只读；未来 TurnLifecycleCoordinator 调度执行后的上下文派生写回，但不实现消息、摘要、记忆或 Manifest 的领域算法和存储。

修订理由：

- 助手消息、摘要检查和记忆提取具有明确的先后依赖。
- 这些动作需要 `PreparedTurn`、最终 Agent State 和 `TurnOutcome`；由窄生命周期协调器消费，避免外层 Service 重复理解完整上下文。
- 独立 Writer 继续拥有摘要版本、记忆冲突、Repository 和缓存一致性，避免 Manager 变成领域存储平台。
- 工具结果和 checkpoint 的运行中持久化仍属于 Agent Runtime/Harness，不推迟到 `FinalizeTurn`。

### D08：模型能力与 Token 计数

初始备选：

- 仅内置模型表。
- 完全由用户填写。
- 内置表 + 显式覆盖。

最终选择：

> `ModelCapabilitiesResolver` 使用显式覆盖和版本化注册表；`TokenCounter` 独立使用 Provider 接口、官方 tokenizer 或保守估算。

未知模型：

- 生产环境拒绝静默猜测。
- 开发环境只有显式配置后才能保守回退。

### D09：用户输入本身超限

备选：

- 提示用户缩短。
- 自动摘要后继续。
- 自动转为资料再通过工具读取。

选择：

> 首期直接提示用户缩短，不自动改写用户原文。

### D10：Provider 故障策略

讨论中澄清的术语：

- Retry：同一 Provider 的尝试策略。
- Fallback：切换到同一能力的备用 Provider。
- Abort：全链耗尽后中止。
- Skip：全链耗尽后跳过该能力并降级继续。

选择：

> 每个能力拥有可组合解析链；每个 Stage 有独立 RetryPolicy；全链耗尽后执行 Abort 或 Skip。

示例：

```text
Redis History（重试）
→ MySQL History（重试）
→ Abort
```

### D11：Provider 接口形态

备选：

- 所有能力共用万能 `Load(any) (any, error)`。
- 按能力定义强类型接口。
- 每个实现自行定义接口并由 Manager 适配。

选择：

> Prompt、Memory、History、ModelCapabilities、TokenCounter 分别定义接口；可裁剪内容进入统一 `ContextItem`。

理由：

- 符合 Eino 和 Go 的能力专用接口风格。
- 避免万能请求对象和运行时类型错误。

### D12：首期 Agent 范围

备选：

- 只接入 Chat/Main。
- 接入 Chat/Main 和 Search。
- 一次迁移所有 Agent。

选择：

> 首期接入 Chat/Main 和 Search；Youdao 与 Generation 后续增加 Profile。

### D13：Search Agent 输入

当前实现事实：

- 直接搜索把 query 传给 Search Agent。
- Main 工具调用只传 `query` 参数。
- Search 不接收 Main 的完整历史或记忆。

选择：

> 保持最小披露语义，将裸字符串演进为 `SearchTask{Query string}`。

明确不传：

- 完整原始会话。
- 完整用户记忆。
- Main 的完整上下文。

### D14：用户记忆召回边界

备选：

- MemoryProvider 返回全部记录，Manager 计算相关性。
- MemoryProvider 直接决定最终注入内容。
- MemoryProvider 返回有序候选，Manager 做全局预算选择。

选择：

> MemoryProvider 负责检索、过滤和相关性排序；ContextCompiler 负责安全过滤、跨来源预算和最终注入。

ContextCompiler 不做第二遍语义检索。

### D15：会话历史策略

备选：

- 摘要 + 最近历史。
- 摘要 + 最近历史 + 旧历史语义检索。
- 完整读取再统一裁剪。

选择：

> 带 `ThroughMessageID` 和版本的增量摘要 + 尚未摘要的最近完整消息组 + Token 窗口。

旧历史语义检索以后作为独立 `HistoricalRecallProvider`，不属于首期。

### D16：Token 预算策略

讨论过的方案：

- 固定百分比。
- 全局统一竞争。
- 硬保留、分类上限和动态填充。

最终选择：

> 硬保留 + 分类最大上限 + 优先级填充 + 高低水位治理 + 边界精确计数。

性能选择：

- 快速路径使用缓存值和本地估算。
- 接近阈值才精确计数。
- 超过高水位才清理或摘要。
- 治理到低水位后停止，避免频繁压缩。

### D17：ContextManifest

备选：

- 只记录总 Token。
- 记录来源、计数、状态和裁剪原因，不保存正文。
- 保存完整 Prompt。

选择：

> 生产默认记录无正文 Manifest；最终上下文使用带密钥 HMAC 关联。

完整 Prompt 诊断不属于首期。

### D18：Provider 并发

备选：

- 全部串行。
- 全部并行。
- 按依赖阶段并行。

选择：

> 身份和 Profile 先验证；独立能力并行解析；单条回退链内部串行。

### D19：缓存所有权

选择：

> 领域数据缓存由 Provider 管理；ContextCompiler 只缓存带版本键的 Profile 编译、Prompt Token、工具 Schema Token 和模型能力等纯计算结果。

单个 Turn 内复用加载结果，ReAct 每轮不重新访问 Provider。

### D20：双上下文平面

选择：

> Go `context.Context` 只承载取消、截止时间和传播信息；userID、conversationID、AgentID 等通过强类型请求传入；只有显式构造的模型上下文会发送给 LLM。

### D21：迁移方式

备选：

- 直接替换。
- Feature Flag 二选一。
- Shadow 对比后灰度。

选择：

> `legacy` → `shadow` → `enabled`。

Shadow 不进行第二次模型调用，只生成无正文 Manifest，并按 userID 稳定采样。

### D22：总体架构

比较方案：

1. 轻量上下文生命周期协调器 + Eino Middleware。
2. 独立 Context Compiler 接管完整模型请求。
3. Eino Graph 上下文流水线。

选择：

> 方案 1，但按 D31 拆成 ContextCompiler 与 TurnLifecycleCoordinator 两个内部端口。

理由：

- 复用 Eino ReAct 和原生 Tool 消息状态。
- 改动集中于现有 Builder、ContextBuilder 和 Middleware 接入点。
- Provider、Writer、预算核心和领域存储仍能保持独立。
- ContextCompiler 覆盖 invocation 前和每次模型调用前；TurnLifecycleCoordinator 在未来 Harness 中覆盖终态写回，二者都不接管 Harness 的存储与执行权。

### D23：文档组织

选择：

> 使用一份总设计作为入口，按核心契约、Provider/Profile、预算/Eino、可观测/迁移/测试拆分模块文档，并增加本决策日志和交付工作流。

### D24：入口用户消息由谁持久化

比较方案：

1. `TurnLifecycleCoordinator.BeginTurn` 调用 MessageWriter 首次保存用户消息。
2. RunService 接受命令时原子创建 Run、入口用户消息和 Outbox，再把稳定 Handle 交给 TurnLifecycleCoordinator。

选择：

> 方案 2。RunService 生成不可变 `AcceptedTurnHandle`；Worker claim 后补充 `ActiveExecutionAuthority`，`BeginTurn` 验证二者。

理由：

- 用户输入是“系统已接受命令”的事实，应早于 Worker 执行成立。
- RunService 已拥有 Run 幂等、Outbox 和事务边界。
- 若上下文模块再负责首次写入，会产生 Run 与消息双写窗口，或迫使它吸收 Harness 事务和 fencing 职责。
- RunService 只生成稳定消息身份与会话顺序，不需要理解摘要、记忆、裁剪或 Prompt 装配。

### D25：执行后写回依赖图

选择：

```text
AssistantMessageWriter 成功
  → 同事务获得稳定 Assistant Message ID / Sequence
  → 同事务持久化 Summary/Memory/Manifest intents
  → SummaryWriter 与 MemoryWriter 可独立调度
  → ManifestWriter 记录最终状态
```

约束：

- 每个 Writer 使用 `runID + operation + revision` 等稳定幂等键。
- 摘要只推进到已持久化的完整消息边界。
- 记忆只从调用方原始输入和本轮真实输出提取，排除 Provider 注入内容，避免反馈循环。
- 需要异步执行时使用 Harness 的可靠任务机制，不启动无人接管的进程内 goroutine。
- 失败、取消或没有最终回答时，不执行成功回答的长期记忆提取。
- Search 使用 StepResultWriter，不向主会话提交 Assistant Message。

### D26：两类摘要

选择：

- 运行内压缩：在每次模型调用前按 Token 水位触发，只治理当前 Eino Agent State，可随 checkpoint 保存。
- 持久化会话摘要：在最终消息持久化后由 SummaryWriter 检查，使用消息边界和版本 CAS，供未来 Turn 读取。

运行内摘要不能直接覆盖持久化会话摘要，但可以作为 SummaryWriter 的候选输入之一。

### D27：写回崩溃窗口

问题：

> 如果 Assistant Message 已保存，但进程在创建摘要/记忆任务前退出，单纯顺序调用 Writer 无法保证补写。

选择：

> TurnLifecycleCoordinator 生成完整 `WritebackIntent` 计划；主结果 Writer 在校验 fencing 后，以同一事务提交主结果、Finalization Journal 和 Outbox intents。Repair Scanner 重试未完成 intent。

成功状态严格按 `running → finalizing → completed` 转换。`running → finalizing` 创建 FinalizationTicket、基础 Manifest intent 和 CompileRecord 引用，并刷新 ActiveExecutionAuthority；主结果和必要 durable intents 提交后才能进入 `completed`。失败/取消在终态事务中直接创建 Ticket 和 Manifest intent。

主结果使用 ActiveExecutionAuthority；派生和 Repair Worker claim Ticket 后使用独立 FinalizationAuthority。派生任务可以异步完成，不阻塞最终回答展示。

### D28：同一会话并发 Turn

选择：

> 首期沿用现有会话锁语义，同一 Conversation 只允许一个活动 Turn。RunService 必须在创建 Run 和用户消息前取得会话执行权；冲突请求直接拒绝，不创建排队用户消息。

理由：

- 避免两个交错 Turn 让摘要错误跨过未完成消息。
- 简化历史排他边界和最终消息配对。
- 未来若允许并发，必须改为 Turn 分组和“连续完成前缀”摘要协议，不能只推进到最大的 Assistant Sequence。

### D29：暂停恢复、Revision 和模式快照

选择：

- 暂停不是终态，不调用 `FinalizeTurn`，也不分配 Finalize revision。
- `PreparedTurnSnapshot`、运行内摘要和 `CompileRecord` 随 Eino checkpoint 或受控 Run-local state 保存；Resume 不重新读取可能变化的 Provider 候选。
- Harness 在首次不可恢复终态分配持久化 revision；同一终态的所有重试复用它。
- `context_mode`、`writeback_owner` 和契约版本在 Run 创建时固化，后续 Attempt/Resume 不重新读取全局 Flag。

### D30：当前代码迁移与包落位

问题：

> 原文直接引用不存在的 `internal/agentcontext`，也没有说明 ContextBuilder、Prompt 常量、ChatCache、MessageRepository 和 chatAgentService 如何迁移。

选择：

```text
internal/agentcontext/           编译核心、类型、Registry、Profile、预算
internal/agentcontext/adapter/   现有 Prompt/History/Token 能力适配
internal/agentcontext/eino/      Eino 消息渲染和 Middleware
internal/agentcontext/writeback/ 未来 Harness 生命周期与 Writer 协调
```

这些都是 W1 之后的目标路径，不是当前已有实现。迁移映射：

- `ContextBuilder` → `LegacyHistoryProvider` + ContextCompiler；Legacy 保留，Shadow 验证后 Enabled 替代 `BuildMessages`。
- `ChatAgentSystemPrompt` / `buildSystemPrompt` → 抽取单一渲染实现供 `StaticPromptProvider` 和 Legacy Builder 共同调用，首期保持渲染行为且不复制规则。
- `ChatCache` + `MessageRepository` + `ConversationRepository` → `LegacyHistoryProvider`，保留摘要/历史的 Redis → MySQL 回退和 Provider 自有缓存回填。
- `ChatAgentBuilder` → 注入 ContextCompiler 与 Eino Handler。
- `chatAgentService.saveResults` → 只作为未来 Writer 的迁移行为来源，不视为已经满足事务、幂等或 fencing 契约。

理由：

- 开发者可以从现有代码追到目标组件。
- 核心包不依赖 Redis、MySQL 或 Eino Builder。
- 迁移期间不删除当前可回滚路径。

### D31：ContextCompiler 与 Harness 生命周期拆分

问题：

> RunService、Worker、Harness、AcceptedTurnHandle 和 Authority 在仓库中不存在，而且 Harness 设计仍待用户审阅；如果 Manager 强制同时实现 Begin/Prepare/Compile/Finalize，上下文模块会被未来系统阻塞。

选择：

```text
ContextCompiler：
  PrepareTurn
  CompileModelInput
  W0–W4 可基于当前运行时独立交付

TurnLifecycleCoordinator：
  BeginTurn
  FinalizeTurn
  W5–W7 依赖获批且已落地的持久化 Harness
```

目标态可以用聚合 `Manager` 作为组装门面，但当前 `chatAgentService` 不得伪造 Run、Handle、fencing token 或跨进程 Resume 保证。W0–W4 只使用明确标识的 Legacy/Shadow Turn Adapter 或测试夹具。

理由：

- 保持上下文编译模块可立即实现和验证。
- 不把尚未批准的 Harness 当作既成事实。
- 未来接入持久化生命周期时不破坏 Provider/Profile/Budget 核心。

### D32：首期 Provider-aware TokenCounter

选择：

- Anthropic：快速路径保守估算；接近阈值调用现有 `anthropic-sdk-go` 的 `Messages.CountTokens`，标记 `exact_provider`。
- 已知 OpenAI 模型：模型注册表显式映射 `o200k_base` 或 `cl100k_base`，使用通过契约测试的 Go tiktoken-compatible 实现，标记 `compatible_local`。
- 自定义 OpenAI-compatible 模型：必须显式配置 `TokenizerStrategy` 和 `ContextWindow`，不能按名称猜测。
- 无匹配策略的开发回退：UTF-8 正文字节数加消息、角色和工具结构开销，标记 `conservative_utf8_bytes`；生产默认拒绝未知模型。
- 中文不使用英文“字符数除以四”经验值；有匹配 tokenizer 时直接编码，否则走 UTF-8 保守估算。

具体第三方 tokenizer 只有在 W3 通过中文、特殊 Token、消息封装、工具 Schema 与真实 usage 偏差测试后才能引入。编码匹配不等于完整聊天请求永久精确。

### D33：Feature Flag 与稳定分桶

选择：

> 复用现有 `pkg/config` + Viper，不新增 Feature Flag 服务。配置挂在 `AgentConfig.ContextManagement`，采样率使用 0–10000 基点。

稳定分桶：

```text
digest = SHA256("context-shadow:" + rollout_version + ":" + decimal(user_id))
bucket = BigEndianUint64(digest[0:8]) % 10000
selected = bucket < shadow_sample_basis_points
```

相同 userID 和 rollout version 始终稳定；提高采样率单调包含原样本；显式更换 rollout version 才重新分桶。当前无持久化 Run 的路径只能生成请求级快照；Harness 可用后在 Run 创建时固化 Mode、WritebackOwner 和契约版本。

## 4. 被明确排除的方向

- 不让所有 Agent 加载相同上下文内容。
- 不让 Search 直接访问完整用户记忆和会话历史。
- 不把用户记忆放进最高优先级系统指令。
- 不让 ContextCompiler 或 TurnLifecycleCoordinator 实现领域写入、领域缓存或 Harness 事务；只允许生命周期协调器调度独立 Writer。
- 不把工具定义和工具输出视为同一种上下文来源。
- 不在首期自动检索 RAG。
- 不在首期引入 Profile 动态配置平台。
- 不在生产默认记录完整 Prompt。
