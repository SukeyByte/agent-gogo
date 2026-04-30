import type {
  Project, Task, TaskAttempt, TaskEvent, ToolCall, Observation,
  TestResult, ReviewResult, Artifact, ChatMessage, SkillCard,
  PersonaCard, MemoryItem, ChannelInfo, AppConfig, DashboardStats,
  ProviderStatus, ChainDecision, FileEntry, Session
} from './types'

const now = new Date().toISOString()
const ago = (h: number) => new Date(Date.now() - h * 3600000).toISOString()

export const mockDashboardStats: DashboardStats = {
  project_count: 5,
  task_count: 23,
  done_count: 14,
  running_count: 2,
  failed_count: 1,
}

export const mockProviders: ProviderStatus[] = [
  { name: 'DeepSeek Chat', type: 'llm', status: 'connected', detail: 'deepseek-chat, latency 1.2s' },
  { name: 'Chrome MCP', type: 'browser', status: 'connected', detail: 'http://127.0.0.1:9222' },
  { name: 'SQLite', type: 'storage', status: 'connected', detail: './data/agent.db (2.3MB)' },
  { name: 'OpenAI Embedding', type: 'embedding', status: 'disconnected', detail: 'API key not configured' },
]

export const mockProjects: Project[] = [
  { id: 'proj-1', name: 'Fix Login Bug', goal: '修复 Go 项目的登录 bug，并跑测试确认', status: 'ACTIVE', created_at: ago(48), updated_at: ago(2) },
  { id: 'proj-2', name: '淘宝详情页生成', goal: '生成一套淘宝详情页结构和文案', status: 'ACTIVE', created_at: ago(24), updated_at: ago(1) },
  { id: 'proj-3', name: '传奇世界观故事', goal: '根据传奇世界观拆成 10 个小故事', status: 'ACTIVE', created_at: ago(12), updated_at: ago(0.5) },
  { id: 'proj-4', name: '后台商品修改', goal: '打开后台修改商品标题并截图确认', status: 'COMPLETED', created_at: ago(72), updated_at: ago(48) },
  { id: 'proj-5', name: 'API 文档编写', goal: '为 REST API 编写完整的 OpenAPI 文档', status: 'ARCHIVED', created_at: ago(168), updated_at: ago(120) },
]

