# M10.2 Capability Validator + Research Evidence Result

日期：2026-04-30

## 背景

用户指出 M10.1 后仍有一批通用 Agent 化问题：god file、demo workflow、硬编码 prompt、静态调研任务、任务分解粒度、上下文预算、code index 重建、TESTING/REVIEWING 卡死、MinimalTester/Reviewer、LLM timeout、Capability Validator 等。

开始本轮前先重新检查当前源码。新基线已经包含多项改动：

1. demo 入口已委托 `RunGeneric`，不再走专用 executor 主路径。
2. prompt 已迁入 `internal/prompts/defaults`，支持 `AGENT_GOGO_PROMPT_DIR` 外部覆盖。
3. LLM provider 已包 `TimeoutProvider`。
4. `Runtime.ContextMaxChars` 已接入 ContextBuilder 序列化后截断。
5. `CapabilityTaskValidator` 已接入 CLI/Web 主路径。
6. `codeindex_cache.go` 已存在。
7. `NewService` 默认 tester/reviewer 已从 minimal 切到 evidence-based。
8. `createRepairTask` 已尝试把 TESTING/REVIEWING 原任务转为 FAILED。

## 本轮完成

1. **研究任务硬验收**
   - GenericEvidenceTester 新增 research evidence gate。
   - 研究/调研/context-gathering 类任务必须有成功 discovery tool evidence。
   - 可接受工具包括：`code.index`、`code.search`、`code.symbols`、`file.read`、`browser.open`、`browser.extract`、`browser.dom_summary`、`git.status`、`git.diff`。
   - 这避免“调研阶段只是静态任务模板 + 模型自称完成”。

2. **code index cache 失效边界**
   - 修复 `file.read` 误清 code index cache。
   - 补上 `file.patch` 后清 cache。
   - 现在读文件不会导致 `code.index` / `code.symbols` 重建，写入和 patch 才会失效。

3. **结果文档可跟踪**
   - `.gitignore` 改为允许 `docs/results/m10_2_*.md` 被 git 看到。
   - 忽略前端 `*.tsbuildinfo` 本地构建产物。

## 三方向测试

方向一：Tester / Research Evidence

```bash
go test ./internal/tester
```

结果：通过。

覆盖点：

1. 研究任务只有 `agent.finish` 和普通写文件证据时失败。
2. 研究任务存在成功 `code.index` discovery tool call 时通过。

方向二：Tool Runtime / Code Index Cache

```bash
go test ./internal/tools
```

结果：通过。

覆盖点：

1. 首次 `code.index` 为 cold cache。
2. `file.read` 后再次 `code.index` 命中 cache。
3. `file.patch` 后再次 `code.index` 不命中 cache。

方向三：Validator + Runtime Repair

```bash
go test ./internal/validator ./internal/runtime
```

结果：通过。

覆盖点：

1. Capability Validator 仍能阻断不可用能力。
2. shell policy 仍能阻断需要 shell 的 task。
3. Tester 失败后原任务会进入 FAILED，并生成 repair task。

真实 LLM smoke：

```bash
RUN_DEEPSEEK_SMOKE=1 DEEPSEEK_API_KEY=*** go test ./internal/planner -run TestDeepSeekPlannerStructuredSmoke -count=1 -v
```

结果：通过。

覆盖点：

1. `code`：修复失败 Go 测试类规划。
2. `web`：读取网页并总结类规划。
3. `document`：根据 README 写项目简介类规划。

全量回归：

```bash
go test ./...
```

结果：通过。

## 剩余风险

1. app / runtime 仍可以继续拆文件；当前基线已经比之前收敛，但 `internal/app/app.go` 仍承担 CLI/Web/provider/browser/bootstrap 多类职责。
2. 分层任务分解目前主要靠 planner prompt 和数量 guard，尚未做真正的 two-pass phase planner。
3. 研究任务现在会被 Tester 要求工具证据，但规划前仍不会主动调用工具做 pre-planning research。
4. Capability Validator 当前以启发式文本推断能力，后续应让 Planner 输出显式 required_capabilities，再由 Validator 精确校验。
