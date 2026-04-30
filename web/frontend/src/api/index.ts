// API client - currently returns mock data
// Replace with real fetch calls when backend API is ready

import type {
  Project, Task, TaskAttempt, TaskEvent, ToolCall, Observation,
  TestResult, ReviewResult, Artifact, ChatMessage, SkillCard,
  PersonaCard, MemoryItem, ChannelInfo, AppConfig, DashboardStats,
  ProviderStatus, ChainDecision, FileEntry
} from './types'

import {
  mockProjects, mockDashboardStats, mockProviders, mockTasks,
  getTasksForProject, getAttemptsForTask, getToolCallsForAttempt,
  mockObservations, getEventsForTask, mockTestResults, mockReviewResults,
  mockArtifacts, mockChatMessages, mockSkills, mockPersonas,
  mockMemories, mockChannels, mockConfig, mockChainDecision, mockFiles,
} from './mock-data'

const delay = (ms: number) => new Promise(r => setTimeout(r, ms))

export const api = {
  // Dashboard
  async getDashboardStats(): Promise<DashboardStats> { await delay(100); return mockDashboardStats },
  async getProviders(): Promise<ProviderStatus[]> { await delay(100); return mockProviders },
  async getRecentProjects(): Promise<Project[]> { await delay(100); return mockProjects.slice(0, 3) },

  // Projects
  async listProjects(): Promise<Project[]> { await delay(150); return mockProjects },
  async getProject(id: string): Promise<Project> { await delay(100); return mockProjects.find(p => p.id === id) || mockProjects[0] },
  async createProject(name: string, goal: string): Promise<Project> {
    await delay(200)
    return { id: `proj-${Date.now()}`, name, goal, status: 'ACTIVE', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
  },

  // Tasks
  async listTasks(projectId: string): Promise<Task[]> { await delay(150); return getTasksForProject(projectId) },
  async getTask(id: string): Promise<Task> {
    await delay(100)
    for (const tasks of Object.values(mockTasks)) {
      const t = tasks.find(t => t.id === id)
      if (t) return t
    }
    return getTasksForProject('proj-1')[0]
  },
  async transitionTask(taskId: string, status: string): Promise<Task> {
    await delay(200)
    const task = await this.getTask(taskId)
    return { ...task, status: status as Task['status'] }
  },

  // Attempts
  async listAttempts(taskId: string): Promise<TaskAttempt[]> { await delay(100); return getAttemptsForTask(taskId) },

  // Events
  async listEvents(taskId: string): Promise<TaskEvent[]> { await delay(100); return getEventsForTask(taskId) },

  // Tool calls
  async listToolCalls(attemptId: string): Promise<ToolCall[]> { await delay(100); return getToolCallsForAttempt(attemptId) },

  // Observations
  async listObservations(attemptId: string): Promise<Observation[]> { await delay(100); return mockObservations[attemptId] || [] },

  // Test results
  async listTestResults(attemptId: string): Promise<TestResult[]> { await delay(100); return mockTestResults[attemptId] || [] },

  // Review results
  async listReviewResults(attemptId: string): Promise<ReviewResult[]> { await delay(100); return mockReviewResults[attemptId] || [] },

  // Artifacts
  async listArtifacts(projectId: string): Promise<Artifact[]> { await delay(100); return mockArtifacts[projectId] || [] },

  // Chat
  async listChatMessages(sessionId: string): Promise<ChatMessage[]> { await delay(100); return mockChatMessages },
  async sendChatMessage(sessionId: string, content: string): Promise<ChatMessage> {
    await delay(300)
    return {
      id: `msg-${Date.now()}`, session_id: sessionId, project_id: '', role: 'assistant',
      content: `收到消息: "${content}"\n\n正在处理中...`, artifacts: [], metadata: {}, created_at: new Date().toISOString(),
    }
  },
  async getChainDecision(sessionId: string): Promise<ChainDecision> { await delay(100); return mockChainDecision },

  // Skills
  async listSkills(): Promise<SkillCard[]> { await delay(100); return mockSkills },
  async getSkill(id: string): Promise<SkillCard> { await delay(100); return mockSkills.find(s => s.id === id) || mockSkills[0] },
  async searchSkills(query: string): Promise<SkillCard[]> { await delay(150); return mockSkills.filter(s => s.name.includes(query) || s.description.includes(query)) },

  // Personas
  async listPersonas(): Promise<PersonaCard[]> { await delay(100); return mockPersonas },
  async getPersona(id: string): Promise<PersonaCard> { await delay(100); return mockPersonas.find(p => p.id === id) || mockPersonas[0] },

  // Memory
  async listMemories(scope?: string): Promise<MemoryItem[]> { await delay(100); return scope ? mockMemories.filter(m => m.scope === scope) : mockMemories },
  async searchMemories(query: string): Promise<MemoryItem[]> { await delay(150); return mockMemories.filter(m => m.summary.includes(query) || m.body.includes(query) || m.tags.some(t => t.includes(query))) },
  async addMemory(item: Partial<MemoryItem>): Promise<MemoryItem> {
    await delay(200)
    return { id: `mem-${Date.now()}`, scope: item.scope || 'working', type: item.type || 'fuzzy', tags: item.tags || [], summary: item.summary || '', body: item.body || '', confidence: item.confidence || 0.8, artifact_ref: '', source_task_id: '', version_hash: `m${Date.now()}` }
  },
  async deleteMemory(id: string): Promise<void> { await delay(100) },

  // Channels
  async listChannels(): Promise<ChannelInfo[]> { await delay(100); return mockChannels },

  // Config
  async getConfig(): Promise<AppConfig> { await delay(100); return mockConfig },
  async saveConfig(config: AppConfig): Promise<AppConfig> { await delay(300); return config },

  // Files
  async listFiles(path?: string): Promise<FileEntry[]> { await delay(100); return mockFiles },
}
