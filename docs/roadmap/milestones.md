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
  -> M8 code runtime + engineering tools
  -> W9 task awareness + generic agent memory
  -> M10 generic agent loop
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

---

## M8：Code Runtime + Engineering Tools

目标：在不破坏 Runtime Core 稳定边界的前提下，补齐“理解仓库、修改文件、运行测试、失败修复、Git 隔离”的信息工程能力，让 agent-gogo 可以参与自身代码迭代，并用“苏柯宇个人网页”作为真实端到端验收任务。

当前状态：已实现并通过本地验证。Code Runtime、工程工具、真实测试反馈、薄 Repair loop、README/config 同步和个人网页验收命令已落地；个人网页部署产物已写入 `web/dist/sukeyu` 并通过本地 HTTP 请求验证。

范围：

1. 新增 Code Runtime 薄实现：遍历 workspace 构建 repo map，提供文件摘要、语言统计和简单符号索引。
2. 扩展 Function Catalog / Tool Runtime：
   - `code.index`：生成仓库地图与符号摘要。
   - `code.symbols`：按文件或 query 返回函数、类型、结构体等符号。
   - `file.read`：读取 workspace 内文件。
   - `file.write`：写入 workspace 内文件，受路径安全策略约束。
   - `file.diff`：返回文件或目录的 git diff。
   - `file.patch`：对 workspace 文件应用受控补丁。
   - `shell.run`：仅在 config 显式允许且命令匹配 allowlist 时执行。
   - `git.branch`、`git.diff`、`git.status`、`git.commit`、`git.rollback`：提供基础 Git 集成，其中回滚必须安全受限。
3. 改造测试反馈：新增真实 `CommandTester`，通过 Tool Runtime 执行测试命令并根据退出结果生成 PASSED / FAILED。
4. Repair / Replan 薄闭环：测试失败或 reviewer 驳回时生成关联原任务的 fix task，并把失败证据写入 task event。
5. 同步配置：增加 workspace root、artifact root、shell allowlist、默认测试命令、Git 工具开关示例。
6. 同步 README：说明 M8 工程工具和验证命令。
7. 用 M8 工具完成一个静态站点任务：生成“苏柯宇”的个人网页，构建到部署目录，并完成本地静态部署验证。

验收输入：

```text
为苏柯宇写一个个人网页并完成部署
```

验收标准：

1. Runtime 可以通过 `code.index` 和 `code.symbols` 读取当前仓库结构。
2. 文件副作用只能通过 Tool Runtime 的 file tools 发生，并限制在 workspace root 内。
3. Shell 执行默认关闭；开启后也只能运行 allowlist 命令。
4. 测试执行使用真实命令结果，失败时 Tester 标记 FAILED 并生成可追踪证据。
5. Repair / Replan 至少能在失败时创建 fix task 并保留失败输出。
6. Git 工具能读取 status / diff，并能在允许时创建隔离分支；危险回滚不自动执行。
7. 个人网页源码、部署产物和验证日志均可追踪；本地部署可被 HTTP 请求验证。
8. `go test ./...` 通过。

非目标：

1. 不实现完整 LSP/gopls 级别语义分析。
2. 不实现跨语言精准引用图。
3. 不自动执行破坏性 Git 回滚或强制提交。
4. 不接入外部部署平台凭据；M8 的部署验收以仓库内静态部署目录和本地 HTTP 验证为准。

---

## W9：Task Awareness + Generic Agent Memory

目标：补齐 agent-gogo 的任务感知能力，让通用 Agent 在规划、执行、测试、评审和修复时都能理解“项目进行到哪里了、已经知道了什么、之前为什么失败或调整”，而不是只看到当前孤立任务。

当前状态：已实现并通过本地测试。W9 在 M8 的工程工具基础上增加项目级事实摘要、自动记忆提升、自动记忆检索和决策追溯，并注入 ContextBuilder 的 L2 层，供所有工作流复用。

范围：

1. 新增 Project Digest：
   - 汇总项目目标、任务状态计数、DAG 依赖位置、已完成任务、失败/阻塞任务、最近事件、观察、测试、评审和 artifact/tool evidence。
   - 在 `PlanProject` 与 `RunNextTask` 前生成确定性摘要，写入 ContextPack L2 的 `ProjectState`。
2. 新增 Task Awareness：
   - 当前任务上下文包含依赖任务、被依赖任务、兄弟任务状态、尝试次数、最近失败/修复原因和前序观察摘要。
   - Executor 获取的 Runtime Context 自动包含这些信息。