export const mockTasks: Record<string, Task[]> = {
  'proj-1': [
    { id: 'task-1-1', project_id: 'proj-1', title: '扫描项目结构', description: '识别代码语言、入口文件、测试命令和关键目录', status: 'DONE', acceptance_criteria: ['输出项目结构摘要', '识别至少一个测试入口'], depends_on: [], created_at: ago(47), updated_at: ago(46) },
    { id: 'task-1-2', project_id: 'proj-1', title: '定位登录相关代码', description: '搜索 login/auth/session 相关文件和函数', status: 'DONE', acceptance_criteria: ['列出相关文件', '列出候选修改点'], depends_on: ['task-1-1'], created_at: ago(47), updated_at: ago(44) },
    { id: 'task-1-3', project_id: 'proj-1', title: '修复登录 Bug', description: '根据分析结果修复 session 处理逻辑', status: 'IN_PROGRESS', acceptance_criteria: ['修改后的代码通过 review', '无新的 lint 错误'], depends_on: ['task-1-2'], created_at: ago(47), updated_at: ago(3) },
    { id: 'task-1-4', project_id: 'proj-1', title: '运行测试', description: '运行 go test 确认修复正确', status: 'READY', acceptance_criteria: ['所有测试通过', '无 race condition'], depends_on: ['task-1-3'], created_at: ago(47), updated_at: ago(47) },
    { id: 'task-1-5', project_id: 'proj-1', title: '验收确认', description: '确认登录功能恢复正常', status: 'DRAFT', acceptance_criteria: ['手动验证登录成功', '截图确认'], depends_on: ['task-1-4'], created_at: ago(47), updated_at: ago(47) },
  ],
  'proj-3': [
    { id: 'task-3-1', project_id: 'proj-3', title: '整理世界观设定', description: '收集并整理传奇世界观的背景信息', status: 'DONE', acceptance_criteria: ['世界观文档完成', '关键人物列表'], depends_on: [], created_at: ago(11), updated_at: ago(10) },
    { id: 'task-3-2', project_id: 'proj-3', title: '故事大纲', description: '生成 10 个小故事的大纲', status: 'DONE', acceptance_criteria: ['10 个故事标题和摘要', '时间线无冲突'], depends_on: ['task-3-1'], created_at: ago(11), updated_at: ago(8) },
    { id: 'task-3-3', project_id: 'proj-3', title: '第 1 章：起源', description: '书写第一个小故事', status: 'REVIEWING', acceptance_criteria: ['基于世界观设定', '字数 2000-3000', 'used_facts 完整'], depends_on: ['task-3-2'], created_at: ago(6), updated_at: ago(1) },
    { id: 'task-3-4', project_id: 'proj-3', title: '第 2 章：试炼', description: '书写第二个小故事', status: 'TESTING', acceptance_criteria: ['基于世界观设定', '字数 2000-3000', '无未授权新设定'], depends_on: ['task-3-2'], created_at: ago(6), updated_at: ago(2) },
    { id: 'task-3-5', project_id: 'proj-3', title: '第 3 章：觉醒', description: '书写第三个小故事', status: 'IN_PROGRESS', acceptance_criteria: ['基于世界观设定', '字数 2000-3000'], depends_on: ['task-3-2'], created_at: ago(6), updated_at: ago(0.5) },
    { id: 'task-3-6', project_id: 'proj-3', title: '第 4 章：背叛', description: '书写第四个小故事', status: 'READY', acceptance_criteria: ['基于世界观设定', '字数 2000-3000'], depends_on: ['task-3-2', 'task-3-3'], created_at: ago(6), updated_at: ago(6) },
    { id: 'task-3-7', project_id: 'proj-3', title: '第 5 章：抉择', description: '书写第五个小故事', status: 'DRAFT', acceptance_criteria: ['基于世界观设定', '字数 2000-3000'], depends_on: ['task-3-2', 'task-3-4'], created_at: ago(6), updated_at: ago(6) },
  ],
  'default': [
    { id: 'task-d-1', project_id: 'proj-4', title: '登录后台', description: '打开后台页面并登录', status: 'DONE', acceptance_criteria: ['成功进入后台首页'], depends_on: [], created_at: ago(70), updated_at: ago(69) },
    { id: 'task-d-2', project_id: 'proj-4', title: '搜索商品', description: '搜索并找到目标商品', status: 'DONE', acceptance_criteria: ['找到目标商品', '进入编辑页面'], depends_on: ['task-d-1'], created_at: ago(70), updated_at: ago(68) },
    { id: 'task-d-3', project_id: 'proj-4', title: '修改标题', description: '修改商品标题为新版文案', status: 'DONE', acceptance_criteria: ['标题修改成功', '截图确认'], depends_on: ['task-d-2'], created_at: ago(70), updated_at: ago(66) },
  ],
}

export function getTasksForProject(projectId: string): Task[] {
  return mockTasks[projectId] || mockTasks['default']
}

export const mockAttempts: Record<string, TaskAttempt[]> = {
  'task-1-3': [
    { id: 'att-1-3-1', task_id: 'task-1-3', number: 1, status: 'FAILED', started_at: ago(5), ended_at: ago(4.5), error: '修改引入了新的 lint 错误：unused variable' },
    { id: 'att-1-3-2', task_id: 'task-1-3', number: 2, status: 'RUNNING', started_at: ago(3), ended_at: null, error: '' },
  ],
  'task-3-3': [
    { id: 'att-3-3-1', task_id: 'task-3-3', number: 1, status: 'SUCCEEDED', started_at: ago(3), ended_at: ago(1.5), error: '' },
  ],
  'task-3-5': [
    { id: 'att-3-5-1', task_id: 'task-3-5', number: 1, status: 'RUNNING', started_at: ago(0.5), ended_at: null, error: '' },
  ],
}

export function getAttemptsForTask(taskId: string): TaskAttempt[] {
  return mockAttempts[taskId] || [
    { id: `${taskId}-att-1`, task_id: taskId, number: 1, status: 'SUCCEEDED', started_at: ago(10), ended_at: ago(9), error: '' },
  ]
}

