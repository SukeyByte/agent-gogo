# PRD：Go Agent Runtime with Web Console

版本：v0.1

状态：草案

目标：验证一个工程稳定、执行强、创作有依据、适合长期运行的 Agent Runtime。

---

## 1. 背景与问题

当前很多 Agent 框架更像“单次任务执行器”或“工具调用 Demo”。它们常见问题包括：

1. Skill / Tool 一次性注入，token 成本高，上下文污染严重。
2. 长任务缺少可靠的拆分、调度、恢复和验收机制。
3. 浏览器操作虽然能点击，但缺少稳定的观察、判断和错误恢复。
4. Memory 多数停留在简单向量检索，不区分精确记忆、模糊记忆和任务状态。
5. 创作任务缺少依据约束，容易自由脑补。
6. 简单任务和复杂任务使用同一套链路，导致简单任务过重，复杂任务又不够稳。
7. 缺少可视化控制台，无法观察 Agent 正在做什么，也不方便人工接管。

本项目目标是用 Go 实现一个可嵌入、可恢复、低 token 成本、支持长期项目型任务的 Agent Runtime。

---

## 2. 产品定位

### 2.1 一句话定义

一个使用 Go 构建的、支持 Skill 检索、任务 DAG、Planner / Executor / Tester / Reviewer、浏览器自动化、记忆检索和 Web 控制台的 Agent Runtime。

### 2.2 核心定位

不是简单聊天机器人，而是：

> 可验证、可恢复、可控成本的 AI 执行系统。

### 2.3 对标理解

与 OpenClaw 等偏浏览器自动化的 Agent 框架相比，本项目更强调：

1. 多任务项目执行。
2. 任务拆解和验证。
3. 上下文管理和缓存友好。
4. Skill 按需加载。
5. 执行、测试、验收闭环。
6. 可视化控制台和人工接管。

短期执行层成熟度可能不如成熟浏览器 Agent，但架构上更适合长期任务和复杂项目。

---

## 3. 产品目标

### 3.1 核心目标

1. 支持复杂目标拆分为 10 到 30 个以上任务。
2. 支持任务可执行性验证。
3. 支持发现缺失任务并自动补充。
4. 支持任务依赖 DAG。
5. 支持任务状态机和断点恢复。
6. 支持每个任务的执行、测试、验收。
7. 支持失败后的 retry、repair、replan。
8. 支持 Claude-compatible `SKILL.md` Skill 通过意图、标签和语义检索按需加载。
9. 支持上下文压缩和 LLM 输入缓存友好。
10. 支持精确记忆和模糊记忆。
11. 支持 Persona 主人格、通道人格、项目人格和角色人格索引。
12. 支持 Chain Router 判断简单任务还是复杂任务。
13. 支持 Intent Analyzer、Function Search 与 Capability Resolver。
14. 支持 Chrome MCP 或类似浏览器 MCP 作为网页控制后端。
15. 支持 Web Console 展示项目、任务、日志、浏览器状态和验收结果。
16. 支持 Telegram、WhatsApp 等未来通讯通道以 capability 形式接入。

### 3.2 非目标

v1 不追求：

1. 完整企业权限系统。
2. 多用户 SaaS。
3. 复杂前端框架。
4. 完整多 Agent 社会化协作。
5. 通用大模型训练。
6. 完整 IDE 替代。
7. 所有工具生态一次性接入。

---

## 4. 目标用户与场景

### 4.1 目标用户

第一阶段目标用户是开发者本人，以及类似需要自动化执行长期任务的个人开发者。

### 4.2 典型场景

#### 场景 A：代码任务

用户输入：

> 修复这个 Go 项目的登录 bug，并跑测试确认。

系统流程：

1. Chain Router 判断为复杂代码任务。
2. 加载代码相关 Persona、Skill、Tool。
3. 扫描 Repo，建立代码索引。
4. Planner 拆分任务。
5. Validator 检查是否缺少环境、测试命令、依赖信息。
6. Executor 执行代码读取、修改、测试。
7. Tester 跑测试。
8. Reviewer 根据验收标准确认。

#### 场景 B：浏览器自动化

用户输入：

> 打开后台，把商品 A 的标题改成新版文案，并截图确认。

系统流程：

1. Chain Router 判断为浏览器执行任务。
2. Planner 拆分登录、搜索商品、进入编辑页、修改标题、保存、截图验收。
3. Browser Runtime 通过 Chrome MCP 操作页面。
4. Observer 采集 URL、DOM、截图、错误提示、网络事件。
5. Tester 判断保存是否成功。
6. Reviewer 验收标题是否正确。

#### 场景 C：创作任务

用户输入：

> 根据这个传奇世界观，拆成 20 个小故事。

系统流程：

1. Chain Router 判断为中等复杂创作任务。
2. Persona Router 加载创作角色。
3. Memory / Background 检索世界观和人物设定。
4. Planner 拆为故事大纲、人物线、章节任务、小故事生成、风格校验。
5. 每个小故事都必须传入背景、约束、must_use、must_avoid。
6. 输出 used_facts 和 new_assumptions。
7. Reviewer 检查是否引入未授权设定。

---

## 5. 总体架构

```text
External Communication Channels
  Web Console / CLI / HTTP API / Telegram / WhatsApp / Email / Webhook
        ↓
Communication Layer
  Channel Adapter / Channel Runtime / Capability Registry / Message Renderer / Outbox
        ↓
Application Layer
  Session Service / Project Service / Runtime Service / Config Service
        ↓
Runtime Core
  Chain Router / Intent Analyzer / Function Search / Capability Resolver
  Persona Router / Skill Router / Memory Router
  Context Builder
  Planner / Validator / Gap Finder
  Task DAG / Scheduler
  Executor / Observer / Tester / Reviewer
  Repair / Replan
        ↓
Capability Modules
  Communication Tools / Function Runtime / Tool Runtime / Browser Runtime / Code Runtime / Document Runtime
  Artifact Runtime / Skill Runtime / Persona Runtime / Memory Runtime / Security Runtime
        ↓
Provider / Adapter Layer
  LLM Provider / Embedding Provider / Browser Provider / Tool Provider
  Communication Provider / Storage Provider
        ↓
Persistence Layer
  SQLite / Event Log / Artifact Files / Claude-compatible Skill Packages
  Persona Files / Memory Indexes
```

---

## 6. 核心模块

---

# 6.1 Communication Layer

## 6.1.1 定义

Communication Layer 负责双向用户通讯，而不是简单的入口 Adapter。

不同 Channel 面向不同通讯工具，会给 Agent 暴露不同的用户沟通能力。例如 Web Console 可以展示任务卡和审批弹窗，Telegram 可以发 inline keyboard，WhatsApp 可以发 quick reply 或 template message，CLI 可以做同步确认。

## 6.1.2 职责

1. 适配真实通讯工具协议。
2. 接收用户输入并标准化为 ChannelEvent / UserRequest。
3. 管理 channel、session、user、project 的映射。
4. 暴露当前 channel 可用的用户沟通能力。
5. 将 Runtime 的 communication intent 渲染成通道支持的消息。
6. 维护 outbox、delivery receipt、重试、限流和失败记录。
7. 为高风险操作提供确认通道。

## 6.1.3 数据结构

```go
type ChannelCapability struct {
    ChannelType             string
    SupportedMessageTypes   []string
    SupportedInteractions   []string
    MaxMessageLength        int
    MaxButtons              int
    FileSizeLimit           int64
    SupportsAsyncReply      bool
    SupportsSyncPrompt      bool
    SupportsConfirmation    bool
    SupportsFileRequest     bool
    SupportsStreaming       bool
    PolicyLimits            map[string]any
}

type UserRequest struct {
    ID          string
    Channel     string
    UserID      string
    SessionID   string
    ProjectID   string
    Message     string
    Attachments []Attachment
    Metadata    map[string]any
}

type CommunicationIntent struct {
    ID        string
    ChannelID string
    SessionID string
    Type      string // send_message | ask_user | ask_confirmation | send_artifact | notify_done
    Payload   map[string]any
    RiskLevel string
    CreatedAt time.Time
}
```

Runtime 不应该直接知道 Telegram Bot API、WhatsApp API 或 Web Console 的具体实现，只能看到抽象沟通能力：

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

---

# 6.2 Chain Router

## 6.2.1 定义

Chain Router 决定当前任务走简单链路还是复杂链路。

