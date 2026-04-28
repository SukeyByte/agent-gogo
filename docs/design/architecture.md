# Agent Runtime 架构草案

状态：草案

目标：定义 agent-gogo 的长期架构边界。v0.1 可以薄实现，但模块边界、能力接口、事件流、索引策略和安全边界必须从第一天完整设计。

核心判断：

> agent-gogo 是一个 capability-oriented modular agent runtime。LLM 负责判断和生成，Runtime 负责状态机、副作用边界、上下文装配和可恢复执行。

---

## 1. 总体分层

```text
External Communication Channels
  Web Console / CLI / HTTP API / Telegram / WhatsApp / Email / Webhook
        |
        v
Communication Layer
  Channel Adapter
  Channel Runtime
  Channel Capability Registry
  Message Renderer
  Outbox / Delivery Receipt
        |
        v
Application Layer
  Session Service
  Project Service
  Runtime Service
  Config Service
        |
        v
Runtime Core
  Chain Router
  Intent Analyzer
  Function Search / Capability Resolver
  Persona Router
  Skill Router
  Memory Router
  Context Builder
  Planner / Validator / Gap Finder
  Task DAG / Scheduler
  Executor
  Observer
  Tester
  Reviewer
  Repair / Replan
        |
        v
Capability Modules
  Communication Tools
  Function Runtime / Tool Runtime
  Browser Runtime
  Code Runtime
  Document Runtime
  Artifact Runtime
  Skill Runtime
  Persona Runtime
  Memory Runtime
  Security Runtime
        |
        v
Provider / Adapter Layer
  LLM Provider
  Embedding Provider
  Browser Provider
  Function / Tool Provider
  Communication Provider
  Storage Provider
        |
        v
Persistence Layer
  SQLite
  Event Log
  Artifact Files
  Claude-compatible Skill Packages
  Persona Files
  Memory Indexes
```

Runtime Core 只表达执行语义，不绑定 UI、不绑定具体模型、不直接操作外部世界。

---

## 2. 架构原则

1. 完整架构，薄实现。
2. Runtime Core 编排能力，不拥有外部系统细节。
3. 每个模块对外暴露 capability interface，不暴露内部实现。
4. Function、Skill、Persona、Memory 都采用索引 + 路由 + 按需加载，不全量进入上下文。
5. 所有副作用必须经过 Tool Runtime 或 Communication Runtime。
6. Task 是事实实体，不是聊天记录。
7. Executor 只能产出执行结果和证据，不能单独决定 DONE。
8. Event Log 是恢复、调试、审计、Web Console 展示的事实源。

---

## 3. 核心数据流

### 3.1 请求进入

```text
External Channel
→ Channel Adapter
→ Channel Event
→ Session Resolver
→ Runtime Service
→ Chain Router
→ Intent Analyzer
```

Channel 不是简单入口适配器。不同 Channel 面向不同通讯工具，会给 AI 暴露不同的用户沟通能力。

示例：

```text
Web Console:
  send_message
  show_task_card
  ask_confirmation_modal
  stream_logs
  request_file
  edit_task_dag

Telegram:
  send_message
  send_image
  send_file
  inline_keyboard
  callback_button
  typing_indicator

WhatsApp:
  send_message
  send_media
  template_message
  quick_reply
  business_policy_limited_followup

CLI:
  print_text
  prompt_input
  confirm_yes_no
  show_table
```

### 3.2 上下文装配

```text
User Request
→ Chain Router
→ Intent Analyzer
→ Function Search
→ Capability Resolver
→ Persona Router
→ Skill Router
→ Memory Router
→ Context Builder
→ Planner / Executor / Reviewer
```

这条链路的重点是：系统内可以全量索引，但 LLM 上下文只接收当前任务需要的最小集合。

### 3.3 任务执行

```text
Scheduler
→ Create TaskAttempt
→ Executor
→ Tool Runtime / Communication Tools / Capability Module
→ Observation
→ Tester
→ Reviewer
→ DONE / FIX_TASK / REPLAN / NEED_USER_INPUT / FAILED
```

Executor 只负责执行 attempt。Task 是否完成由 Tester 和 Reviewer 基于验收标准和 evidence 判断。

### 3.4 对用户输出

```text
Runtime / Agent
→ Communication Intent
→ Channel Capability Check
→ Message Renderer
→ Outbox
→ Channel Adapter
→ External Channel
→ Delivery Receipt
```

