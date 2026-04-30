# M10.4 WebConsole Channel 接入 实现结果

日期：2026-04-30
状态：已实现

---

## 背景

Web Console 前端 (Vue 3 + Vite + Tailwind CSS) 已有 12 个页面和 mock 数据 API 层。后端有完整的 `communication.Adapter` 接口和 `runtime.Service` 管线，但 `RunWebConsole` 和 `RunGeneric` 两条路径互不相连——Web 服务器只有只读 store，没有 runtime，前端只能用 mock 数据。

目标：把 Web Console 接入为真正的 channel，所有写入走 `HandleChannelEvent`，所有读取走 store JSON API，实时消息走 SSE。**不侵入 core**（不改 `internal/communication/`、`internal/runtime/` 内部实现）。

## 实现内容

### 1. SSE Hub (`web/handlers/sse_hub.go` — NEW)

纯基础设施，无业务依赖。

- `SSEHub`：基于 channelID 的 pub/sub，支持多客户端订阅
- `Subscribe(channelID)` → 返回 `<-chan SSEEvent` 和 `unsubscribe` 函数
- `Publish(channelID, SSEEvent)`：向所有订阅者广播
- `Replay(channelID, afterID)`：支持 `Last-Event-ID` 重连
- bufferSize=50（历史回放窗口），client channel buffer=64，慢消费者自动断开

### 2. WebConsoleAdapter (`web/handlers/webconsole_adapter.go` — NEW)

在 web 层实现 `communication.Adapter` 接口（不改动 `internal/communication/` 任何文件）。

- `Capability()`：返回 web channel 能力（streaming / confirmation / async / 8 buttons）
- `Deliver()`：序列化 `RenderedMessage` → `SSEEvent` → `hub.Publish()`
- Intent 类型映射：`IntentAskConfirmation`→"confirmation"、`IntentNotifyDone`→"done"、`IntentSendMessage`→"message"

### 3. JSON Read API (`web/handlers/api_handlers.go` — NEW)

DTO 映射层 + GET 端点。在 handler 层做 `domain` → `jsonDTO` 映射，不碰 `internal/domain/model.go`。

端点：

| 端点 | 数据来源 |
|------|---------|
| `GET /api/stats` | 聚合 project/task 计数 |
| `GET /api/projects` | store.ListProjects |
| `GET /api/projects/:id` | store.GetProject |
| `GET /api/projects/:id/tasks` | store.ListTasksByProject |
| `GET /api/tasks/:id` | store.GetTask |
| `GET /api/tasks/:id/attempts` | store.ListTaskAttemptsByTask |
| `GET /api/tasks/:id/events` | store.ListTaskEvents |
| `GET /api/attempts/:id/tool-calls` | store.ListToolCallsByAttempt |
| `GET /api/attempts/:id/observations` | store.ListObservationsByAttempt |
| `GET /api/projects/:id/artifacts` | store.ListArtifactsByProject |
| `GET /api/config` | ConfigView JSON |
| `GET /api/events` | SSE 流 |

`writeJSON` 使用 `reflect` 处理 typed nil slice → `[]`，避免前端收到 `null`。

### 4. Inbound Message (`web/handlers/api_inbound.go` — NEW)

单一写入入口：

- `POST /api/message`：接收 `{ type, text, payload }` → 调用 `ChannelEventSender.HandleChannelEvent()` → 返回 `202 Accepted`
- `POST /api/confirmation`：接收 approve/reject → 调用 `ChannelEventSender.HandleUserConfirmation()`

### 5. API Server (`web/handlers/api_server.go` — NEW)

路由注册 + SPA 静态文件服务。

```
APIServer { store, sender, hub, config, channelID, sessionID, distDir }

ServeHTTP:
  /api/* → JSON handlers + SSE
  /* → SPA static files (web/frontend/dist/)，fallback to index.html
```

Content-Type 自动检测：.js、.css、.html、.svg、.png、.ico、.woff/.woff2。

### 6. 接口层 (`web/handlers/service.go` — NEW)

```go
type ChannelEventSender interface {
    HandleChannelEvent(ctx context.Context, event InboundEvent) error
    HandleUserConfirmation(ctx context.Context, confirmation InboundConfirmation) error
}
```

保持 `web/handlers` 不直接依赖 `internal/runtime`，层次清晰。

### 7. App Wiring (`internal/app/app.go` — MODIFY)

`RunWebConsole` 改造为完整 runtime 管线：

1. 加载 config（已有）
2. 打开 store（已有）
3. 创建 SSEHub
4. 尝试 LLM 初始化 → 如果有 API key，调用 `initWebRuntime()` 创建完整 runtime
5. 创建 `runtimeServiceBridge`（适配 `*runtime.Service` → `ChannelEventSender`）
6. 创建 `APIServer(store, sender, hub, config, distDir)`
7. 启动 HTTP server

**优雅降级**：无 LLM key 时以 "read-only mode" 运行，仍可查看所有历史数据，只是不能提交新任务。

`runtimeServiceBridge` 将 `InboundEvent` 映射为 `runtime.ChannelEvent`，支持 `goal.submitted`、`task.retry`、`project.replan` 三种事件类型。

### 8. Frontend API (`web/frontend/src/api/index.ts` — MODIFY)

