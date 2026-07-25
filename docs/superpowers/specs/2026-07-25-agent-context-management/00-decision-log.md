# 上下文管理讨论决策记录

## 1. 目的

本文记录本次设计讨论中逐项检验的问题、主要备选方案、最终选择和选择理由。它是设计决策的可追溯记录，不替代模块设计文档。

## 2. 决策总表

| 编号 | 决策主题 | 最终选择 |
|---:|---|---|
| D01 | ContextManager 职责范围 | 编排器；外部模块独立且可替换 |
| D02 | 上下文装配时机 | 请求级准备 + 模型调用级编译 |
| D03 | RAG 触发 | 仅工具驱动 |
| D04 | Agent 差异表达 | 统一引擎 + 强类型 ContextProfile |
| D05 | 用户记忆对主/子 Agent 的可见性 | 主 Agent 检索 + 子 Agent 最小披露 |
| D06 | 信任提升 | Provider 内容不能提升指令优先级 |
| D07 | ContextManager 写回 | 不负责写回 |
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
| D22 | 总体架构方案 | 薄 ContextManager + Eino Middleware |
| D23 | 文档组织 | 总设计 + 按模块拆分文档 + 决策日志 |

## 3. 逐项记录

### D01：ContextManager 的职责范围

备选：

- 纯装配器：调用方准备全部材料。
- 编排器：调用独立的历史、记忆、Prompt 等能力后统一装配。
- 完整上下文平台：同时负责记忆、RAG、会话和存储生命周期。

选择：

> ContextManager 是编排器。记忆、RAG、会话存储等模块独立实现并可替换，只需满足 ContextManager 使用方定义的接口。

理由：

- 满足依赖倒置和依赖注入。
- 不把领域存储职责集中到 ContextManager。
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

### D07：ContextManager 是否写回

备选：

- 只读装配。
- 统一 Commit。
- 完整管理读写生命周期。

选择：

> 只读装配。

理由：

- 摘要写回属于会话模块。
- 记忆提取和更新属于记忆模块。
- 工具结果生命周期属于 Agent Runtime。

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

> MemoryProvider 负责检索、过滤和相关性排序；ContextManager 负责安全过滤、跨来源预算和最终注入。

ContextManager 不做第二遍语义检索。

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

> 领域数据缓存由 Provider 管理；ContextManager 只缓存带版本键的 Profile 编译、Prompt Token、工具 Schema Token 和模型能力等纯计算结果。

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

1. 薄 ContextManager + Eino Middleware。
2. 独立 Context Compiler 接管完整模型请求。
3. Eino Graph 上下文流水线。

选择：

> 方案 1。

理由：

- 复用 Eino ReAct 和原生 Tool 消息状态。
- 改动集中于现有 Builder、ContextBuilder 和 Middleware 接入点。
- Provider 和预算核心仍能保持独立。

### D23：文档组织

选择：

> 使用一份总设计作为入口，按核心契约、Provider/Profile、预算/Eino、可观测/迁移/测试拆分模块文档，并增加本决策日志和交付工作流。

## 4. 被明确排除的方向

- 不让所有 Agent 加载相同上下文内容。
- 不让 Search 直接访问完整用户记忆和会话历史。
- 不把用户记忆放进最高优先级系统指令。
- 不让 ContextManager 同时承担读写和领域缓存。
- 不把工具定义和工具输出视为同一种上下文来源。
- 不在首期自动检索 RAG。
- 不在首期引入 Profile 动态配置平台。
- 不在生产默认记录完整 Prompt。
