# 主 Agent 全功能恢复设计

## 背景

提交 `1b995aa6` 实现了主 Agent 调度 Youdao、Search 和 Generation 子能力，以及异步事件转发、上下文注入和配置开关。后续合并提交 `bf9af4a` 在解决与 `develop` 的冲突时采用了 Builder 重构版本，导致核心接线被覆盖。部分实现文件和配置仍然存在，但当前运行路径不再注册这些能力。

本次恢复保留当前 Builder 架构和后续有效修复，将 `1b995aa6` 的全部主从协同能力适配回当前代码，而不是直接回退文件。

## 目标

- 完整恢复主 Agent 对 Youdao、Search 和 Generation 能力的调度。
- 恢复主 Agent 配置开关、总控提示词和无资料对话能力。
- 恢复子 Agent 所需的用户上下文注入。
- 恢复搜索与生成后台任务的异步事件生命周期。
- 保留当前 Builder 架构、引用原子发送、panic recovery 和后续兼容性修复。
- 确保关闭主 Agent 开关时，普通 RAG 对话行为不发生回归。

## 非目标

- 不重写 Search、Youdao 或 Generation 子系统内部实现。
- 不新增主 Agent 能力类别。
- 不改变现有前端事件协议之外的产品交互。
- 不整体回滚 `bf9af4a` 或后续合并提交。

## 方案选择

采用适配式全恢复：扩展现有 `ChatAgentBuilder`，由 Builder 统一构建普通 ChatAgent 或主 Agent。相比直接恢复旧文件，该方案不会覆盖 `develop` 后续重构；相比建立第二套 MainAgent，它避免复制对话、持久化和事件处理流程。

## 架构设计

### ChatAgentBuilder

Builder 增加以下可选配置：

- 主 Agent 是否启用；
- notebook ID；
- Youdao 子 Agent Builder；
- Search Agent Executor；
- Generation 回调。

构建流程根据模式组装工具：

- 普通模式只注册当前 RAG 和资料摘要工具；
- 主 Agent 模式在普通资料工具之外注册 Youdao、Search 和 Generation 工具；
- 没有资料时不注册依赖资料的工具，但仍允许主 Agent 工作；
- 子能力依赖为空或构建失败时记录告警并跳过该能力，不阻止其他能力启动。

Builder 根据模式选择普通系统提示词或 `MainAgentSystemPrompt`，并继续注入资料列表。主 Agent 模式提高最大迭代次数并启用子 Agent 内部事件转发。

### 子 Agent 接口与上下文

恢复 `SubAgentBuilder` 接口，其职责包括构建 Eino Agent 和注入工具执行所需的用户上下文。Youdao 子 Agent 通过 `adk.NewAgentTool` 同步调用，并使用包装工具在执行前调用 `InjectContext`。

Search 保持 `SearchAgentExecutor` 适配接口，避免 `chat` 包反向依赖 `service` 包。服务层负责把现有 Search 接口事件转换为 chat 层事件。

Generation 通过函数类型注入，避免 `chat` 包直接依赖 GenerationService。

### 工具调度

- Youdao：同步调用。主 Agent 等待操作结果，再形成回复。
- Search：异步触发。工具立即返回已启动状态，搜索事件持续发送到前端搜索面板。
- Generation：异步触发。生成结果发送到笔记面板，不阻塞主 Agent 的文本回复。
- 明确搜索命令：保留确定性路由，避免兼容模型只描述搜索行为但不发起工具调用。

现有 `trigger_tools.go` 作为 Search 和 Generation 异步触发实现继续使用，并重新接入 Builder 构建路径。

## 服务层与依赖注入

`ChatAgentService` 恢复持有 Youdao、Search 和 Generation 依赖。`app.go` 在组装服务时传入这些依赖，Search 通过适配器转换事件类型，Generation 通过闭包转换请求和返回值。

创建 ChatAgent 时读取 `main_agent_enabled`：

- 关闭时要求用户选择资料，维持普通对话语义；
- 开启时允许无资料对话，并挂载可用子能力；
- 开启且请求未提供资料时，按 notebook ID 查询全部就绪资料作为 fallback。

## 事件与生命周期

恢复主 Agent 事件和后台任务事件的分离：

1. 主 Agent token、工具事件和完成事件进入主事件通道。
2. Search 和 Generation 后台任务写入服务层 SSE 通道。
3. 主 Agent 完成后立即释放会话锁，让用户可以继续对话。
4. 服务层等待已注册后台任务结束后关闭 SSE 通道。
5. 取消 context 同时传播到主 Agent 和后台子任务。

引用继续随 `EventDone.Data` 原子发送。Search 结果通过回调转换为统一引用，并用 `sync.Once` 避免重复收集。

## 错误处理

- 用户取消与超时分别记录和上报。
- 子 Agent 构建失败只禁用对应能力，并记录结构化告警。
- 异步工具执行失败发送对应错误或结果事件，同时保证 WaitGroup 完成。
- panic recovery 保留在服务 goroutine 边界，确保释放锁、清理 cancel function 并关闭事件通道。
- 通道写入遵守 context 取消，避免客户端断开后 goroutine 永久阻塞。

## 兼容性

- 保留当前构造流程的 Builder 风格，调用方通过新增链式配置接入主 Agent 依赖。
- 保持已有 SSE 事件名称和前端消费格式。
- `main_agent_enabled=false` 时不挂载子 Agent，行为与当前普通模式一致。
- 配置缺失按关闭处理，避免部署升级后意外启用。

## 测试与验收

至少覆盖以下场景：

- 主 Agent 开关关闭时，普通 RAG 对话和资料校验正常。
- 主 Agent 开启且无资料时，可以正常聊天并调用 Search/Youdao。
- Builder 正确注册 Youdao、Search 和 Generation 三类能力。
- Youdao 工具调用前正确注入 user ID 上下文。
- 明确搜索指令能确定性触发 Search Agent。
- Search 和 Generation 在主回复完成后仍可继续发送后台事件。
- 主回复完成后会话锁及时释放，SSE 在后台任务结束后关闭。
- 用户取消能传播到主 Agent 和后台任务，且资源被清理。
- 子 Agent 构建或执行失败不会破坏其他能力。
- Search 结果只收集一次并进入最终引用。

验收以相关 Go 单元测试、`go test` 对受影响包以及项目可执行构建通过为准。若仓库现有无关测试失败，应单独记录，不与本次恢复混淆。

## 实施边界

主要修改范围限定在：

- `internal/agent/chat` 的 Builder、ChatAgent、提示词和触发工具接线；
- `internal/service/chat_agent_service.go` 的模式判断、适配器和事件生命周期；
- `internal/app/app.go` 的依赖注入；
- 与上述行为直接相关的测试。

不处理工作区内现有的无关未跟踪文件或其他功能改动。
