// API client — real fetch with mock fallback

import type {
  Project, Task, TaskAttempt, TaskEvent, ToolCall, Observation,
  TestResult, ReviewResult, Artifact, ChatMessage, SkillCard,
  PersonaCard, MemoryItem, ChannelInfo, AppConfig, DashboardStats,
  ProviderStatus, ChainDecision, FileEntry, Session, SessionContext,
  TokenUsageSnapshot
} from './types'

import {
  mockProjects, mockDashboardStats, mockProviders, mockTasks,
  getTasksForProject, getAttemptsForTask, getToolCallsForAttempt,
  mockObservations, getEventsForTask, mockTestResults, mockReviewResults,
  mockArtifacts, mockChatMessages, mockSkills, mockPersonas,
  mockMemories, mockChannels, mockConfig, mockChainDecision, mockFiles,
  mockSessions,
} from './mock-data'

// --- Generic request helper ---

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    ...init,
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`API ${res.status}: ${body}`)
  }
  return res.json()
}

// Fire-and-forget POST (no response body parsing needed)
async function post(path: string, body: unknown): Promise<void> {
  const res = await fetch(`/api${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`API ${res.status}: ${text}`)
  }
}

function withFallback<T>(fn: () => Promise<T>, fallback: T): Promise<T> {
  return fn().catch(() => fallback)
}

// --- SSE ---

export function createEventSource(): EventSource {
  return new EventSource('/api/events')
}

// --- API surface ---

export const api = {
  // Dashboard
  async getDashboardStats(): Promise<DashboardStats> {
    return withFallback(() => request<DashboardStats>('/stats'), mockDashboardStats)
  },
  async getProviders(): Promise<ProviderStatus[]> { return mockProviders },
  async getTokenUsage(): Promise<TokenUsageSnapshot | null> {
    try {
      return await request<TokenUsageSnapshot>('/tokens')
    } catch {
      return null
    }
  },
  async getRecentProjects(): Promise<Project[]> {
    return withFallback(async () => {
      const all = await request<Project[]>('/projects')
      return all.slice(0, 3)
    }, mockProjects.slice(0, 3))
  },

  // Projects
  async listProjects(): Promise<Project[]> {
    return withFallback(() => request<Project[]>('/projects'), mockProjects)
  },
  async getProject(id: string): Promise<Project> {
    return withFallback(() => request<Project>(`/projects/${id}`), mockProjects.find(p => p.id === id) || mockProjects[0])
  },
  async createProject(name: string, goal: string): Promise<void> {
    await post('/message', { type: 'goal.submitted', text: goal, payload: { name } })
  },

  // Tasks
  async listTasks(projectId: string): Promise<Task[]> {
    return withFallback(() => request<Task[]>(`/projects/${projectId}/tasks`), getTasksForProject(projectId))
  },
  async getTask(id: string): Promise<Task> {
    return withFallback(() => request<Task>(`/tasks/${id}`), (() => {
      for (const tasks of Object.values(mockTasks)) {
        const t = tasks.find(t => t.id === id)
        if (t) return t
      }
      return getTasksForProject('proj-1')[0]
    })())
  },
  async retryTask(taskId: string): Promise<void> {
    await post('/message', { type: 'task.retry', task_id: taskId })
  },

  // Attempts
  async listAttempts(taskId: string): Promise<TaskAttempt[]> {
    return withFallback(() => request<TaskAttempt[]>(`/tasks/${taskId}/attempts`), getAttemptsForTask(taskId))
  },

  // Events
  async listEvents(taskId: string): Promise<TaskEvent[]> {
    return withFallback(() => request<TaskEvent[]>(`/tasks/${taskId}/events`), getEventsForTask(taskId))
  },

  // Tool calls
  async listToolCalls(attemptId: string): Promise<ToolCall[]> {
    return withFallback(() => request<ToolCall[]>(`/attempts/${attemptId}/tool-calls`), getToolCallsForAttempt(attemptId))
  },

  // Observations
  async listObservations(attemptId: string): Promise<Observation[]> {
    return withFallback(() => request<Observation[]>(`/attempts/${attemptId}/observations`), mockObservations[attemptId] || [])
  },

  // Test / review results
  async listTestResults(attemptId: string): Promise<TestResult[]> {
    return withFallback(() => request<TestResult[]>(`/attempts/${attemptId}/test-results`), mockTestResults[attemptId] || [])
  },
  async listReviewResults(attemptId: string): Promise<ReviewResult[]> {
    return withFallback(() => request<ReviewResult[]>(`/attempts/${attemptId}/review-results`), mockReviewResults[attemptId] || [])
  },

  // Artifacts
  async listArtifacts(projectId: string): Promise<Artifact[]> {
    return withFallback(() => request<Artifact[]>(`/projects/${projectId}/artifacts`), mockArtifacts[projectId] || [])
  },

  // Chat — fire-and-forget via POST /api/message, responses arrive via SSE
  async listChatMessages(_sessionId: string): Promise<ChatMessage[]> { return [] },
  async sendChatMessage(_sessionId: string, content: string): Promise<void> {
    await post('/message', { type: 'goal.submitted', text: content })
  },
  async getChainDecision(_sessionId: string): Promise<ChainDecision> { return mockChainDecision },

  // Confirmation (approve/reject)
  async sendConfirmation(confirmationId: string, projectId: string, taskId: string, attemptId: string, actionId: string, approved: boolean, message: string): Promise<void> {
    await post('/confirmation', { confirmation_id: confirmationId, project_id: projectId, task_id: taskId, attempt_id: attemptId, action_id: actionId, approved, message })
  },

  // Skills
  async listSkills(): Promise<SkillCard[]> { return withFallback(() => request<SkillCard[]>('/skills'), mockSkills) },
  async getSkill(id: string): Promise<SkillCard> { return withFallback(() => request<SkillCard>(`/skills/${id}`), mockSkills.find(s => s.id === id) || mockSkills[0]) },
  async searchSkills(query: string): Promise<SkillCard[]> { return withFallback(() => request<SkillCard[]>(`/skills?q=${encodeURIComponent(query)}`), mockSkills.filter(s => s.name.includes(query) || s.description.includes(query))) },
  async searchGithubRepos(query: string): Promise<Array<{owner: string; repo: string; full_name: string; description: string; stars: number; html_url: string; default_branch: string}>> {
    return request(`/skills/search-github?q=${encodeURIComponent(query)}`)
  },
  async listGithubSkillFiles(owner: string, repo: string, branch?: string): Promise<Array<{owner: string; repo: string; path: string; branch: string}>> {
    const suffix = branch ? `&branch=${encodeURIComponent(branch)}` : ''
    return request(`/skills/github-files?owner=${encodeURIComponent(owner)}&repo=${encodeURIComponent(repo)}${suffix}`)
  },
  async installSkill(data: { owner: string; repo: string; path: string; branch?: string; root?: string }): Promise<SkillCard> {
    return request<SkillCard>('/skills/install', { method: 'POST', body: JSON.stringify(data) })
  },
  async deleteSkill(id: string): Promise<void> {
    await post(`/skills/${id}/delete`, {})
  },

  // Personas
  async listPersonas(): Promise<PersonaCard[]> { return withFallback(() => request<PersonaCard[]>('/personas'), mockPersonas) },
  async getPersona(id: string): Promise<PersonaCard> { return withFallback(() => request<PersonaCard>(`/personas/${id}`), mockPersonas.find(p => p.id === id) || mockPersonas[0]) },
  async createPersona(data: { name: string; type: string; description: string; instructions: string }): Promise<PersonaCard> {
    return request<PersonaCard>('/personas/create', { method: 'POST', body: JSON.stringify(data) })
  },
  async updatePersona(id: string, data: { name: string; type: string; description: string; instructions: string }): Promise<PersonaCard> {
    return request<PersonaCard>(`/personas/${id}/update`, { method: 'POST', body: JSON.stringify(data) })
  },
  async deletePersona(id: string): Promise<void> {
    await post(`/personas/${id}/delete`, {})
  },

  // Memory
  async listMemories(scope?: string): Promise<MemoryItem[]> {
    const suffix = scope ? `?scope=${encodeURIComponent(scope)}` : ''
    return withFallback(() => request<MemoryItem[]>(`/memory${suffix}`), scope ? mockMemories.filter(m => m.scope === scope) : mockMemories)
  },
  async searchMemories(query: string): Promise<MemoryItem[]> {
    return withFallback(() => request<MemoryItem[]>(`/memory?q=${encodeURIComponent(query)}`), mockMemories.filter(m => m.summary.includes(query) || m.body.includes(query) || m.tags.some(t => t.includes(query))))
  },
  async addMemory(item: Partial<MemoryItem>): Promise<MemoryItem> {
    return request<MemoryItem>('/memory/create', {
      method: 'POST',
      body: JSON.stringify({
        scope: item.scope || 'working',
        type: item.type || 'fuzzy',
        summary: item.summary || '',
        body: item.body || '',
        tags: item.tags || [],
      }),
    })
  },
  async deleteMemory(id: string): Promise<void> {
    await post(`/memory/${id}/delete`, {})
  },

  // Channels
  async listChannels(): Promise<ChannelInfo[]> {
    return withFallback(() => request<ChannelInfo[]>('/channels'), mockChannels)
  },

  // Config — read from API, save via channel command
  async getConfig(): Promise<AppConfig> {
    return withFallback(async (): Promise<AppConfig> => {
      const raw = await request<Record<string, any>>('/config')
      return {
        llm: { provider: raw.llm_provider || '', model: raw.llm_model || '', base_url: raw.llm_base_url || '', api_key: '', timeout: raw.llm_timeout_seconds || 0 },
        embedding: { provider: '', model: '', base_url: '', api_key: '' },
        browser: { provider: '', mcp_url: '', headless: !!raw.browser_headless, timeout: raw.browser_timeout_seconds || 0 },
        storage: {
          workspace_path: raw.workspace_path || '',
          sqlite_path: raw.sqlite_path || '',
          artifact_path: raw.artifact_path || '',
          log_path: raw.log_path || '',
          skill_roots: raw.skill_roots || [],
          persona_path: raw.persona_path || '',
        },
        memory: { max_working_items: 0, max_project_items: 0, default_scope: '', enable_auto_extract: false },
        runtime: { max_tasks_per_project: raw.max_tasks_per_project || 0, max_retries: 0, token_budget: raw.context_max_chars || 0, enable_prompt_cache: false, enable_auto_repair: false, enable_debug_log: false },
        chain_router: { l0_max_tokens: 0, l1_max_tools: 0, l2_max_tasks: 0, auto_plan_threshold: '' },
        security: { require_confirm_high_risk: !!raw.require_confirm_high_risk, allow_shell: !!raw.allow_shell, allow_auto_execute_high_risk: false },
      }
    }, mockConfig)
  },
  async saveConfig(config: AppConfig): Promise<void> {
    await post('/config', config as unknown as Record<string, unknown>)
  },

  // Files
  async listFiles(path?: string): Promise<FileEntry[]> {
    const suffix = path ? `?path=${encodeURIComponent(path)}` : ''
    return withFallback(() => request<FileEntry[]>(`/files${suffix}`), mockFiles)
  },
  async readFile(path: string): Promise<{ path: string; content: string; size: number }> {
    return request<{ path: string; content: string; size: number }>(`/files/content?path=${encodeURIComponent(path)}`)
  },

  // Sessions
  async listSessions(): Promise<Session[]> {
    return withFallback(() => request<Session[]>('/sessions'), mockSessions)
  },
  async getSession(id: string): Promise<Session> {
    return withFallback(() => request<Session>(`/sessions/${id}`), mockSessions.find(s => s.id === id) || mockSessions[0])
  },
  async getSessionContext(id: string): Promise<SessionContext | null> {
    return withFallback(() => request<SessionContext>(`/sessions/${id}/context`), null)
  },
  async pauseSession(id: string): Promise<Session> { return request<Session>(`/sessions/${id}/pause`, { method: 'POST' }) },
  async resumeSession(id: string): Promise<Session> { return request<Session>(`/sessions/${id}/resume`, { method: 'POST' }) },
  async expireSession(id: string): Promise<Session> { return request<Session>(`/sessions/${id}/expire`, { method: 'POST' }) },
  async deleteSession(id: string): Promise<void> { await post(`/sessions/${id}/delete`, {}) },
}
