# Agent Runtime Milestones

状态：草案

目标：把 agent-gogo 的早期实现拆成可验收的里程碑，先打稳 Runtime Core、事实源、上下文装配和能力边界，再逐步接入真实 LLM、浏览器、Skill、Persona 和 Memory。

原则：

1. 完整架构，薄实现。
2. 每个里程碑都必须有可运行验证。
3. 先做稳定事实源，再做智能能力。
4. Runtime Core 不直接绑定 UI、模型、浏览器或外部工具。
5. 所有副作用必须经过 Tool Runtime 或 Communication Runtime。

---

## M1：domain + store

目标：建立所有模块依赖的领域实体、状态机和 SQLite 持久化基础。

范围：

1. 定义核心实体：`Project`、`Task`、`TaskDependency`、`TaskAttempt`、`TaskEvent`、`ToolCall`、`Observation`、`TestResult`、`ReviewResult`、`Artifact`。
2. 定义 Task 状态机：`DRAFT`、`READY`、`IN_PROGRESS`、`IMPLEMENTED`、`TESTING`、`REVIEWING`、`DONE`，以及异常状态。
3. 建立 SQLite schema 和 migration。
4. 实现 store repository 的最小接口。
5. 记录 append-only `TaskEvent`。
6. 为状态迁移、attempt 创建、event 写入补单元测试。

交付物：

1. `internal/domain`
2. `internal/store`
3. `migrations`
4. 状态机单元测试
5. SQLite schema 测试或 migration smoke test

验收标准：

1. 可以创建 Project 和 Task。
2. 可以为 Task 创建 TaskAttempt。
3. Task 状态只能按允许路径迁移。
4. 非法状态迁移会返回明确错误。
5. TaskEvent 只能追加，不能覆盖历史。
6. `go test ./...` 通过。

非目标：

1. 不接 LLM。
2. 不实现 Planner 智能拆解。
3. 不实现 Web Console 页面。

---

## M2：contextbuilder + 缓存层模型

目标：先不接 LLM，用模拟数据验证上下文分层、确定性序列化和缓存失效逻辑。

范围：

1. 定义 `ContextPack`、`ContextLayer`、`LayerKey`、`ContextSerializer`。
2. 实现 L0 / L1 / L2 / L3 分层模型。
3. 实现稳定排序和确定性序列化。
4. 实现 LayerKey 生成。
5. 实现缓存失效判断。
6. 用模拟 Project、Task、Function、Skill、Memory 数据写单元测试。

交付物：

1. `internal/contextbuilder`
2. Context layer 数据结构
3. Context serializer
4. LayerKey 生成器
5. 确定性序列化测试
6. 缓存失效测试

验收标准：

1. 相同输入在多次运行中序列化结果字节级一致。
2. L0 / L1 / L2 不包含当前时间、随机 ID、请求 ID、最新工具结果等动态内容。
3. L3 动态内容变化时，不影响 L0 / L1 / L2 的 LayerKey。
4. 集合字段按稳定 ID 或 name 排序。
5. `go test ./...` 通过。

非目标：

1. 不接真实 LLM。
2. 不实现 token 估算。
3. 不实现向量检索。

---

## M3：最小 Runtime 闭环

目标：打通 `CreateProject` 到 `RunNextTask` 的最小执行闭环，哪怕 Planner 先返回固定任务，也必须经过 store、attempt、event 和状态迁移。

范围：

1. 实现 `RuntimeService` 骨架。
2. 实现 `CreateProject`。
3. 实现固定任务 Planner。
4. 实现 Task Validator 最小版本。
5. 实现 Scheduler 选择下一个 `READY` task。
6. 实现 Executor 最小版本，创建 TaskAttempt 并把任务推进到 `IMPLEMENTED`。
7. 实现 Tester / Reviewer 最小通过版本，把任务推进到 `DONE`。

交付物：

1. `internal/runtime`
2. `internal/planner`
3. `internal/validator`
4. `internal/scheduler`
5. `internal/executor`
6. `internal/tester`
7. `internal/reviewer`
8. Runtime service 集成测试

