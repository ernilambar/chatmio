import {
  CreateThread,
  GetConfig,
  GetMessages,
  ListThreads,
  SaveConfig,
  SendMessage
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { config, db } from '../wailsjs/go/models'

interface ChatDelta {
  threadId: string
  delta: string
}

interface ChatDone {
  threadId: string
  message: db.Message
}

class ChatStore {
  config = $state<config.Config | null>(null)
  threads = $state<db.Thread[]>([])
  messages = $state<db.Message[]>([])
  activeId = $state<string | null>(null)
  streaming = $state(false)
  streamText = $state('')
  error = $state<string | null>(null)

  private unsubscribers: Array<() => void> = []

  get activeThread(): db.Thread | null {
    return this.threads.find((thread) => thread.id === this.activeId) ?? null
  }

  async init(): Promise<void> {
    this.config = await GetConfig()
    this.listen()
    this.threads = await ListThreads()
    const first = this.threads[0]
    if (first) {
      await this.select(first.id)
    } else {
      await this.createThread()
    }
  }

  destroy(): void {
    for (const unsubscribe of this.unsubscribers) unsubscribe()
    this.unsubscribers = []
  }

  private listen(): void {
    this.unsubscribers.push(
      EventsOn('chat:delta', (payload: ChatDelta) => {
        if (payload.threadId !== this.activeId) return
        this.streamText += payload.delta
      }),
      EventsOn('chat:done', async (payload: ChatDone) => {
        if (payload.threadId !== this.activeId) return
        this.streamText = ''
        this.messages = await GetMessages(payload.threadId)
        this.threads = await ListThreads()
      })
    )
  }

  async createThread(): Promise<void> {
    if (this.streaming) return
    const thread = await CreateThread()
    this.threads = [thread, ...this.threads]
    this.activeId = thread.id
    this.messages = []
    this.error = null
  }

  async select(id: string): Promise<void> {
    if (this.streaming || id === this.activeId) return
    this.activeId = id
    this.error = null
    this.streamText = ''
    this.messages = await GetMessages(id)
  }

  async send(content: string): Promise<void> {
    content = content.trim()
    if (!content || this.streaming || !this.activeId) return

    const threadId = this.activeId
    this.streaming = true
    this.error = null
    this.streamText = ''
    this.messages = [
      ...this.messages,
      db.Message.createFrom({
        id: crypto.randomUUID(),
        threadId,
        role: 'user',
        content,
        createdAt: Date.now()
      })
    ]

    try {
      await SendMessage(threadId, content)
      this.messages = await GetMessages(threadId)
      this.threads = await ListThreads()
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
      this.streamText = ''
      this.messages = await GetMessages(threadId)
    } finally {
      this.streaming = false
    }
  }

  async saveConfig(next: config.Config): Promise<void> {
    await SaveConfig(next)
    this.config = await GetConfig()
  }
}

export const chat = new ChatStore()