## 6.2.2 链路等级

```text
L0 Direct Answer
简单问答，不跑计划，不调用工具或只调用轻量记忆。

L1 Assisted Task
轻任务，按需加载 Skill / Tool，单步或少量步骤完成。

L2 Planned Task
中等任务，生成短计划，包含执行、测试、验收。

L3 Project Agent
复杂项目任务，生成 Task DAG，支持 subagent、retry、review、resume。
```

## 6.2.3 判断因素

1. 是否需要工具。
2. 是否需要读文档、读代码或浏览网页。
3. 是否需要多步执行。
4. 是否需要长时间运行。
5. 是否有明确验收标准。
6. 是否需要人工确认。
7. 是否可能造成副作用，例如修改文件、提交表单、发送消息。

## 6.2.4 输出结构

```go
type ChainDecision struct {
    Level        string
    Reason       string
    NeedPlan     bool
    NeedTools    bool
    NeedMemory   bool
    NeedReview   bool
    NeedBrowser  bool
    NeedCode     bool
    NeedDocs     bool
    PersonaIDs   []string
    SkillTags    []string
    ToolNames    []string
    RiskLevel    string
}
```

Chain Router 只判断链路等级和粗粒度需求。更细的任务类型、领域、function 召回、能力需求和 grounding 要由 Intent Analyzer、Function Search 与 Capability Resolver 继续处理。

```go
type IntentProfile struct {
    TaskType              string
    Complexity            string
    Domains               []string
    RequiredCapabilities   []string
    RiskLevel             string
    NeedsUserConfirmation bool
    GroundingRequirement  string
    Confidence            float64
}

type CapabilityResolution struct {
    AvailableCapabilities []string
    MissingCapabilities   []string
    Blockers              []string
    Fallbacks             []string
}
```

前置链路：

```text
UserRequest
↓
Chain Router
↓
Intent Analyzer
↓
Function Search
↓
Capability Resolver
↓
Persona Router / Skill Router / Memory Router
↓
Context Builder
```

## 6.2.5 示例

```text
“帮我改一句文案” → L0 / L1
“帮我写一篇头条软文” → L1
“帮我做一套淘宝详情页结构” → L2
“帮我修改代码并跑测试” → L3
“帮我登录后台修改商品信息” → L3
```

---

# 6.3 Persona 系统

## 6.3.1 定义

Persona 决定 Agent 如何表达、以什么角色协作、如何控制风格。

Persona 不决定事实，不替代 Skill，不替代 Memory，不授予工具权限，也不能覆盖安全规则。

边界：

```text
Chain Router 决定怎么跑。
Intent Analyzer 决定当前请求是什么。
Capability / Tool 决定系统实际能调用什么。
Skill 提供遇到某类任务时应该怎么做的指导。
Memory 决定知道什么。
Persona 决定怎么说、以什么角色协作。
```

## 6.3.2 Persona 类型

### 主人格 Main Persona

用于定义默认协作风格，全局或用户级，例如：

1. 简洁还是详细。
2. 主动规划还是被动回答。
3. 是否偏工程化。
4. 是否喜欢先给结论。
5. 安全边界和沟通习惯。

### 通道人格 Channel Persona

用于定义某个通讯通道的表达约束，例如：

1. Telegram 回复更短。
2. Web Console 可以输出结构化任务卡。
3. CLI 可以显示表格和同步确认。
4. WhatsApp 需要考虑 quick reply 和 business policy。

### 项目人格 Project Persona

用于定义某个项目的工作风格，例如：

1. 开源项目维护者。
2. 小说世界观编辑。
3. 严格测试工程师。
4. 品牌运营顾问。

### 角色人格 Role Persona

用于具体任务阶段，例如：

1. 代码审查员。
2. 文档助手。
3. 浏览器操作员。
4. 运营顾问。
5. 历史故事创作者。
6. 英语陪练。
7. 测试工程师。
8. 验收审查员。

## 6.3.3 Persona 创建时机

```text
Built-in
  系统内置 planner / reviewer / tester 等基础角色。

User-created
  用户在 Web Console / CLI 创建长期角色。

Project-created
  项目初始化时创建项目专属角色或风格约束。

Ephemeral
  Runtime 为某个 task 临时组合 role，不持久化。

Promoted
  临时 persona 被反复使用后，经用户确认保存。
```

## 6.3.4 数据结构

```go
type Persona struct {
    ID          string
    Type        string // main | channel | project | role | ephemeral
    Name        string
    Tags        []string
    Description string
    StyleRules  []string
    Boundaries   []string
    UseCases     []string
    ChannelTypes []string
    ProjectID    string
    Priority     int
    Version      string
}
```

## 6.3.5 Persona 检索与引用时机

Persona 可以按标签、语义、通道、项目和任务阶段检索。

```text
User Request
↓
Intent Analyzer 识别任务类型和阶段
↓
Persona Router 选择 main + channel + project + role
↓
Persona Composer 合并规则
↓
Context Builder 加载 active personas
```

阶段示例：

```text
Planning：main + channel + project + planner persona
Execution：main + channel + project + executor persona
Review：main + channel + project + reviewer persona
Writing：main + channel + project + writer persona
```

Persona Router 的输出应记录到 TaskEvent，方便解释某次决策为什么使用了某个角色。

示例：

```yaml
id: role_writer_story
name: 传奇故事创作者
tags:
  - writing
  - story
  - fantasy
  - grounded_generation
style_rules:
  - 保持世界观一致
  - 不自由新增核心设定
  - 每段故事必须基于输入背景
boundaries:
  - 如需新增设定，必须标记为 new_assumption
```

---

# 6.4 Skill 系统

## 6.4.1 定义

Skill 是 Agent 可按需加载的操作指南，不是能力本身。

agent-gogo 不自创 Skill 格式。Skill 系统以兼容 Claude Code / Claude Skills 的 `SKILL.md` package 为目标。

核心原则：

> Skill Index 可以全量存在于系统内，但 Skill Content 不全量进入 LLM Context。

Skill 与 Capability / Tool 的边界：

```text
Skill:
  遇到某类任务时应该怎么做。

Capability / Tool:
  系统实际能调用什么。
```

## 6.4.2 Claude-compatible Skill Package

```text
my-skill/
├── SKILL.md
├── reference.md
├── examples/
├── templates/
└── scripts/
```

`SKILL.md` 使用 YAML frontmatter + Markdown instructions。

典型 frontmatter：

```yaml
---
name: explain-code
description: Explains code with visual diagrams and analogies. Use when explaining how code works.
allowed-tools: Read Grep
disable-model-invocation: false
user-invocable: true
---
```

## 6.4.3 数据结构

内部可以有 normalized model，但它只是索引和运行时表示，不是新的外部格式。

```go
type SkillPackageRef struct {
    ID                 string
    Name               string
    Description        string
    RootPath           string
    SkillPath          string
    Source             string // user | project | plugin | configured
    AllowedTools       []string
    RequiredCapabilities []string
    DisableModelInvocation bool
    UserInvocable      bool
    InferredTags       []string
    VersionHash        string
    UpdatedAt          time.Time
}
```

## 6.4.4 Skill Runtime 职责

```text
Skill Discovery
  扫描 skill roots。

Skill Parser
  解析 SKILL.md frontmatter 和 body 摘要。

Skill Index
  建立 name / description / allowed-tools / invocation policy / path / inferred tags 索引。

Skill Router
  根据 IntentProfile、Capability、Persona、Project context 选择候选 skill。

Skill Loader
  只在激活时加载完整 SKILL.md body，必要时加载 supporting files。

Skill Permission Mapper
  将 allowed-tools 映射到 agent-gogo 的 Capability / ToolRuntime 权限。

Skill Security Gate
  第三方 skill 默认不可信，脚本执行必须经过 Tool Runtime、沙箱、权限确认和审计。
```

## 6.4.5 检索机制

输入：

1. 用户目标。
2. IntentProfile。
3. CapabilityResolution。
4. 当前任务类型。
5. Persona tags。
6. Project context。

输出：

1. candidate skills。
2. active skills。
3. deferred skills。
4. blocked skills。
5. 是否需要加载 supporting files。

```go
type SkillActivationPlan struct {
    CandidateSkills []SkillPackageRef
    ActiveSkills    []SkillPackageRef
    DeferredSkills  []SkillPackageRef
    BlockedSkills   []SkillPackageRef
    LoadFiles       []string
}
```

