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
8. 支持 Skill 通过标签和语义检索按需加载。
9. 支持上下文压缩和 LLM 输入缓存友好。
10. 支持精确记忆和模糊记忆。
11. 支持 Persona 主人格和角色人格索引。
12. 支持 Chain Router 判断简单任务还是复杂任务。
13. 支持 Chrome MCP 或类似浏览器 MCP 作为网页控制后端。
14. 支持 Web Console 展示项目、任务、日志、浏览器状态和验收结果。

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
User / Web Console / API
        ↓
Channel Adapter
        ↓
Chain Router
        ↓
Persona Router
        ↓
Context Builder
        ↓
Planner / Validator / Gap Finder
        ↓
Task DAG / Scheduler
        ↓
Executor / Subagent Runtime
        ↓
Tool Runtime / Browser Runtime / Code Runtime / Document Runtime
        ↓
Observer / State Interpreter
        ↓
Tester
        ↓
Reviewer
        ↓
State Store / Memory Store / Artifact Store
```

---

## 6. 核心模块

---

# 6.1 Channel Adapter

## 6.1.1 定义

Channel Adapter 负责接入不同入口，例如：

1. Web Console。
2. CLI。
3. HTTP API。
4. 未来可扩展到 Telegram、企业微信、飞书等。

## 6.1.2 职责

1. 接收用户输入。
2. 标准化为 UserRequest。
3. 传入 Chain Router。
4. 输出 Response 或 Task Execution 状态。

## 6.1.3 数据结构

```go
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

Persona 不决定事实，不替代 Skill，不替代 Memory。

边界：

```text
Chain Router 决定怎么跑。
Skill 决定会什么。
Tool 决定能做什么。
Memory 决定知道什么。
Persona 决定怎么说、以什么身份协作。
```

## 6.3.2 Persona 类型

### 主人格 Main Persona

用于定义默认协作风格，例如：

1. 简洁还是详细。
2. 主动规划还是被动回答。
3. 是否偏工程化。
4. 是否喜欢先给结论。
5. 安全边界和沟通习惯。

### 角色人格 Role Persona

用于具体场景，例如：

1. 代码审查员。
2. 文档助手。
3. 浏览器操作员。
4. 运营顾问。
5. 历史故事创作者。
6. 英语陪练。
7. 测试工程师。
8. 验收审查员。

## 6.3.3 数据结构

```go
type Persona struct {
    ID          string
    Type        string // main | role
    Name        string
    Tags        []string
    Description string
    StyleRules  []string
    Boundaries   []string
    UseCases     []string
    Priority     int
    Version      string
}
```

## 6.3.4 Persona 检索

Persona 可以按标签和语义检索。

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

Skill 是 Agent 可按需加载的能力说明和操作指南。

核心原则：

> Skill 不一次性全部注入，而是通过标签和语义检索按需加载。

## 6.4.2 Skill 分层

```text
Level 0：索引层
Skill ID、名称、标签、一句话描述。

Level 1：执行条件和输入输出 schema。

Level 2：完整操作指南。

Level 3：示例、边界情况、troubleshooting。
```

大多数任务只加载 Level 0 / Level 1。复杂任务再加载 Level 2 / Level 3。

## 6.4.3 数据结构

```go
type Skill struct {
    ID          string
    Name        string
    Tags        []string
    Description string
    Level       int
    Content     string
    ToolNames   []string
    UseCases    []string
    Version     string
    Priority    int
}
```

## 6.4.4 检索机制

输入：

1. 用户目标。
2. Chain Decision。
3. 当前任务类型。
4. Persona tags。

输出：

1. 相关 Skill IDs。
2. Skill Level。
3. 是否需要加载详细指南。

## 6.4.5 缓存友好要求

1. Skill 结果排序 deterministic。
2. 相同标签组合尽量返回相同顺序。
3. Skill 内容模板稳定。
4. 高频 Skill 常驻内存。

---

# 6.5 Tool Runtime

## 6.5.1 定义

Tool Runtime 统一管理工具注册、Schema、权限、调用和结果记录。

## 6.5.2 工具类型

1. Browser Tool。
2. Code Tool。
3. Document Tool。
4. File Tool。
5. Memory Tool。
6. Search Tool。
7. Shell Tool。
8. Test Tool。

## 6.5.3 接口

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

Memory 分为精确记忆和模糊记忆。

### 精确记忆

适合保存：

1. 用户固定偏好。
2. 项目配置。
3. 账号环境配置。
4. API endpoint。
5. 品牌名、产品名。
6. 固定约束。

### 模糊记忆

适合保存：

1. 用户最近做过的项目。
2. 过往讨论结论。
3. 创作世界观。
4. 长期偏好。
5. 风格倾向。

## 6.6.2 数据结构

