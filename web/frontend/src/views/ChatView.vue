<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { api } from '../api'
import type { ChatMessage, ChainDecision } from '../api/types'

const messages = ref<ChatMessage[]>([])
const input = ref('')
const sessionId = ref('sess-1')
const chainDecision = ref<ChainDecision | null>(null)
const sending = ref(false)
const messagesEl = ref<HTMLElement>()

onMounted(async () => {
  messages.value = await api.listChatMessages(sessionId.value)
  chainDecision.value = await api.getChainDecision(sessionId.value)
  await nextTick()
  scrollToBottom()
})

function scrollToBottom() {
  if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
}

async function send() {
  if (!input.value.trim() || sending.value) return
  sending.value = true
  const content = input.value
  input.value = ''

  messages.value.push({
    id: `msg-${Date.now()}`, session_id: sessionId.value, project_id: '', role: 'user',
    content, artifacts: [], metadata: {}, created_at: new Date().toISOString(),
  })
  await nextTick()
  scrollToBottom()

  const reply = await api.sendChatMessage(sessionId.value, content)
  messages.value.push(reply)
  await nextTick()
  scrollToBottom()
  sending.value = false
}

const roleColor: Record<string, string> = {
  user: 'text-indigo-300',
  assistant: 'text-gray-100',
  tool: 'text-yellow-300',
  system: 'text-gray-500',
}
const roleLabel: Record<string, string> = { user: 'You', assistant: 'Agent', tool: 'Tool', system: 'System' }
</script>

<template>
  <div class="flex h-[calc(100vh-8rem)] flex-col">
    <!-- Chain Decision Banner -->
    <div v-if="chainDecision" class="mb-3 flex items-center gap-4 rounded-lg border border-gray-800 bg-gray-900 px-4 py-2 text-xs">
      <span class="rounded bg-indigo-900/50 px-2 py-0.5 font-mono text-indigo-300">{{ chainDecision.level }}</span>
      <span class="text-gray-400">{{ chainDecision.reason }}</span>
      <div class="ml-auto flex gap-2 text-gray-500">
        <span v-if="chainDecision.need_plan">Plan</span>
        <span v-if="chainDecision.need_tools">Tools</span>
        <span v-if="chainDecision.need_memory">Memory</span>
        <span v-if="chainDecision.need_review">Review</span>
        <span v-if="chainDecision.need_browser">Browser</span>
      </div>
    </div>

    <!-- Messages -->
    <div ref="messagesEl" class="flex-1 overflow-y-auto space-y-4 mb-4">
      <div
        v-for="msg in messages"
        :key="msg.id"
        :class="['rounded-lg border border-gray-800 bg-gray-900 p-4', msg.role === 'user' ? 'border-indigo-900/50' : '']"
      >
        <div class="flex items-center gap-2 mb-1">
          <span :class="['text-xs font-medium', roleColor[msg.role]]">{{ roleLabel[msg.role] }}</span>
          <span class="text-xs text-gray-600">{{ new Date(msg.created_at).toLocaleTimeString() }}</span>
          <span v-if="msg.metadata?.tool" class="rounded bg-gray-800 px-1.5 py-0.5 text-xs text-gray-400">{{ msg.metadata.tool }}</span>
          <span v-if="msg.metadata?.chain_level" class="rounded bg-indigo-900/50 px-1.5 py-0.5 text-xs text-indigo-300">{{ msg.metadata.chain_level }}</span>
        </div>
        <div :class="['text-sm whitespace-pre-wrap', roleColor[msg.role]]">{{ msg.content }}</div>
        <div v-if="msg.artifacts?.length" class="mt-2 flex gap-2">
          <span v-for="a in msg.artifacts" :key="a" class="rounded bg-gray-800 px-2 py-0.5 text-xs text-gray-400">📎 {{ a }}</span>
        </div>
      </div>
      <div v-if="sending" class="text-sm text-gray-500 animate-pulse">Agent is thinking...</div>
    </div>

    <!-- Input -->
    <div class="flex gap-2">
      <input
        v-model="input"
        @keydown.enter="send"
        placeholder="Type a message..."
        class="flex-1 rounded-lg border border-gray-700 bg-gray-900 px-4 py-3 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
      />
      <button
        @click="send"
        :disabled="sending || !input.trim()"
        class="rounded-lg bg-indigo-600 px-6 py-3 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Send
      </button>
    </div>
  </div>
</template>