## 6.4.6 按需加载分层

```text
L0 Index
  name + description + allowed-tools + path
  常驻系统，不全量进 prompt。

L1 Skill Card
  name + description + when_to_use + required capabilities
  进入候选列表。

L2 Active Skill
  完整 SKILL.md body
  只有被激活的 skill 进入 Context Pack。

L3 Supporting Files
  reference.md / examples / templates
  只有 skill 指令明确需要或 Runtime 请求时才加载。

L4 Scripts
  不直接执行。
  必须走 Tool Runtime + Permission + Audit。
```

## 6.4.7 Skill roots

v1 支持多个 skill root：

```text
~/.claude/skills
.claude/skills
<plugin>/skills
configured extra roots
```

同名优先级建议：

```text
user > project > configured > plugin
```

## 6.4.8 缓存友好要求

1. Skill 结果排序 deterministic。
2. 相同标签组合尽量返回相同顺序。
3. Skill index 和 Skill card 模板稳定。
4. 高频 Skill metadata 常驻内存。
5. Skill body 和 supporting files 按需加载。

示例映射：

```text
allowed-tools: Read Grep Bash(python *)

Read       → file.read capability
Grep       → code.search capability
Bash(...)  → shell.run capability with policy gate
```

---

# 6.5 Function Runtime / Tool Runtime

## 6.5.1 定义

Function Runtime 统一管理 function metadata、function index、function search、schema 按需加载和 active function set。

Tool Runtime 统一管理工具注册、CapabilitySpec / ToolSpec、Schema、权限、调用、审计和结果记录。

Tool Runtime 是所有外部副作用的防火墙。Skill 可以指导怎么做，但不能绕过 Tool Runtime 获得副作用能力。

核心原则：

> Function Index 可全量存在于 Runtime，但 Function Schema 不全量进入 LLM Context。

意图识别阶段只暴露固定极小的 meta functions：

```text
function.search
function.load_schema
skill.search
skill.load
memory.search
```

真实 function / tool schema 通过 `function.search` 召回，再通过 `function.load_schema` 按需加载。

## 6.5.2 工具类型

1. Browser Tool。
2. Code Tool。
3. Document Tool。
4. File Tool。
5. Memory Tool。
6. Search Tool。
7. Shell Tool。
8. Test Tool。

## 6.5.3 Function Index

Function Index 保存轻量元数据：

```text
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
```

Function Search 返回 Function Card，而不是完整 schema。

```go
type FunctionCard struct {
    Name                string
    Description         string
    Tags                []string
    TaskTypes           []string
    RiskLevel           string
    InputSummary        string
    OutputSummary       string
    Provider            string
    RequiredPermissions []string
    SchemaRef           string
    VersionHash         string
    Reason              string
}

type FunctionSchema struct {
    Name         string
    Description  string
    InputSchema   map[string]any
    OutputSchema  map[string]any
    RiskLevel     string
    VersionHash   string
}
```

## 6.5.4 Function 搜索流程

```text
Kernel Context
↓
function.search
↓
Function Candidates
↓
Capability Resolver
↓
function.load_schema
↓
Active Function Schemas
↓
Context Builder
```

Runtime 应记录不同层级的 active function set：

```text
Project Active Function Set
Task Active Function Set
Attempt Active Function Set
```

Task 执行时优先复用已召回的 active function schemas。只有当 IntentProfile、TaskType、CapabilityResolution 或 provider 状态变化时，才重新搜索或加载 schema。

## 6.5.5 接口

```go
type ToolProvider interface {
    ListTools(ctx context.Context) ([]ToolSpec, error)
    CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error)
}

type ToolSpec struct {
    Name        string
    Description string
    InputSchema  map[string]any
    OutputSchema map[string]any
    Tags        []string
    RiskLevel   string
}

type CapabilitySpec struct {
    Name        string
    Description string
    FunctionName string
    ToolName    string
    RiskLevel   string
    Available   bool
    Metadata    map[string]any
}

type ToolResult struct {
    Success   bool
    Output    any
    Error     string
    Evidence  []Evidence
    Metadata  map[string]any
}
```

---

# 6.6 Memory 系统

## 6.6.1 定义

Memory 采用索引 + 路由 + 按需加载。

Event Log 是事实源，Memory 是从事实源、用户输入、项目产物和人工确认中提炼出来的可检索知识。

Memory 不应该每次都查，也不应该全量塞进上下文。由 Memory Router 根据 IntentProfile、Project State、Task State 和 grounding requirement 判断是否检索。

## 6.6.2 记忆层级

### Working Memory

当前 session / 当前 task 的短上下文。

适合保存：

1. 当前对话的临时目标。
2. 当前 Task 的中间结论。
3. 当前 attempt 的观察摘要。
4. 最近工具结果摘要。

### Project Memory

当前 project 的决策、状态、约束和 artifact 摘要。

适合保存：

1. 项目目标。
2. 项目配置。
3. 已确认的技术决策。
4. 测试命令。
5. 文件结构摘要。
6. 验收偏好。

### Long-term Memory

用户级或跨项目长期背景。

适合保存：

1. 用户固定偏好。
2. 品牌名、产品名。
3. 创作世界观。
4. 长期风格倾向。
5. 跨项目经验。
6. 固定约束。

## 6.6.3 数据结构

```go
type MemoryItem struct {
    ID          string
    UserID      string
    ProjectID   string
    TaskID      string
    Type        string // exact | fuzzy
    Scope       string // working | project | long_term
    Key         string
    Value       string
    Tags        []string
    Confidence  float64
    Source      string
    SourceEventID string
    SourceArtifactID string
    Confirmed   bool
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## 6.6.4 混合索引

```text
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

## 6.6.5 检索策略

每次不一定都查 Memory。由 Chain Router 决定。

规则：

1. 简单通用问答可不查。
2. 与用户项目相关，查。
3. 与创作世界观相关，查。
4. 与代码项目状态相关，查。
5. 与工具配置相关，查精确记忆。

检索流程：

```text
IntentProfile
↓
Memory Router 判断是否需要记忆
↓
metadata filter 缩小范围
↓
exact memory 优先命中
↓
FTS + vector hybrid search
↓
rerank
↓
Context Builder 加载 top-K 摘要或 artifact 引用
```

## 6.6.6 写入与提升

```text
TaskEvent / ChatMessage / Artifact
↓
Memory Extractor
↓
Candidate Memory
↓
scope / confidence / source
↓
exact low-risk memory auto-save
↓
durable preference 或 long-term fact 需要用户确认
↓
promoted memory
```

长期记忆不能随意写入。用户偏好、跨项目固定事实、世界观设定等 durable memory 应保留来源和确认状态。

---

# 6.7 Context Builder

## 6.7.1 定义

Context Builder 负责构建 LLM 输入。

目标：

1. token 可控。
2. 上下文相关。
3. LLM 输入缓存友好。
4. 稳定前缀。
5. 动态内容后置。

## 6.7.2 强制缓存层

LLM 输入缓存按前缀稳定性设计。实现时必须假设：从第一个 token 开始，只有连续不变的前缀才能稳定命中缓存。因此 ContextPack 的序列化必须是强约束，而不是普通推荐顺序。

```text
L0 System Cache Layer
1. RuntimeRules
2. SecurityRules
3. ActivePersonas

L1 Project / Route Cache Layer
4. ChannelCapabilities
5. MetaFunctionSchemas
6. ActiveCapabilities
7. ActiveFunctionSchemas
8. DeferredFunctionCandidates
9. ActiveSkillInstructions
10. DeferredSkillCandidates

L2 Task Cache Layer
11. IntentProfile
12. ProjectState
13. TaskState
14. RelevantMemories
15. AcceptanceCriteria

L3 Dynamic Step Layer
16. EvidenceRefs
17. RecentMessages
18. CurrentUserInput
```

缓存层失效条件：

```text
L0：runtime prompt version、security policy version、persona version 变化。
L1：project、channel capability set、chain level、active capability set、active function set、active skill set 变化。
L2：task 切换、task goal、acceptance criteria、task-scoped memory set 变化。
L3：每次 executor / reviewer / planner 调用都可以变化，必须放在最后。
```

## 6.7.3 数据结构