export const mockToolCalls: Record<string, ToolCall[]> = {
  'att-1-3-2': [
    { id: 'tc-1', attempt_id: 'att-1-3-2', name: 'code.search', input_json: '{"query":"session","path":"./internal/auth"}', output_json: '{"matches":[{"file":"session.go","line":42,"text":"func (s *Session) Validate() error"}]}', status: 'SUCCEEDED', error: '', evidence_ref: '', created_at: ago(2.5), updated_at: ago(2.5) },
    { id: 'tc-2', attempt_id: 'att-1-3-2', name: 'file.read', input_json: '{"path":"internal/auth/session.go","start_line":40,"end_line":55}', output_json: '{"content":"func (s *Session) Validate() error {\\n  if s.Expired() {\\n    return ErrSessionExpired\\n  }\\n  return nil\\n}" }', status: 'SUCCEEDED', error: '', evidence_ref: '', created_at: ago(2.3), updated_at: ago(2.3) },
    { id: 'tc-3', attempt_id: 'att-1-3-2', name: 'file.patch', input_json: '{"path":"internal/auth/session.go","old":"return ErrSessionExpired","new":"return ErrSessionExpired\\n// TODO: refresh token"}', output_json: '{"applied":true}', status: 'SUCCEEDED', error: '', evidence_ref: 'ev-1', created_at: ago(2), updated_at: ago(2) },
  ],
  'att-3-5-1': [
    { id: 'tc-4', attempt_id: 'att-3-5-1', name: 'document.write', input_json: '{"path":"stories/chapter3.md","content":"# 第三章：觉醒\\n\\n黎明前的黑暗最为浓重..."}', output_json: '{"written":true,"size":2847}', status: 'SUCCEEDED', error: '', evidence_ref: '', created_at: ago(0.3), updated_at: ago(0.3) },
    { id: 'tc-5', attempt_id: 'att-3-5-1', name: 'memory.save', input_json: '{"summary":"第3章主角开始觉醒","tags":["story","chapter3"]}', output_json: '{"saved":true}', status: 'SUCCEEDED', error: '', evidence_ref: '', created_at: ago(0.2), updated_at: ago(0.2) },
  ],
}

export function getToolCallsForAttempt(attemptId: string): ToolCall[] {
  return mockToolCalls[attemptId] || []
}

export const mockObservations: Record<string, Observation[]> = {
  'att-1-3-2': [
    { id: 'obs-1', attempt_id: 'att-1-3-2', tool_call_id: 'tc-1', type: 'search_result', summary: '在 session.go 第 42 行找到 Validate 方法', evidence_ref: '', payload: '', created_at: ago(2.5) },
    { id: 'obs-2', attempt_id: 'att-1-3-2', tool_call_id: 'tc-3', type: 'file_changed', summary: '成功修改 session.go，添加了 refresh token 逻辑', evidence_ref: 'ev-1', payload: '', created_at: ago(2) },
  ],
}

export const mockEvents: Record<string, TaskEvent[]> = {
  'task-1-3': [
    { id: 'ev-1', task_id: 'task-1-3', attempt_id: '', type: 'status_changed', from_state: 'READY', to_state: 'IN_PROGRESS', message: '开始执行', payload: '', created_at: ago(5) },
    { id: 'ev-2', task_id: 'task-1-3', attempt_id: 'att-1-3-1', type: 'attempt_failed', from_state: '', to_state: '', message: '修改引入了新的 lint 错误', payload: '{"error":"unused variable"}', created_at: ago(4.5) },
    { id: 'ev-3', task_id: 'task-1-3', attempt_id: 'att-1-3-2', type: 'attempt_started', from_state: '', to_state: '', message: '第 2 次尝试', payload: '', created_at: ago(3) },
    { id: 'ev-4', task_id: 'task-1-3', attempt_id: 'att-1-3-2', type: 'tool_called', from_state: '', to_state: '', message: 'code.search session', payload: '{"tool":"code.search"}', created_at: ago(2.5) },
    { id: 'ev-5', task_id: 'task-1-3', attempt_id: 'att-1-3-2', type: 'tool_called', from_state: '', to_state: '', message: 'file.patch session.go', payload: '{"tool":"file.patch"}', created_at: ago(2) },
  ],
  'task-3-3': [
    { id: 'ev-6', task_id: 'task-3-3', attempt_id: '', type: 'status_changed', from_state: 'IN_PROGRESS', to_state: 'IMPLEMENTED', message: '故事完成', payload: '', created_at: ago(3) },
    { id: 'ev-7', task_id: 'task-3-3', attempt_id: '', type: 'status_changed', from_state: 'IMPLEMENTED', to_state: 'TESTING', message: '开始测试', payload: '', created_at: ago(2) },
    { id: 'ev-8', task_id: 'task-3-3', attempt_id: '', type: 'status_changed', from_state: 'TESTING', to_state: 'REVIEWING', message: '测试通过，开始验收', payload: '', created_at: ago(1) },
  ],
}