同一个 `ask_confirmation` 在不同 Channel 中有不同渲染：

```text
Web Console → Approve / Reject modal
Telegram    → inline keyboard
WhatsApp    → quick reply 或 template message
CLI         → y/N prompt
HTTP API    → pending_confirmation response
```

---

## 4. Domain Model

第一批核心实体：

```text
Project
Task
TaskDependency
TaskAttempt
TaskEvent
ToolCall
Observation
TestResult
ReviewResult
Artifact
ChatMessage
ChannelEvent
CommunicationIntent
MemoryItem
Persona
SkillPackageRef
```

推荐关系：

```text
Project
  has many Tasks

Task
  has many TaskDependencies
  has many TaskAttempts
  has many TaskEvents
  has many AcceptanceCriteria

TaskAttempt
  has many ToolCalls
  has many Observations
  has many TestResults
  has many ReviewResults
  has many Artifacts

TaskEvent
  append-only record of state changes, decisions, tool calls, user actions,
  failures, repairs, replans, and confirmations
```

`TaskAttempt` 必须是一等实体。Retry、repair、人工接管、失败分析和审计都需要回答“第几次尝试发生了什么”。

---

## 5. Task 状态机

```text
DRAFT
  ↓
READY
  ↓
IN_PROGRESS
  ↓
IMPLEMENTED
  ↓
TESTING
  ↓
REVIEWING
  ↓
DONE
```

异常状态：

```text
BLOCKED
NEED_USER_INPUT
REVIEW_FAILED
FAILED
CANCELLED
```

状态原则：

1. Planner 只能生成 DRAFT。
2. Validator 通过后才能进入 READY。
3. Scheduler 只能选择依赖已完成的 READY task。
4. Executor 执行后进入 IMPLEMENTED，而不是 DONE。
5. Tester 通过后进入 REVIEWING。
6. Reviewer 通过后进入 DONE。
7. Reviewer 失败可以生成 Fix Task。
8. 权限、验证码、缺少信息等进入 NEED_USER_INPUT 或 BLOCKED。

---

## 6. Communication Layer

Communication Layer 负责双向用户通讯，而不只是接收请求。

### 6.1 职责

1. 适配真实通讯工具协议。
2. 管理 channel、session、user、project 的映射。
3. 暴露当前 channel 可用的用户沟通能力。
4. 将 Runtime 的 communication intent 渲染成通道支持的消息。
5. 维护 outbox、delivery receipt、重试、限流和失败记录。
6. 为高风险操作提供确认通道。

### 6.2 Channel Capability

```text
ChannelCapability
  channel_type
  supported_message_types
  supported_interactions
  max_message_length
  max_buttons
  file_size_limit
  supports_async_reply
  supports_sync_prompt
  supports_confirmation
  supports_file_request
  supports_streaming
  policy_limits
```

Runtime 不应该直接知道 Telegram Bot API 或 WhatsApp API，只能看到抽象沟通能力：

```text
send_message
send_progress
send_artifact
ask_user
ask_confirmation
send_options
request_attachment
notify_done
notify_blocked
```

这些能力可以作为 Communication Tools 暴露给 Agent，但底层必须走 Communication Runtime。

---

## 7. Intent、Function Search 与 Capability

Intent Analyzer 负责把用户请求转成结构化意图。

```text
IntentProfile
  task_type
  complexity
  domains
  required_capabilities
  risk_level
  needs_user_confirmation
  grounding_requirement
  confidence
```

Intent Analyzer 的 prompt 应保持极小。它不接收全量 function / tool schema，只能看到固定 Kernel Prompt 和少量 meta functions。

固定 meta functions：

```text
function.search
function.load_schema
skill.search
skill.load
memory.search
```

其中 `function.search` 是最重要的稳定入口。它根据用户请求、IntentProfile 和当前上下文召回候选 function cards，而不是返回完整 schema。

```text
Kernel Context
→ function.search
→ Function Candidates
→ Capability Resolver
→ function.load_schema
→ Active Function Schemas
→ Context Builder
```

Capability Resolver 负责判断候选 function 在当前 Runtime、当前 Channel、当前 Project、当前权限策略下是否真的可用。

示例：