```go
type ContextPack struct {
    RuntimeRules            []Message
    SecurityRules           []Message
    ChannelCapabilities     []ChannelCapability
    IntentProfile           IntentProfile
    MetaFunctionSchemas     []FunctionSchema
    ActiveFunctionSchemas   []FunctionSchema
    DeferredFunctionCandidates []FunctionCard
    ActiveCapabilities      []CapabilitySpec
    ActivePersonas          []Persona
    ActiveSkillInstructions []SkillInstruction
    DeferredSkillCandidates []SkillPackageRef
    RelevantMemories        []MemoryItem
    ProjectState            ProjectState
    TaskState               TaskState
    AcceptanceCriteria      []AcceptanceCriterion
    EvidenceRefs            []ArtifactRef
    RecentMessages          []Message
    UserInput               string
}
```

## 6.7.4 强制排序规则

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

## 6.7.5 压缩策略

优先保留：

1. 当前用户请求。
2. 当前任务目标。
3. 验收标准。
4. 必要 Skill。
5. 必要 Function / Capability / Tool Schema。
6. 关键 Memory。

优先压缩：

1. 旧对话。
2. 长工具结果。
3. 长网页文本。
4. 代码大文件。
5. Skill supporting files。

优先丢弃：

1. 寒暄。
2. 重复结论。
3. 低相关记忆。
4. 已完成任务的中间细节。

## 6.7.6 LLM 输入缓存友好规则

1. 固定内容放前面。
2. 动态内容放后面。
3. Skill / Tool / Persona 排序固定。
4. JSON / YAML 格式固定。
5. 不把当前用户问题放在开头。
6. 不频繁改变 system prompt。
7. Function Index 不等于 Context，只加载 active function schemas。
8. Skill Index 不等于 Context，只加载 active skills。
9. Memory Index 不等于 Context，只加载 relevant memories。
10. function cards 和 schema_ref 排序 deterministic。
11. Function / Skill / Memory / Persona 的 active 集合进入 ContextPack 前必须去重并排序。
12. L0 / L1 / L2 的序列化变更必须产生可解释的 LayerKey 变化。

---

# 6.8 Project 与 Task DAG

## 6.8.1 Project 定义

Project 是一个长期任务容器。

```go
type Project struct {
    ID          string
    UserID      string
    Title       string
    Goal        string
    Status      string
    CreatedAt   time.Time
    UpdatedAt   time.Time
    Metadata    map[string]any
}
```

## 6.8.2 Task 定义

Task 是系统执行的最小可追踪工作单元。

```go
type Task struct {
    ID           string
    ProjectID    string
    Title        string
    Goal         string
    Type         string // plan | code | browser | doc | test | review | writing
    Status       string
    Priority     int
    DependsOn    []string
    Skills       []string
    Tools        []string
    PersonaIDs   []string
    Acceptance   []AcceptanceCriterion
    TestPlan     []TestStep
    Evidence     []Evidence
    AttemptCount int
    MaxAttempts  int
    Assignee     string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

## 6.8.3 TaskAttempt 定义

TaskAttempt 是一次具体执行尝试，是 retry、repair、人工接管、失败分析和审计的基本单位。

```go
type TaskAttempt struct {
    ID          string
    TaskID      string
    ProjectID   string
    AttemptNo   int
    Status      string // running | succeeded | failed | cancelled
    StartedAt   time.Time
    FinishedAt  time.Time
    ToolCalls   []ToolCall
    Observations []Observation
    TestResults []TestResult
    ReviewResults []AcceptanceResult
    Artifacts   []ArtifactRef
    Error       string
    Metadata    map[string]any
}
```

推荐关系：

```text
Project
  has many Tasks

Task
  has many TaskDependencies
  has many TaskAttempts
  has many TaskEvents

TaskAttempt
  has many ToolCalls
  has many Observations
  has many TestResults
  has many ReviewResults
  has many Artifacts
```

## 6.8.4 Task 状态机

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
FAILED
CANCELLED
NEED_USER_INPUT
```

## 6.8.5 状态说明

### DRAFT

任务由 Planner 生成，但未验证。

### READY

任务已验证，依赖满足后可执行。

### BLOCKED

缺少信息、权限、环境、依赖或人工操作。

### IN_PROGRESS

Executor 正在执行。

### IMPLEMENTED

执行已完成，但未测试。

### TESTING

Tester 正在验证。

### REVIEWING

Reviewer 正在验收。

### DONE

任务完成。

### FAILED

任务失败，且超过 retry / repair 上限。

---

# 6.9 Planner

## 6.9.1 定义

Planner 负责把用户目标拆成任务列表。

## 6.9.2 输入

1. 用户目标。
2. Chain Decision。
3. IntentProfile。
4. CapabilityResolution。
5. Project State。
6. Relevant Memory。
7. Active Skill Instructions。
8. Active Persona。
9. 可用 Capability / Tool。

## 6.9.3 输出

Task DAG 草稿。

## 6.9.4 任务拆分原则

1. 每个任务目标明确。
2. 每个任务有验收标准。
3. 每个任务有依赖关系。
4. 每个任务不宜过大。
5. 如果任务可能失败，要有可观察信号。
6. 不把“测试”和“执行”混为一个任务。
7. 不把“验收”和“执行”混为一个任务。

## 6.9.5 典型输出

```json
{
  "tasks": [
    {
      "title": "扫描项目结构",
      "goal": "识别代码语言、入口文件、测试命令和关键目录",
      "type": "code",
      "depends_on": [],
      "acceptance": ["输出项目结构摘要", "识别至少一个测试入口"]
    },
    {
      "title": "定位登录相关代码",
      "goal": "搜索 login/auth/session 相关文件和函数",
      "type": "code",
      "depends_on": ["扫描项目结构"],
      "acceptance": ["列出相关文件", "列出候选修改点"]
    }
  ]
}
```

---

# 6.10 Validator 与 Gap Finder

## 6.10.1 定义

Validator 检查 Planner 生成的任务是否可执行。

Gap Finder 发现缺失任务并补充。

Validator 不只检查任务结构，也要检查 capability availability。

## 6.10.2 检查内容

1. 是否有明确目标。
2. 是否有验收标准。
3. 是否有依赖闭环。
4. 是否缺少前置任务。
5. 是否需要用户提供信息。
6. 是否需要权限。
7. 是否需要工具但工具不可用。
8. 是否测试路径缺失。
9. 是否验收信号不明确。
10. 是否需要浏览器但 BrowserProvider 不可用。
11. 是否需要用户确认但当前 Channel 不支持确认。
12. 是否需要附件但当前 Channel 不支持文件请求。
13. 是否需要 shell 但 security policy 禁止。

## 6.10.3 数据结构

```go
type PlanValidation struct {
    Executable       bool
    MissingTasks     []Task
    Blockers         []string
    Risks            []string
    DependencyIssues []string
    CapabilityIssues []string
    Suggestions      []string
}
```

## 6.10.4 循环机制

```text
Planner 生成任务
↓
Validator 检查
↓
Gap Finder 生成缺失任务
↓
Planner 合并任务
↓
Validator 再检查
↓
Executable = true 后冻结计划
```

---

# 6.11 Scheduler

## 6.11.1 定义

Scheduler 决定下一个执行哪个任务。

## 6.11.2 选择规则

1. 状态必须是 READY。
2. 所有依赖任务必须 DONE。
3. 优先级高者先执行。
4. 风险低者优先。
5. TaskAttempt 数量少者优先。
6. 如果需要人工输入，跳过并标记 BLOCKED。

## 6.11.3 接口

```go
func NextTask(projectID string) (*Task, error)
```

---

# 6.12 Executor / Subagent Runtime

## 6.12.1 定义

Executor 执行单个 Task。

Subagent 不是必须是多个模型，可以是同一个 LLM 使用不同 Persona / Skill / Capability / Tool 组合。

## 6.12.2 执行流程

```text
加载任务
↓
创建 TaskAttempt
↓
构建上下文
↓
选择工具
↓
执行 Action
↓
采集 Observation
↓
生成 StepResult
↓
写入 Task Event
```

## 6.12.3 数据结构

```go
type Step struct {
    ID          string
    TaskID      string
    Goal        string
    ToolName    string
    Args        map[string]any
    Expectation string
    RetryPolicy RetryPolicy
}

type StepResult struct {
    StepID    string
    Success   bool
    Output    any
    Error     string
    Evidence  []Evidence
    Attempts  int
}
```

---