export function getEventsForTask(taskId: string): TaskEvent[] {
  return mockEvents[taskId] || [
    { id: `${taskId}-ev-1`, task_id: taskId, attempt_id: '', type: 'status_changed', from_state: 'READY', to_state: 'IN_PROGRESS', message: '开始执行', payload: '', created_at: ago(8) },
    { id: `${taskId}-ev-2`, task_id: taskId, attempt_id: '', type: 'status_changed', from_state: 'IN_PROGRESS', to_state: 'DONE', message: '任务完成', payload: '', created_at: ago(7) },
  ]
}

export const mockTestResults: Record<string, TestResult[]> = {
  'att-3-3-1': [
    { id: 'tr-1', attempt_id: 'att-3-3-1', name: '设定一致性检查', status: 'PASSED', output: '所有 used_facts 与世界观设定一致', evidence_ref: '', created_at: ago(2) },
    { id: 'tr-2', attempt_id: 'att-3-3-1', name: '字数检查', status: 'PASSED', output: '字数 2847，符合 2000-3000 范围', evidence_ref: '', created_at: ago(2) },
  ],
}

export const mockReviewResults: Record<string, ReviewResult[]> = {
  'att-3-3-1': [
    { id: 'rr-1', attempt_id: 'att-3-3-1', status: 'APPROVED', summary: '故事基于世界观设定，无未授权新设定，风格一致', evidence_ref: '', created_at: ago(1) },
  ],
}

export const mockArtifacts: Record<string, Artifact[]> = {
  'proj-1': [
    { id: 'art-1', attempt_id: 'att-1-3-1', project_id: 'proj-1', type: 'diff', path: 'internal/auth/session.go.patch', description: 'session.go 修复补丁', created_at: ago(4.5) },
    { id: 'art-2', attempt_id: 'att-1-3-2', project_id: 'proj-1', type: 'diff', path: 'internal/auth/session.go.patch', description: 'session.go 第2次修复', created_at: ago(2) },
  ],
  'proj-3': [
    { id: 'art-3', attempt_id: 'att-3-3-1', project_id: 'proj-3', type: 'document', path: 'stories/chapter1.md', description: '第1章：起源', created_at: ago(1.5) },
    { id: 'art-4', attempt_id: 'att-3-5-1', project_id: 'proj-3', type: 'document', path: 'stories/chapter3.md', description: '第3章：觉醒', created_at: ago(0.3) },
    { id: 'art-5', attempt_id: '', project_id: 'proj-3', type: 'document', path: 'worldbuilding/legend-world.md', description: '传奇世界观设定', created_at: ago(10) },
  ],
}

export const mockChatMessages: ChatMessage[] = [
  { id: 'msg-1', session_id: 'sess-1', project_id: '', role: 'user', content: '帮我修复这个 Go 项目的登录 bug，并跑测试确认', artifacts: [], metadata: {}, created_at: ago(48) },
  { id: 'msg-2', session_id: 'sess-1', project_id: 'proj-1', role: 'assistant', content: '好的，我来分析这个登录 bug。让我先扫描项目结构，定位相关代码。\n\n**Chain Decision:** L3 - 复杂代码任务\n**Plan:** 已拆分为 5 个任务', artifacts: [], metadata: { chain_level: 'L3', task_count: 5 }, created_at: ago(47.9) },
  { id: 'msg-3', session_id: 'sess-1', project_id: 'proj-1', role: 'tool', content: '执行 tool: code.search(session) → 找到 3 个匹配文件', artifacts: [], metadata: { tool: 'code.search' }, created_at: ago(47.5) },
  { id: 'msg-4', session_id: 'sess-1', project_id: 'proj-1', role: 'assistant', content: '已定位到 `internal/auth/session.go` 中的 Validate 方法。开始修复...\n\n第 1 次尝试失败（lint 错误），正在进行第 2 次尝试。', artifacts: ['art-1', 'art-2'], metadata: {}, created_at: ago(2) },
]