```text
需要浏览器，但 BrowserProvider 未连接 → BLOCKED
需要用户确认，但当前 Channel 不支持确认 → 使用 fallback 或 NEED_USER_INPUT
需要文件上传，但当前 Channel 不支持附件 → request_attachment unavailable
需要 shell，但 security.allow_shell=false → blocked capability
```

Validator 不只验证 Task DAG，也要验证 capability availability。

### 7.1 Function Index 不是 Context

Function / Tool 数量会随着项目增长。如果启动时把所有 function schema 一次性塞给 LLM，会导致意图识别变重、上下文污染、prompt cache 命中率下降。

正确方式：

```text
Function Registry
→ Function Index
→ function.search
→ Function Card top-K
→ function.load_schema
→ Active Function Schema
→ Context Builder
```

Function Index 可以全量存在于 Runtime 内，但 Function Schema 不全量进入 LLM。

### 7.2 Function Card

`function.search` 返回轻量 function card：

```text
FunctionCard
  name
  description
  tags
  task_types
  risk_level
  input_summary
  output_summary
  provider
  required_permissions
  schema_ref
  version_hash
  reason
```

示例：

```json
[
  {
    "name": "code.search",
    "reason": "Need to locate login-related code",
    "risk_level": "low",
    "schema_ref": "fn:code.search@v1"
  },
  {
    "name": "test.run",
    "reason": "Need to verify code changes",
    "risk_level": "medium",
    "schema_ref": "fn:test.run@v1"
  }
]
```

### 7.3 Function Set 复用

Runtime 应记录不同层级的 active function set：

```text
Project Active Function Set
Task Active Function Set
Attempt Active Function Set
```

Task 执行时优先复用已召回的 active function schemas。只有当 IntentProfile、TaskType、CapabilityResolution 或 provider 状态变化时，才重新搜索或加载 schema。

### 7.4 缓存友好规则

1. Kernel Prompt 固定。
2. meta function schema 固定。
3. function cards 排序 deterministic。
4. schema_ref 和 version_hash 稳定。
5. 每个 Task 只加载 active function schemas。
6. Tool result 用 artifact reference，不塞大段内容。

---

## 8. Skill Runtime

agent-gogo 不创建自己的 Skill 格式。Skill Runtime 以兼容 Claude Code / Claude Skills 的 `SKILL.md` package 为目标。

### 8.1 Skill Package 结构

```text
my-skill/
├── SKILL.md
├── reference.md
├── examples/
├── templates/
└── scripts/
```

`SKILL.md` 使用 YAML frontmatter + Markdown instructions。agent-gogo 内部可以有 normalized model，但它只是索引和运行时表示，不是新的外部格式。

### 8.2 职责

```text
Skill Discovery
  扫描 skill roots

Skill Parser
  解析 SKILL.md frontmatter 和 body 摘要

Skill Index
  建立 name / description / allowed-tools / invocation policy / path / inferred tags 索引

Skill Router
  根据 IntentProfile、Capability、Persona、Project context 选择候选 skill

Skill Loader
  只在激活时加载完整 SKILL.md body，必要时加载 supporting files

Skill Permission Mapper
  将 allowed-tools 映射到 agent-gogo 的 Capability / ToolRuntime 权限

Skill Security Gate
  第三方 skill 默认不可信，脚本执行必须经过 Tool Runtime、沙箱、权限确认和审计
```

### 8.3 Index 不是 Context

```text
Claude Skill Roots
→ Skill Discovery
→ Skill Parser
→ Skill Index
→ Intent-aware Skill Router
→ Skill Activation Plan
→ Context Builder
```

Skill Index 可以全量存在于内存、SQLite、FTS 或向量索引中，但不等于全部进入 LLM prompt。

### 8.4 加载层级

```text
L0 Index
  name + description + allowed-tools + path
  常驻系统，不全量进 prompt

L1 Skill Card
  name + description + when_to_use + required capabilities
  进入候选列表

L2 Active Skill
  完整 SKILL.md body
  只有被激活的 skill 进入 Context Pack

L3 Supporting Files
  reference.md / examples / templates
  只有 skill 指令明确需要或 Runtime 请求时才加载

L4 Scripts
  不直接执行
  必须走 Tool Runtime + Permission + Audit
```

### 8.5 Skill 与 Capability 的区别

```text
Skill:
  遇到某类任务时应该怎么做

Capability / Tool:
  系统实际能调用什么
```

