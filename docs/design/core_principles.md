# Agent Runtime 核心设计原则

状态：草案

适用范围：Go Agent Runtime v0.1 及后续开源版本。

---

## 1. Runtime Core 独立于 Web Console

Runtime Core 是系统的可嵌入执行内核，负责 Chain Router、Intent Analyzer、Function Search、Capability Resolver、Context Builder、Planner、Task DAG、Scheduler、Executor、Tool Runtime、Event Store、Tester 和 Reviewer。

Web Console 是 Runtime Core 的一个控制入口和可视化界面，不应成为业务逻辑的宿主。CLI、HTTP API 和未来 Channel 应该复用同一个 Runtime Core。

架构含义：

1. Web Handler 只做请求解析、权限确认和状态展示。
2. Project、Task、Event、ToolCall、Observation 等核心实体不依赖前端页面。
3. Console 可以替换，Runtime 的执行语义不变。

---

## 2. Task / Event Store 是事实源

长期任务不能只存在于聊天上下文里。Project、Task、Task Dependency、Task Event、Tool Call、Observation、Test Result、Review Result 都应持久化。

架构含义：

1. Task 是最小可追踪工作单元。
2. Task Event 记录状态变化、工具调用、失败原因和人工操作。
3. Resume、Retry、Repair、Replan 都从 Store 恢复事实，而不是从 LLM 对话中猜测。
4. ChatMessage 是交互记录，不是任务状态的唯一来源。

---

## 3. Provider Interface 隔离外部系统

LLM、Embedding、Browser、Tool、Communication、Storage 都应通过接口接入。Runtime 不绑定单一模型、单一浏览器实现、单一通讯工具或单一工具生态。

架构含义：

1. LLMProvider 负责模型调用。
2. EmbeddingProvider 负责向量生成。
3. BrowserProvider 负责底层浏览器动作。
4. CommunicationProvider 负责 Telegram、WhatsApp、Email 等外部通讯工具。
5. ToolProvider 负责工具注册和调用。
6. Provider 的失败、超时和限流必须被 Runtime 明确处理。

---

## 4. Function / Skill / Persona / Memory 是可插拔上下文资产

Function、Skill、Persona 和 Memory 都是 Context Builder 的输入资产，不应和 Planner、Executor 的核心控制流硬耦合。

架构含义：

1. Function 提供 LLM 可发现的调用入口，但 schema 必须按需加载。
2. Skill 提供“怎么做”的指导，兼容 Claude-style `SKILL.md` package，按意图和索引检索加载。
3. Persona 决定怎么协作和表达，不替代事实来源。
4. Memory 决定知道什么，区分精确记忆和模糊记忆。
5. Capability / Tool 决定系统实际能调用什么。
6. Context Builder 负责确定性序列化、压缩和缓存友好布局。
7. 高频稳定内容前置，动态内容后置。
8. L0 / L1 / L2 context layer 必须在相同输入和相同版本下序列化为完全相同的字节。

---

## 5. 所有副作用必须经过 Runtime 边界

LLM 不直接修改文件、点击页面、运行命令、提交表单或发送消息。任何会影响外部世界的动作都必须经过 Tool Runtime 或 Communication Runtime。

架构含义：

1. ToolSpec 必须声明输入 schema、输出 schema、标签和风险等级。
2. 高风险和关键风险动作必须进入人工确认流程。
3. ToolResult 必须带有成功状态、错误信息、证据和元数据。
4. 发送消息、请求确认、请求附件等用户通讯动作必须经过 Communication Runtime。
5. 审计日志必须能回答：谁触发、何时触发、用什么参数、结果如何。

---

## 6. 执行、测试、验收属于同一生命周期

执行成功不等于任务完成。Task 必须经过执行、观察、测试和验收，才能进入 DONE。

架构含义：

1. Executor 负责完成动作。
2. Observer 负责采集真实状态。
3. Tester 负责机械验证。
4. Reviewer 负责目标验收。
5. 验收失败应生成 Fix Task 或触发 Repair / Replan。

---

## 7. 简单任务走轻链路，复杂任务走项目链路

Runtime 需要避免把所有请求都变成重型 Agent 流程。Chain Router 应将任务分为 L0、L1、L2、L3。

架构含义：

1. L0 可以直接回答。
2. L1 可以少量使用 Tool / Skill。
3. L2 需要短计划、测试和验收。
4. L3 需要 Project、Task DAG、Scheduler、Retry、Review 和 Resume。
5. 路由决策必须记录原因，方便调试和人工纠正。

---

## 8. 先观察，再判断，再行动

浏览器任务、代码任务和文档任务都不能只依赖模型主观判断。Runtime 应优先采集结构化证据，再由规则或 LLM 判断状态。

架构含义：

1. Browser Runtime 采集 URL、DOM Summary、截图、console、network 和表单状态。
2. Code Runtime 使用 repo map、符号索引、搜索结果、diff 和测试输出。
3. Document Runtime 使用文件存在性、内容匹配和格式验证结果。
4. RuleJudge 优先，LLMJudge 兜底。
5. Reviewer 的结论必须引用 Evidence。

---

## 9. 默认本地优先，开源友好

v0.1 以本地单用户为默认形态，降低安装、理解和贡献成本。

架构含义：

1. 默认使用 SQLite。
2. 默认配置放在 config.yaml 和环境变量。
3. API Key 不写入仓库，不进入日志。
4. Web Console 不做完整登录系统，但高风险操作仍需确认。
5. 示例、Demo、Skill 和 Persona 应该可在无私有服务的环境中运行。

---

## 10. MVP 先打穿闭环，再扩展模块

v0.1 最重要的是证明 Runtime 闭环可行，而不是一次性完成所有管理页面和所有工具生态。

推荐最小闭环：

```text
输入目标
→ Chain Router
→ Intent Analyzer
→ Function Search
→ Capability Resolver
→ Persona / Skill / Memory Router
→ Context Builder
→ Planner
→ Validator
→ Task DAG
→ Scheduler
→ Executor
→ Tool Runtime
→ Event Store
→ Tester
→ Reviewer
→ Done / Fix Task / Replan
```

Context 序列化必须分层：

```text
L0 System Cache Layer:
  RuntimeRules / SecurityRules / ActivePersonas

L1 Project / Route Cache Layer:
  ChannelCapabilities / MetaFunctionSchemas / ActiveCapabilities / ActiveFunctionSchemas / ActiveSkillInstructions

L2 Task Cache Layer:
  IntentProfile / ProjectState / TaskState / RelevantMemories / AcceptanceCriteria

L3 Dynamic Step Layer:
  EvidenceRefs / RecentMessages / CurrentUserInput
```

L0 / L1 / L2 禁止混入当前时间、随机 ID、请求 ID、最新工具结果和其他动态内容。所有块内集合必须按稳定 ID 或 name 排序。

优先级建议：

1. 先实现 Project、Task、TaskAttempt、TaskEvent 和状态机。
2. 再实现 Planner、Validator、Scheduler 和 Executor。
3. 再接入最小 Function Runtime / Tool Runtime。
4. 再加入 Tester、Reviewer 和 Fix Task。
5. 最后扩展 Web Console、Browser Runtime、Memory Runtime、Skill Runtime 和 Persona Runtime。