3. 自动 Memory 提取：
   - Reviewer 通过后，从 TaskEvent、Observation、ToolCall、TestResult、ReviewResult 中提取低风险项目记忆。
   - 记忆记录来源 TaskID / AttemptID / EvidenceRef，形成可追溯链。
4. 自动 Memory 检索：
   - ContextBuilder 组装前，根据 IntentProfile、Project Digest 和当前任务摘要检索相关 Memory。
   - 自动加载的 Memory 进入 `RelevantMemories`，并保持稳定排序和可缓存性。
5. 决策追溯：
   - Reviewer 通过/驳回、Tester 失败、Repair 生成等事件被提升为 decision/failure/repair 类型记忆候选。
6. 通用化要求：
   - 不绑定 story、web、code 任一特定工作流。
   - 不让 Executor 绕过 Runtime/Store/ContextBuilder 直接拼接全局状态。
7. CLI 入口通用化：
   - `agent-gogo` 主入口只接自然语言目标，不暴露 story / personal-site / plan 等显式演示命令。
   - 演示工作流可以保留为内部 demo/workflow 函数，由 Runtime 根据用户目标自动路由。

验收输入：

```text
运行一个至少两步的项目：第一步产生观察或决策，第二步执行前 ContextPack 必须能看到第一步的 digest 和自动 memory。
```

验收标准：

1. `go test ./...` 通过。
2. `ProjectState` L2 中包含结构化 `digest`，并能回答完成数量、失败数量、当前卡点、最近证据。
3. `TaskState` L2 中包含当前任务 DAG 位置、依赖、被依赖、兄弟任务和最近尝试摘要。
4. 成功任务完成后自动生成至少一条 project scope memory，并记录 source task / attempt / evidence。
5. 下一次规划或执行前可以检索到自动生成的 memory，并注入 `RelevantMemories`。
6. Repair / Reviewer / Tester 的失败或驳回信息能进入 digest，并在后续上下文中可见。
7. 所有新增摘要由既有事实源确定性生成，不引入当前时间、随机 ID 或 LLM 自由改写。
8. 用户可以直接运行 `agent-gogo "为苏柯宇写一个个人网页并完成部署"`，不需要记忆或输入显式子命令。

非目标：

1. 不实现向量数据库。
2. 不实现长期跨仓库用户画像。
3. 不把所有历史原文塞进上下文；只注入摘要、证据引用和可追溯 memory。
4. 不引入新的外部服务。

---

## M10：Generic Agent Loop

目标：把 W9 之后的 Runtime 零件收敛成真正的通用执行脑，让代码任务、网页任务、文档任务都走同一套“结构化规划 -> 能力/工具选择 -> action loop -> Observer -> Tester/Reviewer -> 修复或完成”的主路径，而不是依赖关键词 demo workflow。

当前状态：M10.1 已落地并通过本地测试和 DeepSeek 真实 smoke。主路径已经具备结构化 JSON 请求、schema repair、DeepSeek `json_schema` 不可用时的 `json_object` 降级、Provider Registry、规划前研究/反思任务注入、Executor 工具别名归一、重复 action fingerprint guard、只读文件任务 auto-finish，以及代码修改任务的机械验收防早停。

### M10.1：Structured Output + Progress Guard

范围：

1. 为 `ChatRequest` 增加 `response_format`、JSON schema、tool schema 和 temperature 字段。
2. OpenAI-compatible provider 透传结构化输出和 tool schema；DeepSeek 不支持 `json_schema` 时自动降级到 `json_object`，schema 继续进入 prompt。
3. 新增统一 `llmjson.ChatObject`，所有 router / intent / planner / executor / reviewer 的 JSON 输出都走结构化请求和一次 schema repair。
4. Planner 对中高复杂度、代码、网页、文档、调试、修复类任务强制补“研究上下文与可用资料”和“反思任务拆解与验收口径”任务，避免无资料直接猜测拆解。
5. GenericExecutor 只接受可用工具和常见别名归一后的工具名，并把 `read_file` / `write_file` / `edit_file` / `run_tests` 等概念名映射到真实 tool。
6. GenericExecutor 增加 action fingerprint guard，同一 tool + args 重复超过阈值时阻断，避免一直读同一文件耗到 max-step。
7. 只读文件任务在 `file.read` 成功后可以机械 auto-finish；代码修改任务如果 acceptance 提到 gofmt、compile、build、syntax、lint、signature 等机械验收，不允许仅凭 patch 过早完成。
8. 新增默认跳过的 DeepSeek smoke test，用真实 LLM 跑代码、网页、文档三类规划验收。