每个方法替换为真实 fetch + mock 降级：

```typescript
async listProjects(): Promise<Project[]> {
  return withFallback(() => request<Project[]>('/projects'), mockProjects)
}
```

- `request<T>(path)`：通用 GET helper，解析 JSON
- `post(path, body)`：fire-and-forget POST
- `withFallback(fn, fallback)`：try-catch wrapper，后端不可用时自动降级到 mock
- `createEventSource()`：创建 `/api/events` 的 SSE 连接

### 9. Frontend Chat (`web/frontend/src/views/ChatView.vue` — MODIFY)

- `onMounted`：建立 `EventSource('/api/events')`
- 监听 `message`、`done`、`confirmation` 三种 SSE 事件
- `send()`：`POST /api/message`（fire-and-forget），通过 SSE 接收响应
- 确认请求自动显示 Approve/Reject 按钮
- `onUnmounted`：关闭 EventSource

### 10. Frontend 其他页面

- `ProjectsView.vue`：`createProject` → `POST /api/message { type: "goal.submitted", text: goal, payload: { name } }`，500ms 后刷新列表
- `TaskDetailView.vue`：Retry → `POST /api/message { type: "task.retry" }`；Approve/Reject → `POST /api/confirmation`
- `ConfigView.vue`：Save → `POST /api/message { type: "goal.submitted", text: "/config ..." }`
- `ProjectDetailView.vue`：null-safety (`a || []`) 防止后端空数组序列化问题

## 文件清单

| 文件 | 动作 | 说明 |
|------|------|------|
| `web/handlers/sse_hub.go` | NEW | SSE pub/sub |
| `web/handlers/webconsole_adapter.go` | NEW | Adapter 实现 |
| `web/handlers/api_handlers.go` | NEW | JSON 读 API + SSE |
| `web/handlers/api_inbound.go` | NEW | POST 写入 |
| `web/handlers/api_server.go` | NEW | 路由 + SPA 服务 |
| `web/handlers/service.go` | NEW | ChannelEventSender 接口 |
| `internal/app/app.go` | MODIFY | RunWebConsole 接入 runtime |
| `internal/runtime/service.go` | MODIFY | 删除重复声明（已在 runtime_context.go） |
| `internal/runtime/runtime_context.go` | NEW | 从 service.go 提取的辅助方法 |
| `internal/store/sqlite.go` | MODIFY | 修复 RowsAffected 返回值 |
| `internal/session/service.go` | MODIFY | 添加 SaveSessionRuntimeContext 别名方法 |
| `web/frontend/src/api/index.ts` | MODIFY | 真实 fetch + mock fallback |
| `web/frontend/src/views/ChatView.vue` | MODIFY | SSE EventSource |
| `web/frontend/src/views/ConfigView.vue` | MODIFY | save → channel command |
| `web/frontend/src/views/ProjectsView.vue` | MODIFY | createProject → channel |
| `web/frontend/src/views/TaskDetailView.vue` | MODIFY | actions → channel |
| `web/frontend/src/views/ProjectDetailView.vue` | MODIFY | null-safety |

## 架构设计

```
浏览器 ← SSE ─→ API Server ──hub.Publish()──→ SSEHub ──Subscribe()──→ 浏览器
                 │
                 ├── GET /api/* ──→ Store (只读)
                 │
                 ├── POST /api/message ──→ ChannelEventSender
                 │                            │
                 │                   runtimeServiceBridge
                 │                            │
                 │                   runtime.Service.HandleChannelEvent()
                 │                            │
                 │                   CreateProject → PlanProject → RunNextTask
                 │                            │
                 │                   Communication.Dispatch()
                 │                            │
                 │                   WebConsoleAdapter.Deliver()
                 │                            │
                 │                   hub.Publish() ──→ SSE ──→ 浏览器
                 │
                 └── POST /api/confirmation ──→ HandleUserConfirmation()
```

核心约束：**不侵入 core**。WebConsoleAdapter 在 `web/handlers` 包内实现 `communication.Adapter`，`runtimeServiceBridge` 在 `internal/app` 内桥接 `runtime.Service` → `ChannelEventSender`。

## 验证结果

1. `go build ./...` 编译通过
2. `go test ./internal/... ./web/...` 全部通过
3. `npm run build` 前端构建通过
4. 启动 `go run ./cmd/agent-gogo web`，浏览器验证：
   - Dashboard 显示真实 SQLite 数据（11 projects, 19 tasks）
   - Projects 列表显示真实项目
   - Project Detail 显示真实 tasks
   - Chat 页面 SSE 连接建立
   - Config 页面读取真实配置
   - 无 console 错误
5. 无 LLM key 时以 read-only mode 运行，所有只读 API 正常工作
6. `POST /api/message` 在无 runtime 时正确返回 `"runtime not available"` 错误

## 仍待完善

1. 配置页面的保存需要后端实现 `/config` 命令解析器
2. SSE 事件需要 runtime 实际运行时才能验证端到端推送
3. 前端 Skills/Personas/Memory/Channels/Files 页面仍使用 mock 数据
4. 需要添加 WebSocket 或增强 SSE 以支持双向通信（当前 confirmation 走单独 POST）
5. 生产环境需要添加 CORS、认证、rate limiting