示例映射：

```text
allowed-tools: Read Grep Bash(python *)

Read       → file.read capability
Grep       → code.search capability
Bash(...)  → shell.run capability with policy gate
```

Skill 不能绕过 Tool Runtime 获得副作用能力。

---

## 9. Memory Runtime

Memory 也采用索引 + 路由 + 按需加载。Event Log 是事实源，Memory 是从事实源、用户输入和项目产物中提炼出来的可检索知识。

### 9.1 记忆层级

```text
Working Memory
  当前 session / 当前 task 的短上下文

Project Memory
  当前 project 的决策、状态、约束、artifact 摘要

Long-term Memory
  用户偏好、固定事实、跨项目经验、世界观、长期背景
```

### 9.2 混合索引

```text
Memory Index
  metadata index:
    user_id
    project_id
    task_id
    type
    scope
    tags
    source
    confidence
    created_at
    updated_at

  exact index:
    key/value
    user preference
    project config
    fixed constraints

  text index:
    BM25 / SQLite FTS

  semantic index:
    embedding vector

  event link:
    source_event_id
    source_artifact_id
```

### 9.3 检索流程

```text
IntentProfile
→ Memory Router 判断是否需要记忆
→ metadata filter 缩小范围
→ exact memory 优先命中
→ FTS + vector hybrid search
→ rerank
→ Context Builder 加载 top-K 摘要或 artifact 引用
```

### 9.4 写入与提升

```text
TaskEvent / ChatMessage / Artifact
→ Memory Extractor
→ Candidate Memory
→ scope / confidence / source
→ exact low-risk memory auto-save
→ durable preference 或 long-term fact 需要用户确认
→ promoted memory
```

长期记忆不能随意写入。用户偏好、跨项目固定事实、世界观设定等 durable memory 应保留来源和确认状态。

---

## 10. Persona Runtime

Persona 决定表达方式、协作策略和阶段角色，不决定事实、不授予工具权限、不覆盖安全规则。

### 10.1 Persona 类型

```text
Main Persona
  默认协作风格，全局或用户级

Channel Persona
  某个通道的表达约束，例如 Telegram 要短，Web Console 可以结构化

Project Persona
  某个项目的工作风格，例如开源项目维护者、小说世界观编辑

Role Persona
  任务阶段角色，例如 planner、executor、tester、reviewer、writer、browser-operator
```

### 10.2 创建时机

```text
Built-in
  系统内置 planner / reviewer / tester 等基础角色

User-created
  用户在 Web Console / CLI 创建长期角色

Project-created
  项目初始化时创建项目专属角色或风格约束

Ephemeral
  Runtime 为某个 task 临时组合 role，不持久化

Promoted
  临时 persona 被反复使用后，经用户确认保存
```

### 10.3 引用时机

```text
User Request
→ Intent Analyzer 识别任务类型和阶段
→ Persona Router 选择 main + channel + project + role
→ Persona Composer 合并规则
→ Context Builder 加载 active personas
```

阶段示例：

```text
Planning:
  main + channel + project + planner persona

Execution:
  main + channel + project + executor persona

Review:
  main + channel + project + reviewer persona

Writing:
  main + channel + project + writer persona
```

Persona Router 的输出应记录到 TaskEvent，方便解释某次决策为什么使用了某个角色。

---

## 11. Context Builder

Context Builder 是 Function、Skill、Memory、Persona、Capability、Task State 进入 LLM 的唯一装配点。

LLM 输入缓存按前缀稳定性设计。实现时必须假设：从第一个 token 开始，只有连续不变的前缀才能稳定命中缓存。因此 ContextPack 的序列化不是普通拼接逻辑，而是 Runtime 的缓存契约。

核心原则：

> Everything searchable, nothing fully injected. Everything serialized deterministically.

### 11.1 ContextPack 结构

```text
ContextPack
  RuntimeRules
  SecurityRules
  ChannelCapabilities
  IntentProfile
  MetaFunctionSchemas
  ActiveFunctionSchemas
  DeferredFunctionCandidates
  ActiveCapabilities
  ActivePersonas
  ActiveSkillInstructions
  DeferredSkillCandidates
  RelevantMemories
  ProjectState
  TaskState
  AcceptanceCriteria
  EvidenceRefs
  RecentMessages
  UserInput
```