验收标准：

1. 调用 `CreateProject` 可以创建项目。
2. 调用 `PlanProject` 可以生成固定任务并写入 store。
3. 调用 `RunNextTask` 可以创建 attempt、记录 event、推进状态。
4. 单个任务可以从 `DRAFT` 最终走到 `DONE`。
5. 事件日志能还原关键执行过程。
6. `go test ./...` 通过。

非目标：

1. Planner 不需要接 LLM。
2. Executor 不需要真实调用外部工具。
3. Reviewer 不需要 LLM 判断。

---

## M4：function + tool 最小实现

目标：实现 mock function 和 mock tool 的按需发现、schema 加载、active function set 与审计记录。

范围：

1. 定义 FunctionCard、FunctionSchema、ActiveFunctionSet。
2. 实现 mock Function Registry。
3. 实现 `function.search`。
4. 实现 `function.load_schema`。
5. 实现 Tool Runtime 最小调用接口。
6. 将 ToolCall 写入 store。
7. 将 active function set 接入 ContextBuilder。

交付物：

1. `internal/function`
2. `internal/tools`
3. mock function registry
4. mock tool runtime
5. function search/load schema 测试
6. tool call 审计测试

验收标准：

1. `function.search` 只返回轻量 FunctionCard。
2. 完整 schema 只在 `function.load_schema` 时加载。
3. ContextBuilder 只接收 active function schemas。
4. ToolCall 记录输入、输出、状态、错误和 evidence 引用。
5. `go test ./...` 通过。

非目标：

1. 不接真实 shell、浏览器或文件系统工具。
2. 不实现语义搜索。
3. 不执行高风险副作用。

---

## M5：communication 基础版

目标：先抽象 CLI / Web Console 的基本通讯能力，确保 Runtime 与通道解耦。

范围：

1. 定义 ChannelEvent、CommunicationIntent、ChannelCapability。
2. 实现 Communication Runtime 最小接口。
3. 实现 CLI adapter 的 `send_message` 和 `ask_confirmation`。
4. 实现 Web Console adapter 的抽象占位。
5. 实现 Message Renderer 最小版本。
6. 实现 outbox 的本地记录。
7. 将 Runtime 的用户输出改为 CommunicationIntent。

交付物：

1. `internal/communication`
2. CLI communication adapter
3. Web Console communication adapter 占位
4. outbox store
5. communication intent 测试

验收标准：

1. Runtime 不直接调用 CLI 或 Web 细节。
2. `send_message` 可以根据 channel 渲染。
3. `ask_confirmation` 可以根据 channel capability 选择实现或返回不可用。
4. 高风险操作可以生成 confirmation intent。
5. `go test ./...` 通过。

非目标：

1. 不接 Telegram、WhatsApp、Email。
2. 不做完整 Web Console UI。
3. 不做完整多用户权限系统。

---

## M6：真实能力接入

目标：在前五个里程碑的接口稳定后，逐步接入真实 LLM、Chrome MCP、Skill 索引、Persona 和 Memory。

范围：

1. 实现 LLM Provider 接口和一个真实 provider。
2. 接入 Chrome MCP 或等价 Browser Provider。
3. 实现 Claude-compatible Skill Discovery / Parser / Index / Loader。
4. 实现 Persona Registry / Router / Loader。
5. 实现 Memory Index / Retriever 的最小版本。
6. 将真实 provider 接入 Runtime，但不破坏已有 mock 测试。

交付物：

1. `internal/provider`
2. `internal/browser`
3. `internal/skill`
4. `internal/persona`
5. `internal/memory`
6. provider contract tests
7. 真实能力 smoke tests

验收标准：

1. 真实 provider 通过接口接入，不污染 Runtime Core。
2. Skill 兼容 Claude-style `SKILL.md` package，不自创格式。
3. Browser Runtime 可以采集 URL、DOM summary、截图或等价 evidence。
4. Memory / Skill / Persona 都按需加载，不全量进入上下文。
5. mock provider 测试和真实 provider smoke test 都通过。

非目标：