export const mockSkills: SkillCard[] = [
  {
    id: 'skill-pdf-edit', name: 'pdf-edit', description: 'Edit PDFs with natural-language instructions',
    allowed_tools: ['file.read', 'file.write'], path: '.claude/skills/pdf-edit/SKILL.md',
    version_hash: 'abc123', frontmatter: { name: 'pdf-edit', 'user-invocable': 'true' },
    body: '# PDF Edit Skill\n\nUse this skill when the user wants to edit a PDF file.\n\n## Steps\n1. Read the PDF file\n2. Understand the edit request\n3. Apply changes\n4. Save the modified PDF',
  },
  {
    id: 'skill-go-debug', name: 'go-debug', description: 'Debug Go applications with structured approach',
    allowed_tools: ['code.search', 'code.symbols', 'file.read', 'shell.run', 'test.run'],
    path: '.claude/skills/go-debug/SKILL.md', version_hash: 'def456',
    frontmatter: { name: 'go-debug', 'allowed-tools': 'Read Grep Bash(go test *)' },
    body: '# Go Debug Skill\n\n## Steps\n1. Build code index\n2. Search for error-related code\n3. Read relevant files\n4. Apply fix\n5. Run tests',
  },
  {
    id: 'skill-browser-form', name: 'browser-form-fill', description: 'Fill web forms using browser automation',
    allowed_tools: ['browser.open', 'browser.click', 'browser.type', 'browser.input', 'browser.screenshot'],
    path: '.claude/skills/browser-form-fill/SKILL.md', version_hash: 'ghi789',
    frontmatter: { name: 'browser-form-fill', 'user-invocable': 'true' },
    body: '# Browser Form Fill Skill\n\n## Steps\n1. Open the target URL\n2. Analyze form fields\n3. Fill each field\n4. Submit\n5. Verify result',
  },
  {
    id: 'skill-story-write', name: 'story-writing', description: 'Write grounded stories based on worldbuilding settings',
    allowed_tools: ['document.write', 'memory.save'], path: '.claude/skills/story-writing/SKILL.md',
    version_hash: 'jkl012', frontmatter: { name: 'story-writing', description: 'Write grounded stories' },
    body: '# Story Writing Skill\n\n## Constraints\n- Only use facts from Background\n- Mark new assumptions\n- Output used_facts',
  },
  {
    id: 'skill-explain-code', name: 'explain-code', description: 'Explains code with visual diagrams and analogies',
    allowed_tools: ['Read', 'Grep'], path: '~/.claude/skills/explain-code/SKILL.md',
    version_hash: 'mno345', frontmatter: { name: 'explain-code', 'user-invocable': 'true', 'disable-model-invocation': 'false' },
    body: '# Explain Code Skill\n\nUse visual diagrams and analogies to explain code.',
  },
]

export const mockPersonas: PersonaCard[] = [
  {
    id: 'persona-main', name: '默认助手', type: 'main', description: '默认协作风格：简洁、工程化、先给结论',
    path: 'personas/main.md', version_hash: 'p1', style_rules: ['简洁直接', '先给结论再给分析', '使用 markdown 格式'], boundaries: ['不猜测用户意图'], instructions: '你是 Agent GoGo 的默认助手。',
  },
  {
    id: 'persona-web', name: 'Web Console', type: 'channel', description: 'Web Console 通道人格：支持结构化展示',
    path: 'personas/channel-web.md', version_hash: 'p2', style_rules: ['使用任务卡展示', '支持实时日志流'], boundaries: [], instructions: '面向 Web Console 用户。',
  },
  {
    id: 'persona-code-reviewer', name: '代码审查员', type: 'role', description: '严格代码审查，关注安全和性能',
    path: 'personas/code-reviewer.md', version_hash: 'p3', style_rules: ['检查安全漏洞', '检查性能问题', '关注错误处理'], boundaries: ['不修改代码，只给建议'], instructions: '你是严格的代码审查员。',
  },
  {
    id: 'persona-novelist', name: '传奇故事创作者', type: 'role', description: '基于世界观的创作，保持设定一致性',
    path: 'personas/novelist.md', version_hash: 'p4', style_rules: ['保持世界观一致', '不自由新增核心设定', '每段故事必须基于输入背景'], boundaries: ['如需新增设定，必须标记为 new_assumption'], instructions: '你是传奇世界的创作者。',
  },
  {
    id: 'persona-test-engineer', name: '测试工程师', type: 'role', description: '自动化测试专家',
    path: 'personas/test-engineer.md', version_hash: 'p5', style_rules: ['优先自动化测试', '关注边界条件'], boundaries: [], instructions: '你是测试工程师。',
  },
  {
    id: 'persona-project-story', name: '传奇世界观编辑', type: 'project', description: '传奇世界观项目风格约束',
    path: 'personas/project-story.md', version_hash: 'p6', style_rules: ['所有创作必须基于世界观', '新设定需要标记'], boundaries: [], instructions: '传奇世界观项目角色。',
  },
]