# 6.13 Retry / Repair / Replan

## 6.13.1 RetryPolicy

```go
type RetryPolicy struct {
    MaxAttempts int
    BackoffMs   int
    RetryOn     []string
}
```

## 6.13.2 默认规则

```text
网络错误：最多 3 次。
浏览器点击失败：重新观察页面后最多 2 次。
LLM JSON 解析失败：自动修复 1 次。
权限问题：不重试，直接 NEED_USER_INPUT。
验证码：不重试，直接 NEED_USER_INPUT。
测试失败：生成修复任务。
验收失败：生成 fix task。
```

## 6.13.3 RepairDecision

```go
type RepairDecision struct {
    Action      string // retry | replan | ask_user | fail | create_fix_task
    Reason      string
    NewArgs     map[string]any
    FixTasks    []Task
    UserQuestion string
}
```

---

# 6.14 Browser Runtime

## 6.14.1 定义

Browser Runtime 是网页操作能力层，底层可接 Chrome MCP。

Chrome MCP 负责真实浏览器操作，Browser Runtime 负责封装稳定接口、观察、fallback 和状态理解。

## 6.14.2 不直接裸用 MCP 的原因

裸用 MCP 容易出现：

1. LLM 直接点错元素。
2. selector 不稳定。
3. 页面变化后无法恢复。
4. 缺少任务验收信号。
5. 操作成功不等于任务成功。

## 6.14.3 封装 API

```go
type BrowserRuntime interface {
    Open(ctx context.Context, url string) (*ActionResult, error)
    Click(ctx context.Context, target ElementTarget) (*ActionResult, error)
    Type(ctx context.Context, target ElementTarget, text string) (*ActionResult, error)
    ExtractText(ctx context.Context, target ElementTarget) (*ActionResult, error)
    Screenshot(ctx context.Context) (*ActionResult, error)
    GetDOMSummary(ctx context.Context) (*PageSummary, error)
    FindElements(ctx context.Context, query string) ([]Element, error)
    WaitFor(ctx context.Context, condition WaitCondition) (*ActionResult, error)
}
```

## 6.14.4 Element Target

```go
type ElementTarget struct {
    Selector string
    Text     string
    Role     string
    Label    string
    TestID   string
    XPath    string
}
```

## 6.14.5 Selector fallback 优先级

```text
1. data-testid
2. aria-label
3. role
4. label text
5. visible text
6. CSS selector
7. XPath
8. coordinate click（最后兜底）
```

## 6.14.6 页面缓存

缓存内容：

1. URL。
2. DOM hash。
3. 可点击元素。
4. 表单字段。
5. 常用 selector。
6. 上次成功路径。

---

# 6.15 Observer 与 State Interpreter

## 6.15.1 定义

Observer 负责观察执行后的真实状态。

State Interpreter 负责把观察结果解释为任务状态。

核心原则：

> 观察不是截图识别，而是把页面变化、工具结果、测试结果转成可验证状态。

## 6.15.2 三层观察

### Raw Observation

原始信息：

1. URL。
2. Title。
3. Screenshot。
4. DOM。
5. Console logs。
6. Network events。
7. Local storage / cookies。
8. Tool raw output。

### Structured Observation

结构化摘要：

1. 页面标题。
2. 关键文本。
3. 按钮。
4. 输入框。
5. 链接。
6. 错误提示。
7. alert / toast。
8. 表单状态。
9. 网络状态。

### Interpreted State

语义状态：

1. success。
2. failed。
3. blocked。
4. progress。
5. unknown。

## 6.15.3 数据结构

```go
type RawObservation struct {
    URL        string
    Title      string
    Screenshot string
    DOM        string
    Console    []string
    Network    []NetworkEvent
    Metadata   map[string]any
}

type PageSummary struct {
    URL      string
    Title    string
    Texts    []string
    Buttons  []Element
    Inputs   []Element
    Links    []Element
    Alerts   []string
    Errors   []string
    Forms    []FormSummary
}

type InterpretedState struct {
    State       string // success | failed | blocked | progress | unknown
    Confidence  float64
    Signals     []string
    Missing     []string
    NextActions []string
}
```

## 6.15.4 判断流程

```text
Action 执行
↓
WaitFor 页面稳定
↓
Capture RawObservation
↓
Extract PageSummary
↓
RuleJudge
↓
如果规则置信度足够高，直接返回
↓
否则 LLMJudge
↓
MergeDecision
↓
进入 Tester / Repair
```

## 6.15.5 RuleJudge

规则层判断：

1. URL 是否变化。
2. 目标元素是否出现。
3. 错误文案是否出现。
4. 按钮是否 disabled。
5. 表单值是否改变。
6. network 是否 200 / 4xx / 5xx。
7. console 是否报错。
8. DOM hash 是否变化。
9. 页面是否出现验证码。
10. 页面是否出现登录态失效。

## 6.15.6 LLMJudge

LLM 只在规则不足时参与。

输入：

1. 当前任务目标。
2. 验收标准。
3. Action 前状态。
4. Action 后 PageSummary。
5. 关键截图说明。

输出必须是结构化 JSON：

```json
{
  "state": "success",
  "confidence": 0.92,
  "signals": [
    "URL changed to /dashboard",
    "page contains welcome text",
    "user avatar visible"
  ],
  "missing": [],
  "next_actions": []
}
```

## 6.15.7 Acceptance Signals

每个任务可以定义验收信号：

```go
type AcceptanceSignal struct {
    Type  string // text_exists | url_contains | element_exists | network_ok | file_exists | test_passed
    Value string
}
```

示例：

```json
[
  {"type": "url_contains", "value": "/dashboard"},
  {"type": "text_exists", "value": "欢迎回来"},
  {"type": "element_exists", "value": "[data-testid='user-avatar']"}
]
```

---

# 6.16 Tester

## 6.16.1 定义

Tester 验证任务执行结果是否符合 TestPlan。

## 6.16.2 测试类型

1. 代码测试：go test、npm test、flutter test。
2. 浏览器测试：页面信号、URL、元素、截图。
3. 文档测试：文件存在、内容匹配、格式正确。
4. 创作测试：是否满足结构、字数、风格、设定一致性。

## 6.16.3 数据结构

```go
type TestStep struct {
    ID       string
    Type     string
    Command  string
    Expected string
}

type TestResult struct {
    Passed bool
    Issues []string
    Evidence []Evidence
}
```

---

# 6.17 Reviewer

## 6.17.1 定义

Reviewer 根据 Acceptance Criteria 验收任务。

Tester 更偏机械验证，Reviewer 更偏目标验证。

## 6.17.2 输出

```go
type AcceptanceCriterion struct {
    ID          string
    Description string
    Signals     []AcceptanceSignal
    Required    bool
}

type AcceptanceResult struct {
    Passed         bool
    FailedCriteria []string
    Evidence       []Evidence
    FixTasks       []Task
}
```

## 6.17.3 验收失败处理

如果失败：

1. 生成 Fix Task。
2. 标记原任务 REVIEW_FAILED。
3. Fix Task 依赖原任务。
4. Fix Task 完成后重新测试和验收。

---

# 6.18 Code Understanding Runtime

## 6.18.1 定义

Agent 不全量读取代码，而是建立代码索引并按需检索。

## 6.18.2 代码理解四层

```text
Level 0：仓库地图
目录结构、语言、框架、入口文件、依赖文件。

Level 1：符号索引
package、class、struct、function、method、interface、import/export。

Level 2：语义摘要
文件职责、函数职责、模块关系。

Level 3：原始代码片段
只有要修改时读取。
```

## 6.18.3 数据结构

```go
type CodeIndex struct {
    RepoID       string
    Files        []FileMeta
    Symbols      []Symbol
    Dependencies []DependencyEdge
}

type Symbol struct {
    Name      string
    Kind      string
    FilePath  string
    StartLine int
    EndLine   int
    Summary   string
    Tags      []string
}
```

## 6.18.4 工具

```text
SearchCode(query)
ReadFile(path, startLine, endLine)
FindSymbol(name)
FindReferences(symbol)
FindCallers(symbol)
ApplyPatch(diff)
RunTests(scope)
GitDiff()
Rollback()
```

## 6.18.5 技术实现建议

1. ripgrep：文本搜索。
2. tree-sitter：多语言 AST。
3. gopls：Go 项目符号和引用。
4. ctags：通用符号索引。
5. git diff：修改追踪。
6. go test / npm test / flutter test：验证。