验收标准：

1. `go test ./...` 通过。
2. `RUN_DEEPSEEK_SMOKE=1 DEEPSEEK_API_KEY=... go test ./internal/planner -run TestDeepSeekPlannerStructuredSmoke -count=1 -v` 通过，且覆盖 code / web / document 三类规划。
3. DeepSeek 不支持 `response_format.type=json_schema` 时，Runtime 自动降级并继续完成结构化 JSON 解析。
4. Planner 生成的复杂任务 DAG 必须先有研究任务，再有反思任务，后续实现任务依赖反思任务。
5. Executor 遇到重复 action 会记录 rejected event，并允许模型改走有进展的下一步。
6. 读取文件类任务不再完全依赖模型自称 finish。
7. 有明确机械验收要求的代码任务不能仅凭 `file.patch` / `file.write` 进入 implemented。

非目标：

1. 不在 M10.1 完整替换 Validator 的能力校验；Capability Resolver 接入规划期校验留到 M10.2。
2. 不完成完整 Web Console 操作台；M10.1 只处理 Generic Agent Loop 的协议和执行保护。
3. 不清理所有 demo workflow；旧 workflow 保留为兼容和回归样本，主路径继续收敛到 GenericExecutor。
4. 不把 Reviewer / Tester 升级成完整语义断言系统；M10.1 只补关键机械证据门槛。

### M10.2：Capability Resolver + Validator Gate

目标：把 `internal/capability` 真正接进 Validator 和 Planner 输出校验，在规划期检查任务所需 capability 是否存在、可用、需确认或被策略禁用，并在缺能力时明确 ask user / degrade / block。

当前状态：已完成 M10.2 的硬化补丁。当前基线已包含外置 prompt、provider timeout、ContextMaxChars、CapabilityTaskValidator、code index cache、GenericEvidenceTester / EvidenceReviewer 默认链路、TESTING/REVIEWING 失败转 FAILED 修复、demo 入口委托 Generic 路径。本次补丁继续补强两处执行硬边界：研究/调研任务必须有真实 discovery tool 成功证据，code index cache 只在写入/patch 后失效，读取文件不再误清缓存。

范围：

1. Capability Resolver 通过 `CapabilityTaskValidator` 接入 CLI/Web 主路径，在规划任务进入 READY 前校验 browser/read/write/execute/verify/memory 等能力可用性。
2. 研究/调研类任务的验收不再只听模型声明；Tester 要求成功调用 `code.index`、`code.search`、`code.symbols`、`file.read`、`browser.open`、`browser.extract`、`browser.dom_summary`、`git.status` 或 `git.diff` 之一。
3. code index cache 对 `code.index` / `code.symbols` 复用仓库索引；`file.read` 不清缓存，`file.write` / `file.patch` 清缓存。
4. 保留 M10.1 的 DeepSeek code / web / document 三方向 smoke，验证规划仍能生成研究与反思任务。
5. 记录 M10.2 结果文档，避免验收只停留在口头状态。

验收标准：

1. `go test ./internal/tester` 通过，证明研究任务必须有真实 discovery tool evidence。
2. `go test ./internal/tools` 通过，证明 code index cache 命中与失效边界正确。
3. `go test ./internal/validator ./internal/runtime` 通过，证明 capability validator 和失败修复链路仍可用。
4. `RUN_DEEPSEEK_SMOKE=1 DEEPSEEK_API_KEY=... go test ./internal/planner -run TestDeepSeekPlannerStructuredSmoke -count=1 -v` 通过，覆盖 code / web / document 三类真实 LLM 规划。
5. `go test ./...` 通过。

### M10.3：Hard Observer + Acceptance Checks

目标：补强 Observer / State Interpreter，让工具结果能转成更硬的状态证据，包括文件 diff 是否真实存在、测试失败原因、浏览器页面是否满足目标、文档产物是否包含目标内容，以及 reviewer 可引用的机械断言。

### M10.5：Web Console Session 管理 + Project 看板

目标：将 session 管理和 project 展示放入 Web Console。Session 页面支持列表、状态筛选、详情面板（含运行时上下文）。Project 页面改造为 Jira 风格看板，按 Active / Completed / Archived 分列展示，每个卡片显示任务进度条和统计。