export const mockMemories: MemoryItem[] = [
  { id: 'mem-1', scope: 'project', type: 'exact', tags: ['go', 'test', 'command'], summary: 'Go 测试命令为 go test ./...', body: '项目使用 go test ./... 作为测试命令', confidence: 1.0, artifact_ref: '', source_task_id: 'task-1-1', version_hash: 'm1' },
  { id: 'mem-2', scope: 'project', type: 'exact', tags: ['go', 'structure', 'entry'], summary: '项目入口文件 cmd/agent-gogo/main.go', body: '项目入口为 cmd/agent-gogo/main.go，使用 app.Main() 分发', confidence: 1.0, artifact_ref: '', source_task_id: 'task-1-1', version_hash: 'm2' },
  { id: 'mem-3', scope: 'project', type: 'fuzzy', tags: ['story', 'worldbuilding', 'character'], summary: '传奇世界观主要人物设定', body: '主角：李明（剑修，觉醒期）、反派：暗影王（魔道修士）、导师：云长老', confidence: 0.95, artifact_ref: 'worldbuilding/legend-world.md', source_task_id: 'task-3-1', version_hash: 'm3' },
  { id: 'mem-4', scope: 'project', type: 'fuzzy', tags: ['story', 'worldbuilding', 'rules'], summary: '传奇世界修炼体系', body: '修炼等级：炼气、筑基、金丹、元婴、化神、合体、大乘、渡劫', confidence: 0.9, artifact_ref: 'worldbuilding/legend-world.md', source_task_id: 'task-3-1', version_hash: 'm4' },
  { id: 'mem-5', scope: 'long_term', type: 'exact', tags: ['user', 'preference', 'style'], summary: '用户偏好简洁回复', body: '用户喜欢简洁直接的回复风格，不需要过多解释', confidence: 1.0, artifact_ref: '', source_task_id: '', version_hash: 'm5' },
  { id: 'mem-6', scope: 'long_term', type: 'exact', tags: ['user', 'preference', 'language'], summary: '用户默认中文交流', body: '用户使用中文进行所有交流', confidence: 1.0, artifact_ref: '', source_task_id: '', version_hash: 'm6' },
  { id: 'mem-7', scope: 'working', type: 'fuzzy', tags: ['task', 'progress'], summary: '第 3 章正在创作中', body: '第 3 章"觉醒"正在由 Executor 执行，主角开始觉醒修炼天赋', confidence: 0.85, artifact_ref: 'stories/chapter3.md', source_task_id: 'task-3-5', version_hash: 'm7' },
]

export const mockChannels: ChannelInfo[] = [
  {
    id: 'ch-web', type: 'web', name: 'Web Console', enabled: true,
    capabilities: { supported_message_types: ['text', 'task_card', 'artifact'], supported_interactions: ['modal', 'button'], supports_confirmation: true, supports_streaming: true, supports_file_request: true },
  },
  {
    id: 'ch-cli', type: 'cli', name: 'CLI', enabled: true,
    capabilities: { supported_message_types: ['text'], supported_interactions: ['prompt', 'confirm_yes_no'], supports_confirmation: true, supports_streaming: false, supports_file_request: false },
  },
  {
    id: 'ch-api', type: 'api', name: 'HTTP API', enabled: true,
    capabilities: { supported_message_types: ['json'], supported_interactions: [], supports_confirmation: false, supports_streaming: false, supports_file_request: false },
  },
]