### 11.2 强制缓存层

```text
L0 System Cache Layer
  RuntimeRules
  SecurityRules
  ActivePersonas

L1 Project / Route Cache Layer
  ChannelCapabilities
  MetaFunctionSchemas
  ActiveCapabilities
  ActiveFunctionSchemas
  DeferredFunctionCandidates
  ActiveSkillInstructions
  DeferredSkillCandidates

L2 Task Cache Layer
  IntentProfile
  ProjectState
  TaskState
  RelevantMemories
  AcceptanceCriteria

L3 Dynamic Step Layer
  EvidenceRefs
  RecentMessages
  CurrentUserInput
```

缓存层失效条件：

```text
L0:
  只因 runtime prompt version、security policy version、persona version 变化而失效。

L1:
  因 project、channel capability set、chain level、active capability set、active function set、active skill set 变化而失效。

L2:
  因 task 切换、task goal、acceptance criteria、task-scoped memory set 变化而失效。

L3:
  每次 executor / reviewer / planner 调用都可以变化，必须放在最后。
```

### 11.3 强制排序规则

所有 ContextPack 序列化必须由单一 `ContextSerializer` 完成。任何模块不得手写 prompt 拼接。

块顺序必须固定为：

```text
1. RuntimeRules
2. SecurityRules
3. ActivePersonas
4. ChannelCapabilities
5. MetaFunctionSchemas
6. ActiveCapabilities
7. ActiveFunctionSchemas
8. DeferredFunctionCandidates
9. ActiveSkillInstructions
10. DeferredSkillCandidates
11. IntentProfile
12. ProjectState
13. TaskState
14. RelevantMemories
15. AcceptanceCriteria
16. EvidenceRefs
17. RecentMessages
18. CurrentUserInput
```

块内排序必须固定：

```text
RuntimeRules:              按 rule.id 字母序。
SecurityRules:             按 rule.id 字母序。
ActivePersonas:            按 persona.id 字母序。
ChannelCapabilities:       按 channel_type 字母序，其次 capability name。
MetaFunctionSchemas:       按 function.name 字母序。
ActiveCapabilities:        按 capability.name 字母序。
ActiveFunctionSchemas:     按 function.name 字母序，其次 version_hash。
DeferredFunctionCandidates:按 function.name 字母序，其次 schema_ref。
ActiveSkillInstructions:   按 skill.id 字母序，其次 version_hash。
DeferredSkillCandidates:   按 skill.id 字母序，其次 version_hash。
IntentProfile:             使用稳定字段顺序，domains 和 required_capabilities 按字母序。
ProjectState:              使用稳定摘要模板，不包含当前时间。
TaskState:                 按固定字段顺序：goal、acceptance、status、attempt_count。
RelevantMemories:          按 memory.id 字母序。
AcceptanceCriteria:        按 criterion.id 字母序。
EvidenceRefs:              按 created_at 降序取最近 N 条，再按 created_at 升序、id 字母序输出。
RecentMessages:            按 created_at 降序取最近 N 轮，再按 created_at 升序、id 字母序输出。
CurrentUserInput:          永远最后。
```

硬性约束：

1. L0 / L1 / L2 禁止写入当前时间、随机 ID、请求 ID、token 计数、临时错误、最新工具结果。
2. map 必须转成 key 排序后的数组或稳定 JSON，不允许依赖 Go map 遍历顺序。
3. 所有 schema、skill、persona、memory 必须携带 version hash。
4. 相同输入和相同版本下，序列化字节必须完全一致。
5. L2 在 Task 开始时组装一次，Task 执行期间默认冻结。
6. 如果 TaskState 必须变化，只允许进入新的 task cache version，不允许静默改写旧 L2。
7. L3 是唯一允许频繁变化的区域。

### 11.4 ContextSerializer 接口

```go
type ContextSerializer interface {
    Serialize(ctx context.Context, pack ContextPack) (*SerializedContext, error)
}

type SerializedContext struct {
    Text      string
    LayerKeys ContextLayerKeys
    Version   string
}

type ContextLayerKeys struct {
    L0 string
    L1 string
    L2 string
    L3 string
}
```

`LayerKeys` 用于调试缓存失效。每次 L0 / L1 / L2 key 改变时，Runtime 应能解释是哪一个输入块发生变化。

### 11.5 Function / Skill / Memory 与缓存