当前状态：已实现。后端新增 session API（GET /api/sessions、GET /api/sessions/:id、GET /api/sessions/:id/context），前端新增 SessionsView 和 Jira 风格 ProjectsView，侧边栏导航已更新。

范围：

1. 后端：SessionStore 接口、session API 路由和 handler、ListSessions store 方法、APIServer 注入 SessionStore。
2. 前端：Session/SessionContext 类型定义、API 客户端 session 方法、mock 数据。
3. SessionsView：状态筛选栏、session 列表（通道图标、标题、状态、最后活跃）、详情面板（基础信息网格、项目链接、运行时上下文）。
4. ProjectsView：Board / List 切换、三列看板（Active / Completed / Archived）、卡片含进度条和任务统计。
5. 路由和导航：/sessions 路由、侧边栏 Sessions 项、StatusBadge 新增 PAUSED/EXPIRED 颜色。

验收标准：

1. `go build ./...` 通过。
2. Web Console 侧边栏显示 Sessions 导航项。
3. Sessions 页面列出所有 session，支持状态筛选。
4. 点击 session 展开详情面板，显示运行时上下文。
5. Projects 页面默认 Board 视图，按 Active / Completed / Archived 分列。
6. 每个 Project 卡片显示任务进度条和统计。
7. Board / List 视图可切换。

非目标：

1. 不实现 session 暂停/恢复/过期清理的操作按钮（需后端 POST 接口）。
2. 不实现 project 看板拖拽排序。
3. 不实现实时 SSE 推送 session 状态变化。

### M10.6：Generic Agent Loop 收尾与 Web Console 操作闭环

目标：给 M10 收尾，补齐结构拆分、规划前探测、分层任务元数据、Web Console 管理 API、session 恢复执行和 code index 磁盘缓存，让通用 agent 主路径更接近 PRD/架构定义。

当前状态：已实现。`internal/app/app.go` 拆出 CLI/Web/provider/browser 组装，`internal/tools/runtime.go` 拆出类型、注册表和内置工具注册；Planner 输出支持 `phases` 与任务级 `required_capabilities`；Runtime 在 Planner 前运行只读 DiscoveryLoop；Validator 优先使用结构化能力字段；Web Console 配置、Skills、Personas、Memory、session 操作和确认 ID 已接后端；code index cache 可持久化到 `data/code_index.json`。

范围：

1. 结构拆分：`app/cli.go`、`app/providers.go`、`app/web.go`、`tools/types.go`、`tools/registry.go`、`tools/builtin_runtime.go`、`runtime/context_assets.go`。
2. 规划质量：新增 `internal/discovery`，在 Planner 调用前用 `code.index`、`code.search`、`file.read`、`git.status` 和 memory search 收集上下文并注入 planning context。
3. 分层任务：Task 持久化 `phase` 与 `required_capabilities`，Planner schema/prompt 要求 `phases + tasks`，Capability Validator 精确校验结构化能力。
4. Web Console：`POST /api/config` 与 `/config {...}` 命令热更新安全策略和上下文预算；Skills/Personas/Memory 页面读取真实 registry/index；确认 SSE 携带 `confirmation_id`。
5. Session 操作：新增 Pause / Resume / Expire / Delete POST 接口，Resume 会触发关联 project 的 ready task 继续执行。
6. 工程缓存：code index cache 落盘并在写入/patch 后失效，重启后可复用。

验收标准：

1. `go test ./...` 通过。
2. `cd web/frontend && npm run build` 通过。
3. `RUN_DEEPSEEK_SMOKE=1 DEEPSEEK_API_KEY=... go test ./internal/planner -run TestDeepSeekPlannerStructuredSmoke -count=1 -timeout=5m` 通过，覆盖代码、网页、文档三类真实 LLM 规划。
4. Web Console API 验证：`/api/config` 可读写，`/api/skills` 返回真实 skill registry，`/api/memory` 返回真实 memory index。
5. Computer Use 验证：本地打开 `http://127.0.0.1:18080`，Dashboard、Skills、Memory、Config 页面可见且数据正常渲染。

非目标：

1. 不在 M10.6 做完整语义级 Tester/Reviewer 断言系统。
2. 不把 Web Console 做成完整可编辑 Skill/Persona/Memory 管理器；本阶段先完成只读真实数据与 session/config 操作闭环。
3. 不删除历史 demo 包；主入口和 Web/CLI 主路径已经走 Generic Runtime。