export const mockConfig: AppConfig = {
  llm: { provider: 'deepseek', model: 'deepseek-chat', base_url: 'https://api.deepseek.com', api_key: 'sk-***masked***', timeout: 120 },
  embedding: { provider: 'openai', model: 'text-embedding-3-small', base_url: 'https://api.openai.com', api_key: '' },
  browser: { provider: 'chrome_mcp', mcp_url: 'http://127.0.0.1:9222', headless: false, timeout: 60 },
  storage: { workspace_path: '.', sqlite_path: './data/agent.db', artifact_path: './data/artifacts', log_path: './logs', skill_roots: ['~/.claude/skills', '.claude/skills'], persona_path: './personas' },
  memory: { max_working_items: 100, max_project_items: 500, default_scope: 'project', enable_auto_extract: true },
  runtime: { max_tasks_per_project: 50, max_retries: 3, token_budget: 32000, enable_prompt_cache: true, enable_auto_repair: true, enable_debug_log: false },
  chain_router: { l0_max_tokens: 1024, l1_max_tools: 3, l2_max_tasks: 10, auto_plan_threshold: 'medium' },
  security: { require_confirm_high_risk: true, allow_shell: false, allow_auto_execute_high_risk: false },
}

export const mockChainDecision: ChainDecision = {
  level: 'L3', reason: '复杂代码任务：需要扫描项目、定位代码、修改、测试和验收', need_plan: true, need_tools: true, need_memory: true, need_review: true, need_browser: false, risk_level: 'medium',
}

export const mockFiles: FileEntry[] = [
  { name: 'stories', path: 'data/artifacts/stories', type: 'dir', size: 0, modified: ago(0.3) },
  { name: 'chapter1.md', path: 'data/artifacts/stories/chapter1.md', type: 'file', size: 2847, modified: ago(1.5) },
  { name: 'chapter3.md', path: 'data/artifacts/stories/chapter3.md', type: 'file', size: 2847, modified: ago(0.3) },
  { name: 'worldbuilding', path: 'data/artifacts/worldbuilding', type: 'dir', size: 0, modified: ago(10) },
  { name: 'legend-world.md', path: 'data/artifacts/worldbuilding/legend-world.md', type: 'file', size: 15230, modified: ago(10) },
  { name: 'memory', path: 'data/artifacts/memory', type: 'dir', size: 0, modified: ago(2) },
  { name: 'project-memory.md', path: 'data/artifacts/memory/project-memory.md', type: 'file', size: 4500, modified: ago(2) },
  { name: 'session.go.patch', path: 'data/artifacts/internal/auth/session.go.patch', type: 'file', size: 320, modified: ago(2) },
  { name: 'agent.db', path: 'data/agent.db', type: 'file', size: 2411724, modified: ago(0.1) },
]

export const mockSessions: Session[] = [
  { id: 'sess-1', user_id: 'sukeke', channel_type: 'cli', channel_id: 'local', project_id: 'proj-1', status: 'ACTIVE', title: '修复登录 Bug', last_active_at: ago(0.1), created_at: ago(48), updated_at: ago(0.1) },
  { id: 'sess-2', user_id: 'sukeke', channel_type: 'web', channel_id: 'web-local', project_id: 'proj-3', status: 'ACTIVE', title: '传奇世界观故事', last_active_at: ago(0.5), created_at: ago(12), updated_at: ago(0.5) },
  { id: 'sess-3', user_id: 'sukeke', channel_type: 'cli', channel_id: 'local', project_id: 'proj-2', status: 'PAUSED', title: '淘宝详情页生成', last_active_at: ago(6), created_at: ago(24), updated_at: ago(6) },
  { id: 'sess-4', user_id: 'sukeke', channel_type: 'cli', channel_id: 'local', project_id: 'proj-4', status: 'COMPLETED', title: '后台商品修改', last_active_at: ago(48), created_at: ago(72), updated_at: ago(48) },
  { id: 'sess-5', user_id: 'sukeke', channel_type: 'cli', channel_id: 'local', project_id: '', status: 'EXPIRED', title: '临时问答', last_active_at: ago(30), created_at: ago(36), updated_at: ago(30) },
]