Function、Skill、Memory、Persona 的索引可以变化，但进入 ContextPack 的 active 集合必须稳定：

```text
Function:
  function.search 结果先去重，再按 function.name + version_hash 排序。
  只有 active function schemas 进入 L1。

Skill:
  Skill Router 结果先去重，再按 skill.id + version_hash 排序。
  只有 active skill instructions 进入 L1。

Memory:
  Memory Router 结果先过滤 scope，再按 memory.id 排序。
  L2 只保留 task-start 时冻结的 relevant memories。

Persona:
  Persona Composer 输出按 persona.id 排序。
  L0 中只能放稳定 persona 规则，临时 persona 必须有 deterministic id。
```

不得全量注入 Function Schema、Skill、Memory、Tool Result、网页内容或代码文件。长内容必须以摘要和 artifact reference 进入上下文。

---

## 12. Function Runtime / Tool Runtime 与副作用边界

Function Runtime 管理可被 LLM 发现和调用的 function metadata、索引、schema 加载和 active function set。

Tool Runtime 是所有外部副作用的防火墙。

职责：

1. Function / Tool 注册。
2. Function Index 和 Function Card 构建。
3. `function.search` 和 `function.load_schema`。
4. ToolSpec / CapabilitySpec 暴露。
5. 参数 schema 校验。
6. 权限和风险等级检查。
7. 超时、重试、取消。
8. ToolCall 记录。
9. Evidence 和 Artifact 生成。
10. 审计日志。

原则：

```text
Function Index 可全量存在于 Runtime。
Function Schema 只有被召回后才进入 LLM Context。
```

工具风险等级：

```text
low:
  只读、生成文本、读取已授权状态

medium:
  读取本地文件、访问网页、运行测试

high:
  修改文件、提交表单、发送消息、运行 shell

critical:
  删除数据、发布生产内容、支付、转账、删除仓库
```

高风险和关键风险操作必须进入确认流程。确认流程走 Communication Runtime，不由具体工具自己实现。

---

## 13. Browser / Code / Document Runtime

这些模块是 capability modules，不是 Runtime Core。

### 13.1 Browser Runtime

职责：

1. 封装 Chrome MCP / Playwright 等 Provider。
2. 提供稳定动作：open、click、type、extract、screenshot、wait。
3. 采集 URL、DOM Summary、截图、console、network、表单状态。
4. selector fallback。
5. 输出 Observation 和 Artifact。

### 13.2 Code Runtime

职责：

1. 建立 repo map。
2. 建立符号索引。
3. 搜索代码。
4. 局部读取文件。
5. 生成 patch。
6. 跑测试。
7. 输出 diff、test result 和 artifact reference。

### 13.3 Document Runtime

职责：

1. 读取文档结构。
2. 提取内容。
3. 修改文档。
4. 验证格式。
5. 输出 artifact reference。

这些 runtime 不应直接决定 Task 是否完成，只提供能力、观察和证据。

---

## 14. Store 与 Event Log

SQLite 是 v0.1 默认存储。

建议表：

```text
projects
tasks
task_dependencies
task_attempts
task_events
tool_calls
observations
test_results
review_results
artifacts
chat_messages
channel_events
communication_outbox
skills
personas
memories
settings
```

Store 原则：

1. TaskEvent append-only。
2. 状态表保存当前快照。
3. 复杂 payload 可用 JSON，但关键查询字段必须拆列。
4. Artifact 大内容放文件系统，数据库只保存引用。
5. 所有恢复逻辑从 Store 和 Event Log 重建，不从 prompt 猜测。

---

## 15. 包边界建议

