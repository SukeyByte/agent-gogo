# Agent Runtime 架构草案

状态：草案

目标：定义 v0.1 最合理的模块边界，保证先打穿可恢复、可验证的执行闭环，再逐步扩展 Web Console、浏览器自动化、Memory 和 Skill 管理。

---

## 1. 推荐总体分层

```text
Channel Layer
  Web Console / CLI / HTTP API
        |
        v
Application Layer
  Session Service / Project Service / Runtime Service
        |
        v
Runtime Core
  Chain Router
  Context Builder
  Planner / Validator
  Task DAG / Scheduler
  Executor
  Observer
  Tester
  Reviewer
        |
        v
Capability Layer
  Tool Runtime
  Browser Runtime
  Code Runtime
  Document Runtime
  Memory Runtime
        |
        v
Provider Layer
  LLM Provider
  Embedding Provider
  Browser Provider
  Tool Provider
  Storage Provider
        |
        v
Storage Layer
  SQLite
  Artifact Files
  Skill / Persona Files
```

核心取舍：Runtime Core 只表达执行语义，不绑定 UI、不绑定具体模型、不直接操作外部世界。

---

## 2. v0.1 最小闭环

v0.1 应优先证明这条链路可以稳定运行：

```text
Create Project
→ Route Chain
→ Build Context
→ Plan Tasks
→ Validate Plan
→ Persist Task DAG
→ Pick Next Task
→ Execute Step
→ Record Event
→ Test Result
→ Review Acceptance
→ Done / Fix Task / Replan
```

先做这个闭环，再扩展浏览器、代码索引、复杂 Memory 和 Skill 编辑器。

---

## 3. 包边界建议

```text
cmd/agent-gogo
  程序入口，只做配置加载和 app 启动。

internal/app
  组装依赖，启动 HTTP、CLI 或 runtime worker。

internal/channel
  Web、CLI、HTTP API 等入口适配。

internal/runtime
  Runtime Core 门面，协调 route、plan、schedule、execute、test、review。

internal/chain
  L0 / L1 / L2 / L3 路由决策。

internal/contextbuilder
  组装 LLM 输入，管理稳定区、半稳定区和动态区。

internal/planner
  生成 Task DAG 草稿。

internal/validator
  校验任务可执行性，发现缺口。

internal/scheduler
  选择下一个 READY 任务。

internal/executor
  执行单个任务或步骤，不直接绕过 Tool Runtime。

internal/tools
  工具注册、schema、权限、调用、审计。

internal/observer
  将工具结果、页面状态、测试输出解释为可验证证据。

internal/tester
  执行机械验证。

internal/reviewer
  根据验收标准做目标级验收。

internal/store
  SQLite repository、事务和迁移。

internal/domain
  Project、Task、Event、ToolCall、Observation 等核心实体。

internal/provider
  LLM、Embedding、Browser 等外部系统接口。
```

建议把 `domain` 放得足够底层，让 Planner、Scheduler、Store、Web Handler 都依赖同一批实体，避免每层各自发明一套 Task。

---

## 4. 核心实体

第一批只需要这些实体：

1. Project
2. Task
3. TaskDependency
4. TaskEvent
5. ToolCall
6. Observation
7. TestResult
8. ReviewResult
9. Artifact

暂缓实体：

1. Full Skill Manager
2. Full Persona Manager
3. Long-term fuzzy memory graph
4. Multi-agent messaging
5. Plugin marketplace

---

## 5. 控制流建议

Runtime Service 暴露少量高层方法：

```go
type RuntimeService interface {
    CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error)
    PlanProject(ctx context.Context, projectID string) error
    ResumeProject(ctx context.Context, projectID string) error
    RunNextTask(ctx context.Context, projectID string) (*TaskRunResult, error)
    RetryTask(ctx context.Context, taskID string) error
    ReplanProject(ctx context.Context, projectID string, reason string) error
}
```

这样 Web Console、CLI 和 HTTP API 只调用应用服务，不直接拼装内部模块。

---

## 6. 最重要的设计判断

### 6.1 不要从 Web Console 开始设计

Web Console 很重要，但它应该观察和控制 Runtime，不应该定义 Runtime。否则后续 CLI、API、后台 worker 都会被前端状态拖住。

### 6.2 不要从多 Agent 开始设计

v0.1 的 Subagent 可以先理解为同一个 Executor 使用不同 Persona、Skill、Tool 和 Context。真正的多 Agent 协作等 Task/Event 模型稳定后再做。

### 6.3 不要让 Planner 直接写数据库

Planner 只生成计划草稿。Validator 检查后，由 Runtime Service 在事务里写入 Project、Task 和 Dependency。这样计划生成失败、格式错误或需要人工编辑时不会污染事实源。

### 6.4 不要让 Executor 直接完成任务

Executor 只能产出执行结果和证据。Task 是否 DONE 由 Tester 和 Reviewer 决定。

### 6.5 不要把 Memory 做成第一优先级

Memory 很有价值，但 v0.1 可以先做精确记忆和项目状态检索。模糊记忆、语义搜索和长期偏好等到执行闭环稳定后再扩展。

---

## 7. 推荐 v0.1 实现顺序

1. domain + store：Project、Task、Dependency、Event 状态机。
2. runtime：CreateProject、PlanProject、RunNextTask。
3. planner + validator：先用结构化 LLM 输出生成任务。
4. tools：先接 mock tool、shell read-only tool、test command tool。
5. executor + observer：记录 StepResult 和 Evidence。
6. tester + reviewer：支持 acceptance signal 和 fix task。
7. channel：先 CLI，再 HTTP API。
8. web：最后做 Dashboard、Project Detail、Task Detail。

这条顺序可以让每一阶段都有可运行结果，而不是等 UI 完成后才知道 runtime 是否成立。