```go
type MemoryItem struct {
    ID          string
    UserID      string
    ProjectID   string
    Type        string // exact | fuzzy
    Key         string
    Value       string
    Tags        []string
    Confidence  float64
    Source      string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## 6.6.3 检索策略

每次不一定都查 Memory。由 Chain Router 决定。

规则：

1. 简单通用问答可不查。
2. 与用户项目相关，查。
3. 与创作世界观相关，查。
4. 与代码项目状态相关，查。
5. 与工具配置相关，查精确记忆。

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

## 6.7.2 推荐顺序

```text
稳定区：
1. System Rules
2. Runtime Rules
3. Channel Rules
4. Tool Schema
5. Skill Index / Active Skill
6. Persona

半稳定区：
7. Project State
8. Task State
9. Relevant Memory

动态区：
10. Recent Messages
11. Tool Results
12. Current User Input
```

## 6.7.3 数据结构

```go
type ContextPack struct {
    RuntimeRules   []Message
    Personas       []Persona
    ActiveSkills   []Skill
    ToolSchemas    []ToolSpec
    ProjectState   ProjectState
    TaskState      TaskState
    Memories       []MemoryItem
    RecentMessages []Message
    Evidence       []Evidence
    UserInput      string
}
```

## 6.7.4 压缩策略

优先保留：

1. 当前用户请求。
2. 当前任务目标。
3. 验收标准。
4. 必要 Skill。
5. 必要 Tool Schema。
6. 关键 Memory。

优先压缩：

1. 旧对话。
2. 长工具结果。
3. 长网页文本。
4. 代码大文件。
5. Skill 示例。

优先丢弃：

1. 寒暄。
2. 重复结论。
3. 低相关记忆。
4. 已完成任务的中间细节。

## 6.7.5 LLM 输入缓存友好规则

1. 固定内容放前面。
2. 动态内容放后面。
3. Skill / Tool / Persona 排序固定。
4. JSON / YAML 格式固定。
5. 不把当前用户问题放在开头。
6. 不频繁改变 system prompt。

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
    Attempts     int
    MaxAttempts  int
    Assignee     string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

## 6.8.3 Task 状态机

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

## 6.8.4 状态说明

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
2. Project State。
3. Relevant Memory。
4. Relevant Skills。
5. 可用工具。
6. Chain Decision。

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

## 6.10.3 数据结构

```go
type PlanValidation struct {
    Executable       bool
    MissingTasks     []Task
    Blockers         []string
    Risks            []string
    DependencyIssues []string
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
5. Attempts 少者优先。
6. 如果需要人工输入，跳过并标记 BLOCKED。

## 6.11.3 接口

```go
func NextTask(projectID string) (*Task, error)
```

---

# 6.12 Executor / Subagent Runtime

## 6.12.1 定义

Executor 执行单个 Task。

Subagent 不是必须是多个模型，可以是同一个 LLM 使用不同 Persona / Skill / Tool 组合。

## 6.12.2 执行流程

```text
加载任务
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

Vue 适合这个控制台，因为任务 DAG、日志流、配置表单、Skill 编辑器、Memory 管理器和 Chat 都需要比较自然的前端状态管理。

### 6.21.3 页面结构

```text
Web Console
├── Dashboard
├── Chat
├── Projects
├── Project Detail
├── Task Detail
├── Browser View
├── Skill Manager
├── Persona Manager
├── Memory Manager
├── File / Artifact Manager
├── Channel Manager
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

### 6.21.6 Skill Manager

用于管理 Skill 文件和索引。

能力：

1. 查看 Skill 列表。
2. 按 tags 搜索。
3. 查看 Skill Level 0 / 1 / 2 / 3。
4. 编辑 Skill。
5. 新增 Skill。
6. 删除 Skill。
7. 重新构建 Skill Index。
8. 测试 Skill Search。

Skill 文件建议使用 YAML / Markdown：

```text
skills/
├── document/pdf_edit.yaml
├── browser/form_fill.yaml
├── code/go_debug.yaml
└── writing/story_grounding.yaml
```

### 6.21.7 Persona Manager

用于管理主人格和角色人格。

能力：

1. 查看 Main Persona。
2. 查看 Role Persona。
3. 按 tags 搜索。
4. 编辑表达风格。
5. 编辑边界规则。
6. 测试 Persona Router。

### 6.21.8 Memory Manager

用于管理精确记忆和模糊记忆。

能力：

1. 查看 Memory 列表。
2. 按项目、标签、类型筛选。
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
5. Skill 路径。
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
    Server    ServerConfig
    LLM       LLMConfig
    Embedding EmbeddingConfig
    Browser   BrowserConfig
    Storage   StorageConfig
    Runtime   RuntimeConfig
    Security  SecurityConfig
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

Channel 是用户与 Agent Runtime 通信的入口。

Channel 不直接决定任务怎么执行，只负责消息接入、消息输出、上下文标记和默认 Persona / Project 绑定。

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

### 6.22.4 Channel 数据结构

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

type ChannelMessage struct {
    ID          string
    ChannelID   string
    UserID      string
    SessionID   string
    ProjectID   string
    Content     string
    Attachments []Attachment
    Metadata    map[string]any
    CreatedAt   time.Time
}

type ChannelResponse struct {
    ChannelID string
    SessionID string
    Content   string
    Cards     []ResponseCard
    Actions   []UserAction
    Metadata  map[string]any
}
```

### 6.22.5 Channel Router

Channel Router 做三件事：

1. 找到默认 Persona。
2. 找到默认 Project / Session。
3. 标记消息来源和能力边界。

示例：

```text
Web Chat：支持完整项目控制和文件上传。
CLI：适合本地代码任务。
HTTP API：适合外部系统触发任务。
Telegram：适合轻量通知和确认。
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
4. Skill 路径。
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

storage:
  sqlite_path: "./data/agent.db"
  artifact_path: "./data/artifacts"
  skill_path: "./skills"
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

1. Skill search result。
2. Skill content。
3. Tool schema。
4. Persona search result。
5. Memory query result。
6. Context pack。
7. Tool result summary。
8. Browser DOM summary。
9. Code index。
10. Document chunks。

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

1. Skill 变更时按版本失效。
2. Tool schema 变更时按版本失效。
3. 文件变更时 code index 失效。
4. URL DOM hash 变化时 browser cache 失效。
5. Memory 更新时相关 memory query cache 失效。

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
task_events
skills
personas
memories
artifacts
tool_calls
observations
test_results
review_results
settings
```

### 8.3 Task Event

```go
type TaskEvent struct {
    ID        string
    TaskID    string
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

### 9.4 兼容目标

1. OpenAI。
2. DeepSeek。
3. Claude。
4. Gemini。
5. Ollama。
6. 本地模型。
7. Chrome MCP。
8. Playwright。

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
3. 工具参数。
4. 执行结果。
5. Evidence。

---

## 11. MVP 范围

### 11.1 v0.1 必须实现

1. Go 后端。
2. SQLite 状态存储。
3. Web Console 基础页面。
4. Chain Router。
5. Persona Registry。
6. Skill Registry。
7. Context Builder。
8. Planner。
9. Validator。
10. Task DAG。
11. Scheduler。
12. Executor。
13. 基础 Retry。
14. Browser Runtime 接 Chrome MCP。
15. Observer 基础版。
16. Tester 基础版。
17. Reviewer 基础版。
18. Memory 基础版。

### 11.2 v0.1 不做

1. 多用户权限。
2. SaaS 部署。
3. 高级 UI。
4. 完整插件市场。
5. 高级代码智能补全。
6. 完整并发任务调度。
7. 复杂多 Agent 通信。

---

## 12. 里程碑

### Milestone 1：骨架

目标：跑通一个项目创建和任务拆分。

内容：

1. Web Console 创建项目。
2. 输入目标。
3. Chain Router 判断等级。
4. Planner 生成任务。
5. Validator 检查任务。
6. SQLite 保存项目和任务。

验收：

1. 能从一个目标生成 10 个以上任务。
2. 每个任务有目标、类型、依赖、验收标准。
3. 页面能展示任务列表。

### Milestone 2：任务执行

目标：跑通单任务执行。

内容：

1. Scheduler 选择 next task。
2. Executor 执行任务。
3. Tool Runtime 调用工具。
4. Task Event 记录日志。
5. 状态流转。

验收：

1. 点击 Resume 后可执行 READY 任务。
2. 执行成功后进入 IMPLEMENTED。
3. 失败后记录错误。

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

1. Skill 标签检索。
2. Persona 标签检索。
3. 精确记忆。
4. 模糊记忆。
5. Context token budget。

验收：

1. 不再全量注入 Skill。
2. 相同任务命中相同 Skill 顺序。
3. 可保存和检索项目相关记忆。

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
3. Skill / Tool Schema 复用稳定。

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
│   ├── channel/
│   ├── chain/
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
├── skills/
├── personas/
├── migrations/
├── configs/
└── README.md
```

---

## 16. 关键设计原则

详细架构原则见 `docs/design/core_principles.md`。

1. LLM 不直接控制底层系统，Runtime 负责约束。
2. 不全量注入 Skill，按需检索。
3. 不全量读代码，建立索引后按需读取。
4. 不全量塞工具结果，保留摘要和 artifact 引用。
5. 任务是数据库实体，不是聊天记录。
6. 每个任务必须有验收标准。
7. 执行成功不等于任务完成，必须测试和验收。
8. 浏览器点击成功不等于业务成功，必须观察状态。
9. 创作可以新增设定，但必须显式标记。
10. 简单任务走简单链路，复杂任务走项目链路。
11. 稳定内容放 prompt 前缀，动态内容放后缀。
12. 高风险操作必须人工确认。

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
→ 加载 Persona / Skill / Memory
→ 拆任务
→ 验证任务
→ 执行任务
→ 观察状态
→ 测试
→ 验收
→ 失败修复
→ 下一个任务
```

只要这个闭环跑通，后续扩展 Tool、Skill、Persona 和 UI 都会自然增长。
