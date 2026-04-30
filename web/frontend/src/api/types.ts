// Types matching Go domain models

export type ProjectStatus = 'ACTIVE' | 'COMPLETED' | 'ARCHIVED'
export type TaskStatus = 'DRAFT' | 'READY' | 'IN_PROGRESS' | 'IMPLEMENTED' | 'TESTING' | 'REVIEWING' | 'DONE' | 'BLOCKED' | 'NEED_USER_INPUT' | 'REVIEW_FAILED' | 'FAILED' | 'CANCELLED'
export type AttemptStatus = 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'CANCELLED'
export type ToolCallStatus = 'PENDING' | 'SUCCEEDED' | 'FAILED'
export type TestStatus = 'PASSED' | 'FAILED'
export type ReviewStatus = 'APPROVED' | 'REJECTED'

export interface Project {
  id: string
  name: string
  goal: string
  status: ProjectStatus
  created_at: string
  updated_at: string
}

export interface Task {
  id: string
  project_id: string
  title: string
  description: string
  status: TaskStatus
  acceptance_criteria: string[]
  depends_on: string[]
  created_at: string
  updated_at: string
}

export interface TaskDependency {
  id: string
  task_id: string
  depends_on_task_id: string
  created_at: string
}

export interface TaskAttempt {
  id: string
  task_id: string
  number: number
  status: AttemptStatus
  started_at: string
  ended_at: string | null
  error: string
}

export interface TaskEvent {
  id: string
  task_id: string
  attempt_id: string
  type: string
  from_state: string
  to_state: string
  message: string
  payload: string
  created_at: string
}

export interface ToolCall {
  id: string
  attempt_id: string
  name: string
  input_json: string
  output_json: string
  status: ToolCallStatus
  error: string
  evidence_ref: string
  created_at: string
  updated_at: string
}

export interface Observation {
  id: string
  attempt_id: string
  tool_call_id: string
  type: string
  summary: string
  evidence_ref: string
  payload: string
  created_at: string
}

export interface TestResult {
  id: string
  attempt_id: string
  name: string
  status: TestStatus
  output: string
  evidence_ref: string
  created_at: string
}

export interface ReviewResult {
  id: string
  attempt_id: string
  status: ReviewStatus
  summary: string
  evidence_ref: string
  created_at: string
}

export interface Artifact {
  id: string
  attempt_id: string
  project_id: string
  type: string
  path: string
  description: string
  created_at: string
}

export interface ChatMessage {
  id: string
  session_id: string
  project_id: string
  role: 'user' | 'assistant' | 'tool' | 'system'
  content: string
  artifacts: string[]
  metadata: Record<string, any>
  created_at: string
}

export interface SkillCard {
  id: string
  name: string
  description: string
  allowed_tools: string[]
  path: string
  version_hash: string
  frontmatter: Record<string, string>
  body: string
}

export interface PersonaCard {
  id: string
  name: string
  type: 'main' | 'channel' | 'project' | 'role' | 'ephemeral'
  description: string
  path: string
  version_hash: string
  style_rules: string[]
  boundaries: string[]
  instructions: string
}

export interface MemoryItem {
  id: string
  scope: 'working' | 'project' | 'long_term'
  type: 'exact' | 'fuzzy'
  tags: string[]
  summary: string
  body: string
  confidence: number
  artifact_ref: string
  source_task_id: string
  version_hash: string
}

export interface ChannelInfo {
  id: string
  type: string
  name: string
  enabled: boolean
  capabilities: {
    supported_message_types: string[]
    supported_interactions: string[]
    supports_confirmation: boolean
    supports_streaming: boolean
    supports_file_request: boolean
  }
}

export interface AppConfig {
  llm: {
    provider: string
    model: string
    base_url: string
    api_key: string
    timeout: number
  }
  embedding: {
    provider: string
    model: string
    base_url: string
    api_key: string
  }
  browser: {
    provider: string
    mcp_url: string
    headless: boolean
    timeout: number
  }
  storage: {
    workspace_path: string
    sqlite_path: string
    artifact_path: string
    log_path: string
    skill_roots: string[]
    persona_path: string
  }
  memory: {
    max_working_items: number
    max_project_items: number
    default_scope: string
    enable_auto_extract: boolean
  }
  runtime: {
    max_tasks_per_project: number
    max_retries: number
    token_budget: number
    enable_prompt_cache: boolean
    enable_auto_repair: boolean
    enable_debug_log: boolean
  }
  chain_router: {
    l0_max_tokens: number
    l1_max_tools: number
    l2_max_tasks: number
    auto_plan_threshold: string
  }
  security: {
    require_confirm_high_risk: boolean
    allow_shell: boolean
    allow_auto_execute_high_risk: boolean
  }
}

export interface DashboardStats {
  project_count: number
  task_count: number
  done_count: number
  running_count: number
  failed_count: number
}

export interface ProviderStatus {
  name: string
  type: string
  status: 'connected' | 'disconnected' | 'error'
  detail: string
}

export interface ChainDecision {
  level: 'L0' | 'L1' | 'L2' | 'L3'
  reason: string
  need_plan: boolean
  need_tools: boolean
  need_memory: boolean
  need_review: boolean
  need_browser: boolean
  risk_level: string
}

export interface FileEntry {
  name: string
  path: string
  type: 'file' | 'dir'
  size: number
  modified: string
}

export type SessionStatus = 'ACTIVE' | 'PAUSED' | 'COMPLETED' | 'EXPIRED'

export interface Session {
  id: string
  user_id: string
  channel_type: string
  channel_id: string
  project_id: string
  status: SessionStatus
  title: string
  last_active_at: string
  created_at: string
  updated_at: string
}

export interface SessionContext {
  session_id: string
  project_id: string
  chain_decision: string
  intent_profile: string
  context_text: string
  memory_snapshot: string
  updated_at: string
}