1. 不一次性实现所有 provider。
2. 不追求完整浏览器 Agent 能力。
3. 不做 SaaS 级别权限系统。

---

## Suggested Order

```text
M1 domain + store
  -> M2 contextbuilder + cache layers
  -> M3 minimal runtime loop
  -> M4 function + tool minimal implementation
  -> M5 communication baseline
  -> M6 real capability integration
  -> M7 story workflow integration
```

M1 和 M3 是执行系统的骨架，必须保持简单、可测、可恢复。M2 决定后续 Function、Skill、Persona、Memory 是否能保持低成本和缓存友好。M4 和 M5 负责把能力调用和用户通讯从 Runtime Core 中解耦。M6 只在接口稳定后接入真实外部系统。

---

## M7：DAG + Context Assets + Story Workflow

目标：把前六个里程碑中的薄模块串成符合 PRD 的端到端执行链路，先以“短篇推理小说编写”为验收场景，验证 DAG、Function Search、Skill、Persona、Memory、Tool Runtime、Communication 和日志事实源可以协同工作。

当前状态：已实现主链路，并通过 `go test ./...`。DAG 依赖保存与调度、Runtime Context Assets、本地 `SKILL.md` 索引加载、运行时小说家 persona 生成、Tool Runtime 安全门禁、StoryExecutor、BrowserExecutor 抽象、demo 迁移、JSONL 日志、README/config 示例已同步。两个差异较大的 Claude-compatible story skills 已从线上拉取到 `.claude/skills`，真实验收运行仍依赖环境提供 DeepSeek API key。

范围：

1. 保存 Planner 返回的 `depends_on`，将其转为 `TaskDependency`，Scheduler 只选择依赖已 `DONE` 的 `READY` task。
2. Runtime Service 在规划前接入 Function Search、Skill Search/Load、Persona Search/Load、Memory Search，并通过 `ContextBuilder` 生成稳定上下文。
3. Tool Runtime 增加 Capability Resolver、安全策略和确认门禁；高风险工具必须经过 confirmation gate，shell 默认由配置显式允许。
4. 将特定网页答案 demo 从通用 executor 层移出，保留为 demo/example；新增通用 BrowserExecutor 只负责浏览器证据采集。
5. Skill 使用本地索引加载；本仓库只保留从线上拉取的 Claude Code / Claude-compatible `SKILL.md` 包，不手写创建。
6. 新增小说工作流命令：输入用户目标后，由 agent 在运行时生成小说家 ephemeral persona，检索并加载本地 skill 索引，通过 function/tool 写入文档，将重点保存到 memory，并把最终内容发送到配置 channel。
7. 所有链路阶段和 LLM 提示词必须写入日志文件夹，日志不得包含 API key。
8. 同步 README、配置示例和里程碑状态。

验收输入：

```text
我希望完成一个短篇推理小说的编写
```

验收标准：

1. DeepSeek 通过 provider 接口接入，API key 只从 `DEEPSEEK_API_KEY` 或 `AGENT_GOGO_LLM_API_KEY` 注入，不写入仓库、日志或生成文档。
2. Chain Router、Intent Analyzer、Function Search、Skill Search、Persona 创建/选择、Memory Search、ContextBuilder、Planner、Executor、Tool Runtime、Tester、Reviewer、Communication output 均有日志记录。
3. 至少一个 active skill 来自本地索引中的 Claude Code / Claude-compatible `SKILL.md` package，且完整 skill body 只在激活时进入 ContextPack。
4. Function Search 返回轻量 cards；只有 active functions 的 schema 进入 ContextPack。
5. 小说正文通过 function/tool 写入 artifact 文档。
6. 故事重点、人物关系、线索、伏笔和事实约束通过 memory tool 保存。
7. Runtime Core 不直接写文件或发送消息；副作用经 Tool Runtime 或 Communication Runtime。
8. `go test ./...` 通过。

非目标：

1. 不实现完整 Web Console 页面。
2. 不实现通用长篇小说生产系统。
3. 不把真实 API key 固化到配置文件或示例文件。