---

# 6.19 创作任务 Grounding

## 6.19.1 问题

创作任务容易出现：

1. 人物设定跑偏。
2. 时间线冲突。
3. 背景设定被自由修改。
4. 小故事与主线不一致。
5. 模型新增未授权设定。

## 6.19.2 解决方式

将大创作任务拆成小任务，并为每个小任务传入背景。

```text
世界观背景
↓
主线设定
↓
人物设定
↓
章节目标
↓
小故事任务
↓
输出 used_facts / new_assumptions
↓
Reviewer 检查一致性
```

## 6.19.3 StoryTask

```go
type StoryTask struct {
    ID          string
    Background  string
    Goal        string
    Constraints []string
    MustUse     []string
    MustAvoid   []string
    OutputStyle string
    Acceptance  []string
}
```

## 6.19.4 输出格式

```json
{
  "story": "...",
  "used_facts": ["..."],
  "new_assumptions": ["..."],
  "confidence": {
    "input_grounding": 0.9,
    "task_clarity": 0.85,
    "output_validity": 0.8
  }
}
```

## 6.19.5 幻觉控制原则

1. 只能基于 Background 和 Memory 中的信息创作。
2. 如需新增设定，必须标记为 new_assumption。
3. Reviewer 检查是否引入未授权设定。
4. 输出必须声明 used_facts。
5. 背景不足时，不直接编造核心设定。

---

# 6.20 Confidence 机制

## 6.20.1 定义

每个 Task 和 Output 都可以带置信度。

但不应只有一个 confidence，而要拆分。

## 6.20.2 推荐结构

```go
type Confidence struct {
    InputGrounding float64 // 输入依据是否充分
    TaskClarity    float64 // 任务是否清晰
    OutputValidity float64 // 输出是否符合要求
}
```

## 6.20.3 是否每次 Search

不需要每次都 search，而是按任务类型决定。

```text
创作类：不一定 search，但必须查背景库。
事实类：必须 search 或查可信资料。
代码类：必须查 repo。
文档类：必须读原文。
浏览器类：必须观察页面。
```

## 6.20.4 触发补充检索的条件

1. InputGrounding < 0.7。
2. 任务涉及事实。
3. 验收标准依赖外部信息。
4. 代码任务未定位到相关文件。
5. 浏览器任务页面状态 unknown。
6. Reviewer 标记依据不足。

---

## 6.21 Web Console

### 6.21.1 定义

Web Console 是项目控制台，用于创建目标、查看任务、观察执行、管理配置、管理 Skill、管理记忆、管理文件和进行 Chat。

v1 定位为本地单用户控制台，不做登录系统。

### 6.21.2 技术选型

```text
后端：Go + Gin / Echo / Fiber
前端：Vue 3 + Vite
样式：Tailwind / UnoCSS / Naive UI / Element Plus
数据库：SQLite
实时通信：SSE 优先，WebSocket 可选
浏览器控制：Chrome MCP
```

Vue 适合这个控制台，因为任务 DAG、日志流、配置表单、Skill Index 查看器、Memory 管理器和 Chat 都需要比较自然的前端状态管理。

### 6.21.3 页面结构

```text
Web Console
├── Dashboard
├── Chat
├── Projects
├── Project Detail
├── Task Detail
├── Browser View
├── Skill Runtime Manager
├── Persona Runtime Manager
├── Memory Runtime Manager
├── File / Artifact Manager
├── Communication Channel Manager
└── Config / Settings
```

### 6.21.4 Dashboard

显示：

1. 当前运行状态。
2. 项目数量。
3. 运行中任务。
4. 失败任务。
5. 最近 Tool 调用。
6. 最近 Memory 写入。
7. 当前 Provider 状态。
8. Chrome MCP 连接状态。

### 6.21.5 Chat

Chat 是主入口之一。

能力：

1. 输入自然语言目标。
2. 选择 Channel / Persona / Project。
3. 显示 Agent 回复。
4. 显示是否进入计划模式。
5. 展示当前 Chain Decision。
6. 展示调用的 Skill、Tool、Memory。
7. 支持把一次聊天升级为 Project。
8. 支持人工确认高风险操作。

```go
type ChatMessage struct {
    ID          string
    SessionID   string
    ProjectID   string
    Role        string // user | assistant | tool | system
    Content     string
    Artifacts   []string
    Metadata    map[string]any
    CreatedAt   time.Time
}
```

### 6.21.6 Skill Runtime Manager

用于查看 Claude-compatible `SKILL.md` packages、skill roots、索引和加载状态。v1 不自创 Skill 格式。

能力：

1. 查看 Skill 列表。
2. 按 description、tags、allowed-tools、capability 搜索。
3. 查看 Skill frontmatter、SKILL.md body、supporting files。
4. 查看 Skill Activation Plan。
5. 查看 allowed-tools 到 Capability 的映射。
6. 启用 / 禁用 skill root。
7. 重新构建 Skill Index。
8. 测试 Skill Search。

Skill 文件使用 Claude-compatible package：

```text
.claude/skills/
├── pdf-edit/
│   └── SKILL.md
├── browser-form-fill/
│   └── SKILL.md
└── go-debug/
    ├── SKILL.md
    └── reference.md
```

### 6.21.7 Persona Runtime Manager

用于管理 Main / Channel / Project / Role Persona，以及查看 Persona Router 和 Persona Composer 的输出。

能力：

1. 查看 Main Persona。
2. 查看 Channel / Project / Role Persona。
3. 按 tags 搜索。
4. 编辑表达风格。
5. 编辑边界规则。
6. 测试 Persona Router。

### 6.21.8 Memory Runtime Manager

用于管理 working / project / long-term memory，以及查看 Memory Router 的命中原因。

能力：

1. 查看 Memory 列表。
2. 按项目、标签、类型、scope 筛选。
3. 新增精确记忆。
4. 删除记忆。
5. 编辑记忆。
6. 测试 Memory Search。
7. 查看 Memory 命中原因。

### 6.21.9 File / Artifact Manager

用于管理本地文件、上传文件、工具产物和执行证据。

能力：

1. 上传文件。
2. 查看文件列表。
3. 查看 artifact。
4. 查看截图。
5. 查看代码 patch。
6. 查看测试报告。
7. 删除临时 artifact。
8. 对文件建立索引。

### 6.21.10 Config / Settings

Config 是 v1 的核心页面之一。

配置内容：

1. LLM Provider。
2. Model。
3. Embedding Provider。
4. Browser MCP 地址。
5. Skill roots。
6. Persona 路径。
7. Memory 配置。
8. Artifact 存储路径。
9. Token Budget。
10. Retry Policy。
11. Chain Router 阈值。
12. 是否允许自动执行高风险任务。
13. 是否开启 prompt cache 友好模式。
14. 是否开启调试日志。

```go
type AppConfig struct {
    Server        ServerConfig
    LLM           LLMConfig
    Embedding     EmbeddingConfig
    Browser       BrowserConfig
    Communication CommunicationConfig
    Storage       StorageConfig
    Runtime       RuntimeConfig
    Security      SecurityConfig
}

type LLMConfig struct {
    Provider string
    Model    string
    BaseURL  string
    APIKey   string
    Timeout  int
}

type BrowserConfig struct {
    Provider string // chrome_mcp | playwright
    MCPURL   string
    Headless bool
    Timeout  int
}

type CommunicationConfig struct {
    EnabledChannels []string
    DefaultChannel  string
    Providers       map[string]map[string]any
}

type RuntimeConfig struct {
    MaxTasksPerProject int
    MaxRetries         int
    TokenBudget        int
    EnablePromptCache  bool
    EnableAutoRepair   bool
}
```

### 6.21.11 操作按钮

全局常用操作：

1. Pause。
2. Resume。
3. Retry。
4. Skip。
5. Approve。
6. Reject。
7. Create Fix Task。
8. Replan。
9. Ask User。
10. Stop Project。
11. Rebuild Index。
12. Test Provider。
13. Test Browser MCP。

---

## 6.22 Communication Channel 系统

### 6.22.1 定义

Channel 是用户与 Agent Runtime 的双向通讯能力层。

Channel 不直接决定任务怎么执行，但它决定 Agent 能用什么方式继续和用户沟通，例如发送文本、请求确认、请求附件、展示按钮、发送文件、显示进度等。