```text
cmd/agent-gogo
  程序入口，只做配置加载和 app 启动。

internal/app
  组装依赖，启动 HTTP、CLI、worker 或 Web Console。

internal/domain
  Project、Task、TaskAttempt、Event、Artifact、Memory、Persona 等核心实体。

internal/runtime
  Runtime Core 门面，协调 route、intent、context、plan、schedule、execute、test、review。

internal/communication
  Channel Adapter、Channel Runtime、Capability Registry、Renderer、Outbox。

internal/chain
  L0 / L1 / L2 / L3 路由决策。

internal/intent
  意图识别、任务类型、领域、风险和 grounding requirement。

internal/function
  Function registry、function index、function.search、schema loader、active function set。

internal/capability
  Capability registry、availability check、capability-to-function/tool mapping。

internal/contextbuilder
  组装 LLM 输入，管理稳定区、半稳定区和动态区。

internal/skill
  Claude-compatible skill discovery、parser、index、router、loader、permission mapper。

internal/persona
  persona registry、index、router、composer、loader。

internal/memory
  memory extractor、index、retriever、promotion、loader。

internal/planner
  生成 Task DAG 草稿，不直接写数据库。

internal/validator
  校验任务、依赖、验收标准和 capability availability。

internal/scheduler
  选择下一个 READY task。

internal/executor
  执行 TaskAttempt，不绕过 Tool Runtime。

internal/tools
  工具注册、schema、权限、调用、审计。

internal/observer
  将工具结果、页面状态、测试输出解释为结构化观察。

internal/tester
  执行机械验证。

internal/reviewer
  根据验收标准做目标级验收。

internal/store
  SQLite repository、事务和迁移。

internal/provider
  LLM、Embedding、Browser、Communication、Storage 等外部系统接口。
```

建议把 `domain` 放得足够底层，让 Planner、Scheduler、Store、Web Handler 都依赖同一批实体，避免每层各自发明一套 Task。

---

## 16. Runtime Service 边界

Runtime Service 暴露少量高层方法：

```go
type RuntimeService interface {
    CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error)
    PlanProject(ctx context.Context, projectID string) error
    ResumeProject(ctx context.Context, projectID string) error
    RunNextTask(ctx context.Context, projectID string) (*TaskRunResult, error)
    RetryTask(ctx context.Context, taskID string) error
    ReplanProject(ctx context.Context, projectID string, reason string) error
    HandleChannelEvent(ctx context.Context, event ChannelEvent) error
    HandleUserConfirmation(ctx context.Context, confirmation UserConfirmation) error
}
```

Web Console、CLI、HTTP API 和消息通道只调用应用服务，不直接拼装内部模块，不直接修改 Task 状态。

---

## 17. v0.1 实现顺序

1. `domain + store`：Project、Task、TaskAttempt、Dependency、Event 状态机。
2. `communication`：先支持 CLI / Web Console 的基础 send / ask / confirm 抽象。
3. `runtime`：CreateProject、PlanProject、RunNextTask、ResumeProject。
4. `intent + function + capability`：结构化意图、function.search、schema 按需加载和能力可用性检查。
5. `skill`：Claude-compatible SKILL.md discovery、index、top-K route、active load。
6. `persona + memory`：先做 registry / exact memory / project memory 的薄实现。
7. `planner + validator`：结构化 LLM 输出生成任务，校验 function / capability 和 acceptance。
8. `tools`：mock tool、read-only file tool、test command tool。
9. `executor + observer`：记录 TaskAttempt、ToolCall、Observation、Evidence。
10. `tester + reviewer`：支持 acceptance signal、review result、fix task。
11. `browser`：接 Chrome MCP，输出 DOM Summary 和 screenshot artifact。
12. `web`：Dashboard、Project Detail、Task Detail、Event Log、Approval UI。

这条顺序保留完整架构边界，但每一步都有可运行结果。

---

## 18. 与 PRD 的对齐

PRD 定义的是产品能力，本文档定义工程边界。

已对齐：

1. 支持 Chain Router 的简单链路和项目链路。
2. 支持 Function / Tool schema 按需检索和加载。
3. 支持 Skill 按需检索和加载。
4. 支持 Persona 主人格和角色人格。
5. 支持精确记忆和模糊记忆。
6. 支持 Task DAG、Scheduler、Retry、Repair、Replan。
7. 支持 Tool Runtime、Browser Runtime、Observer、Tester、Reviewer。
8. 支持 Web Console 观察、控制和人工接管。

需要 PRD 后续同步强调：

1. Channel 是 Communication Capability，不只是入口 Adapter。
2. Skill 系统兼容 Claude Code / Claude Skills 的 `SKILL.md` package，不自创格式。
3. TaskAttempt 是核心实体。
4. Intent Analyzer、Function Search、Capability Resolver、Skill Router、Memory Router、Persona Router 是 Context Builder 前置链路。
5. Function、Memory 和 Persona 也采用索引 + 路由 + 按需加载。
6. Validator 需要校验 function / capability availability，而不只是任务结构。