因此 Channel 不是简单入口 Adapter，而是 Communication Capability。

### 6.22.2 v1 Channel

v1 支持：

1. Web Chat。
2. HTTP API。
3. CLI。

### 6.22.3 未来 Channel

未来可支持：

1. Telegram Bot。
2. Discord Bot。
3. 企业微信。
4. 飞书。
5. 邮件。
6. Webhook。
7. Browser Extension。

### 6.22.4 模块组成

```text
Channel Adapter
  接入 Telegram / WhatsApp / Web / CLI 等真实协议。

Channel Runtime
  管理 session、identity、message send、receipt、retry、rate limit。

Channel Capability Registry
  描述该通道支持的通讯方法和限制。

Message Renderer
  把 Runtime 的 CommunicationIntent 渲染成通道支持的消息。

Outbox / Delivery Receipt
  记录待发送消息、发送结果、失败原因和重试状态。
```

### 6.22.5 Channel 数据结构

```go
type Channel struct {
    ID               string
    Type             string // web | cli | api | telegram | webhook
    Name             string
    Enabled          bool
    DefaultPersonaID string
    DefaultProjectID string
    Config           map[string]any
}

type ChannelEvent struct {
    ID          string
    ChannelID   string
    UserID      string
    SessionID   string
    ProjectID   string
    Type        string // message | callback | attachment | delivery_receipt
    Content     string
    Attachments []Attachment
    Metadata    map[string]any
    CreatedAt   time.Time
}

type ChannelCapability struct {
    ChannelType             string
    SupportedMessageTypes   []string
    SupportedInteractions   []string
    MaxMessageLength        int
    MaxButtons              int
    FileSizeLimit           int64
    SupportsAsyncReply      bool
    SupportsSyncPrompt      bool
    SupportsConfirmation    bool
    SupportsFileRequest     bool
    SupportsStreaming       bool
    PolicyLimits            map[string]any
}

type CommunicationIntent struct {
    ID        string
    ChannelID string
    SessionID string
    Type      string // send_message | ask_user | ask_confirmation | send_artifact | notify_done
    Payload   map[string]any
    RiskLevel string
    CreatedAt time.Time
}
```

### 6.22.6 Channel Capability 示例

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

同一个 `ask_confirmation` 在不同通道中的渲染：

```text
Web Console → Approve / Reject modal
Telegram    → inline keyboard
WhatsApp    → quick reply 或 template message
CLI         → y/N prompt
HTTP API    → pending_confirmation response
```

---

## 6.23 Config 系统

### 6.23.1 定义

Config 系统负责管理 Runtime 的全局配置和项目级配置。

v1 不做登录，因此 Config 默认本地可信。

### 6.23.2 配置层级

```text
Default Config
↓
App Config
↓
Project Config
↓
Runtime Override
```

优先级从下到上覆盖。

### 6.23.3 配置来源

1. config.yaml。
2. 环境变量。
3. Web Console Settings。
4. Project-level overrides。

### 6.23.4 配置热更新

v1 允许部分热更新：

1. Model。
2. Token Budget。
3. Browser MCP URL。
4. Skill roots。
5. Retry 次数。

不建议热更新：

1. 数据库路径。
2. Artifact 根目录。
3. Server 端口。

### 6.23.5 config.yaml 示例

```yaml
server:
  host: "127.0.0.1"
  port: 8080

llm:
  provider: "openai"
  model: "gpt-4.1"
  base_url: ""
  api_key: "${OPENAI_API_KEY}"
  timeout: 120

embedding:
  provider: "openai"
  model: "text-embedding-3-small"

browser:
  provider: "chrome_mcp"
  mcp_url: "http://127.0.0.1:9222"
  headless: false
  timeout: 60

communication:
  enabled_channels:
    - "web"
    - "cli"
  default_channel: "web"
  providers: {}

storage:
  sqlite_path: "./data/agent.db"
  artifact_path: "./data/artifacts"
  skill_roots:
    - "~/.claude/skills"
    - ".claude/skills"
  persona_path: "./personas"

runtime:
  max_tasks_per_project: 50
  max_retries: 3
  token_budget: 32000
  enable_prompt_cache: true
  enable_auto_repair: true

security:
  require_confirm_high_risk: true
  allow_shell: false
```

---

## 7. 缓存策略

### 7.1 缓存对象

1. Function search result。
2. Function schema。
3. Skill search result。
4. Skill card / active skill content。
5. Tool schema。
6. Persona search result。
7. Memory query result。
8. Context pack。
9. Tool result summary。
10. Browser DOM summary。
11. Code index。
12. Document chunks。

### 7.2 缓存层级

```text
L1：进程内内存缓存。
L2：SQLite / Badger / Redis。
L3：本地文件 artifact cache。
```

### 7.3 缓存 key

不要只用原始 query。

推荐：

```text
intent + normalized_query + tags + version + project_id
```

### 7.4 失效策略

1. Function schema 变更时按 version hash 失效。
2. SKILL.md 或 supporting files 变更时按 version hash 失效。
3. Tool schema 变更时按版本失效。
4. 文件变更时 code index 失效。
5. URL DOM hash 变化时 browser cache 失效。
6. Memory 更新时相关 memory query cache 失效。

---

## 8. 数据存储

### 8.1 v1 推荐

SQLite。

原因：

1. 简单。
2. 可嵌入。
3. 可恢复。
4. 易部署。

### 8.2 表设计建议

```text
projects
tasks
task_dependencies
task_attempts
task_events
tool_calls
functions
function_schemas
active_function_sets
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

### 8.3 Task Event

```go
type TaskEvent struct {
    ID        string
    TaskID    string
    AttemptID string
    Type      string
    Message   string
    Payload   map[string]any
    CreatedAt time.Time
}
```

---

## 9. Provider 抽象

### 9.1 LLMProvider

```go
type LLMProvider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
```

### 9.2 EmbeddingProvider

```go
type EmbeddingProvider interface {
    Embed(ctx context.Context, text string) ([]float32, error)
}
```

### 9.3 BrowserProvider

```go
type BrowserProvider interface {
    Call(ctx context.Context, action string, args map[string]any) (*BrowserProviderResult, error)
}
```

### 9.4 CommunicationProvider

```go
type CommunicationProvider interface {
    Send(ctx context.Context, req CommunicationSendRequest) (*CommunicationSendResult, error)
    GetCapability(ctx context.Context) (*ChannelCapability, error)
}
```

### 9.5 ToolProvider

```go
type ToolProvider interface {
    ListTools(ctx context.Context) ([]ToolSpec, error)
    CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error)
}
```

### 9.6 兼容目标

1. OpenAI。
2. DeepSeek。
3. Claude。
4. Gemini。
5. Ollama。
6. 本地模型。
7. Chrome MCP。
8. Playwright。
9. Telegram Bot API。
10. WhatsApp Business API。
11. Email / SMTP。

---

## 10. 安全与权限

### 10.1 风险等级

```text
low：只读、生成文本。
medium：读取本地文件、读取网页、运行测试。
high：修改文件、提交表单、删除数据、发送消息。
critical：支付、转账、删除仓库、发布生产内容。
```

### 10.2 人工确认规则

必须确认：

1. 删除文件。
2. 提交表单。
3. 发送邮件或消息。
4. 修改线上内容。
5. 运行高风险 shell 命令。
6. 发布内容。
7. 涉及支付或资金。

### 10.3 审计

所有高风险操作必须记录：

1. 谁发起。
2. 何时发起。
3. 工具参数或 communication intent payload。
4. 执行结果。
5. Evidence。
6. 用户确认记录。

---

## 11. MVP 范围

### 11.1 v0.1 必须实现

1. Go 后端。
2. SQLite 状态存储。
3. Web Console 基础页面。
4. Chain Router。
5. Communication Layer 基础版。
6. Intent Analyzer。
7. Function Runtime 基础版。
8. Capability Resolver。
9. Persona Runtime 基础版。
10. Claude-compatible Skill Runtime 基础版。
11. Memory Runtime 基础版。
12. Context Builder。
13. Planner。
14. Validator。
15. Task DAG。
16. TaskAttempt。
17. Scheduler。
18. Executor。
19. 基础 Retry。
20. Browser Runtime 接 Chrome MCP。
21. Observer 基础版。
22. Tester 基础版。
23. Reviewer 基础版。

### 11.2 v0.1 不做

1. 多用户权限。
2. SaaS 部署。
3. 高级 UI。
4. 完整插件市场。
5. 高级代码智能补全。
6. 完整并发任务调度。
7. 复杂多 Agent 通信。
8. 自定义 Skill DSL 或 Skill 市场。

---

## 12. 里程碑

### Milestone 1：骨架

目标：跑通一个项目创建和任务拆分。

内容：

1. Web Console 创建项目。
2. 输入目标。
3. Chain Router 判断等级。
4. Intent Analyzer 输出 IntentProfile。
5. function.search 召回基础 function candidates。
6. Capability Resolver 检查基础能力。
7. Planner 生成任务。
8. Validator 检查任务。
9. SQLite 保存项目、任务和 TaskEvent。

验收：

1. 能从一个目标生成 10 个以上任务。
2. 每个任务有目标、类型、依赖、验收标准。
3. 页面能展示任务列表。

### Milestone 2：任务执行

目标：跑通单任务执行。

内容：

1. Scheduler 选择 next task。
2. 创建 TaskAttempt。
3. 加载 Task Active Function Schemas。
4. Executor 执行任务。
5. Tool Runtime 调用工具。
6. Task Event 记录日志。
7. 状态流转。

验收：

1. 点击 Resume 后可执行 READY 任务。
2. 执行成功后进入 IMPLEMENTED。
3. 失败后记录 attempt、错误和 evidence。

### Milestone 3：浏览器闭环

目标：通过 Chrome MCP 执行网页操作。

内容：

1. Browser Open。
2. Browser Click。
3. Browser Type。
4. Screenshot。
5. DOM Summary。
6. Observer 判断页面状态。

验收：

1. 能打开指定页面。
2. 能点击指定按钮。
3. 能输入内容。
4. 能截图并显示在 Web Console。
5. 能识别基础成功 / 失败信号。

### Milestone 4：测试与验收

目标：执行后自动测试和验收。

内容：

1. Tester。
2. Reviewer。
3. AcceptanceSignal。
4. Fix Task。

验收：

1. 任务执行后自动进入 TESTING。
2. 测试通过后进入 REVIEWING。
3. 验收通过后 DONE。
4. 验收失败生成 Fix Task。

### Milestone 5：Memory / Skill / Persona 完整化

目标：按需加载上下文。

内容：

1. Claude-compatible `SKILL.md` discovery。
2. Skill Index / Skill Router / Skill Loader。
3. Persona Registry / Persona Router / Persona Composer。
4. exact memory / project memory。
5. fuzzy memory 检索基础版。
6. Context token budget。

验收：

1. 不再全量注入 Skill。
2. 相同任务命中相同 Skill 顺序。
3. 只有 active skills 进入 Context Pack。
4. 可保存和检索项目相关记忆。
5. Persona / Memory / Skill 的选择记录进 TaskEvent。

---

## 13. 成功指标

### 13.1 工程指标

1. 可稳定执行 10 个以上任务。
2. 中断后可恢复。
3. 每个任务有完整状态流转。
4. 每个工具调用可追踪。
5. 失败任务可重试或生成 Fix Task。

### 13.2 成本指标

1. 相比全量注入 Skill，token 降低 50% 以上。
2. 相同类型任务 LLM 输入缓存命中率明显提升。
3. Function Index / Skill Index / Tool Schema / Capability Schema 复用稳定。
4. Active Function Schema 数量保持可控，不随 function 总量线性增长。
5. Active Skill 数量保持可控，不随 skill root 总量线性增长。

### 13.3 质量指标

1. 创作任务 new_assumptions 可追踪。
2. 浏览器任务能输出成功 / 失败信号。
3. 代码任务能跑测试并记录结果。
4. Reviewer 可以阻止明显不合格输出进入 DONE。

---

## 14. 验证用 Demo 场景

### Demo 1：创作任务

目标：

> 根据一个传奇世界观，生成 10 个小故事。

验证点：

1. 背景传入。
2. 任务拆分。
3. 每个小故事 used_facts。
4. new_assumptions 标记。
5. Reviewer 检查设定一致性。

### Demo 2：浏览器任务

目标：

> 打开一个测试网页，填写表单，提交并截图确认。

验证点：

1. Chrome MCP 调用。
2. DOM Summary。
3. 点击和输入。
4. 提交后 Observer 判断成功。
5. Web Console 显示截图。

### Demo 3：代码任务

目标：

> 在一个 Go demo repo 里修复一个简单 bug，并跑 go test。

验证点：

1. Repo 扫描。
2. 代码搜索。
3. 读取局部文件。
4. ApplyPatch。
5. RunTests。
6. 验收。

---

## 15. 项目目录建议

```text
agent-runtime/
├── main.go
├── internal/
│   ├── app/
│   ├── domain/
│   ├── communication/
│   ├── chain/
│   ├── intent/
│   ├── function/
│   ├── capability/
│   ├── persona/
│   ├── skill/
│   ├── memory/
│   ├── contextbuilder/
│   ├── planner/
│   ├── validator/
│   ├── scheduler/
│   ├── executor/
│   ├── browser/
│   ├── observer/
│   ├── tester/
│   ├── reviewer/
│   ├── tools/
│   ├── codeindex/
│   ├── store/
│   └── provider/
├── web/
│   ├── templates/
│   ├── static/
│   └── handlers/
├── .claude/
│   └── skills/
├── personas/
├── migrations/
├── configs/
└── README.md
```

---

## 16. 关键设计原则

详细架构原则见 `docs/design/core_principles.md`。

1. LLM 不直接控制底层系统，Runtime 负责约束。
2. Skill 兼容 Claude-style `SKILL.md` package，不自创格式。
3. 不全量读代码，建立索引后按需读取。
4. 不全量塞工具结果，保留摘要和 artifact 引用。
5. Function / Skill / Memory / Persona 都采用索引 + 路由 + 按需加载。
6. 每个任务必须有验收标准。
7. 执行成功不等于任务完成，必须测试和验收。
8. 浏览器点击成功不等于业务成功，必须观察状态。
9. 创作可以新增设定，但必须显式标记。
10. 简单任务走简单链路，复杂任务走项目链路。
11. 稳定内容放 prompt 前缀，动态内容放后缀。
12. 高风险操作必须人工确认。
13. Task / TaskAttempt / TaskEvent 是事实源，不是聊天记录。
14. Channel 是 Communication Capability，不只是请求入口。
15. 意图识别只暴露固定 meta functions，不全量注入 function schema。

---

## 17. 风险与应对

### 风险 1：Planner 拆分质量差

应对：

1. Validator 检查。
2. Gap Finder 补任务。
3. 人工可编辑 Task DAG。

### 风险 2：浏览器操作不稳定

应对：

1. Selector fallback。
2. DOM Summary。
3. Observer 状态判断。
4. Retry。
5. 人工接管。

### 风险 3：上下文成本过高

应对：

1. Skill 按需加载。
2. Memory 检索。
3. TaskState 压缩。
4. Context budget。
5. LLM 输入缓存友好顺序。

### 风险 4：创作幻觉

应对：

1. 背景库。
2. 小任务 grounding。
3. used_facts。
4. new_assumptions。
5. Reviewer 检查。

### 风险 5：代码任务误改

应对：

1. Git diff。
2. ApplyPatch 而不是直接覆盖。
3. RunTests。
4. Rollback。
5. 人工确认高风险修改。

---

## 18. 最终结论

该项目的核心价值不是“让 LLM 更聪明”，而是通过 Go Runtime 构建一个稳定、可控、可恢复的执行系统。

最终产品形态是：

> Go Agent Runtime with Web Console：一个支持 Skill 检索、Persona 路由、任务 DAG、浏览器自动化、代码理解、记忆检索、测试验收和长期运行的 Agent 执行系统。

v0.1 的目标不是做大而全，而是验证闭环：

```text
输入目标
→ 判断链路
→ 识别意图
→ 搜索 Function
→ 按需加载 Function Schema
→ 解析可用能力
→ 路由 Persona / Skill / Memory
→ 按需加载上下文
→ 拆任务
→ 验证任务
→ 执行任务
→ 观察状态
→ 测试
→ 验收
→ 失败修复
→ 下一个任务
```

只要这个闭环跑通，后续扩展 Tool、Communication Channel、Skill、Persona、Memory 和 UI 都会自然增长。
