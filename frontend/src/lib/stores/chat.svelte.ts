import {
  CancelReply,
  CreateThread,
  DeleteThread,
  EditMessage,
  GetConfig,
  GetMessages,
  ListModels,
  ListThreads,
  RegenerateReply,
  RenameThread,
  SaveConfig,
  SendMessage,
  SetThreadModel,
  SetThreadSettings
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { config, db } from '../wailsjs/go/models'

interface ChatDelta {
  threadId: string
  delta: string
  reasoning: string
}

interface ChatDone {
  threadId: string
  message: db.Message
}

interface ChatTitle {
  threadId: string
  title: string
}

const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024
const MAX_TOTAL_ATTACHMENT_BYTES = 20 * 1024 * 1024
const MAX_ATTACHMENTS = 10

async function readAttachment(file: File): Promise<db.AttachmentInput> {
  // WKWebView on the oldest supported macOS lacks Promise.withResolvers.
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error(`Could not read ${file.name}`))
    reader.readAsDataURL(file)
  })
  const comma = dataUrl.indexOf(',')
  if (comma < 0) throw new Error(`Could not read ${file.name}`)
  return db.AttachmentInput.createFrom({
    name: file.name,
    mimeType: file.type || 'application/octet-stream',
    data: dataUrl.slice(comma + 1)
  })
}

async function readAttachments(files: File[]): Promise<db.AttachmentInput[]> {
  if (files.length === 0) return []
  if (files.length > MAX_ATTACHMENTS) {
    throw new Error(`Attach at most ${MAX_ATTACHMENTS} files.`)
  }
  let total = 0
  for (const file of files) {
    if (file.size === 0) {
      throw new Error(`${file.name} is empty.`)
    }
    if (file.size > MAX_ATTACHMENT_BYTES) {
      throw new Error(`${file.name} is larger than 10 MB.`)
    }
    total += file.size
  }
  if (total > MAX_TOTAL_ATTACHMENT_BYTES) {
    throw new Error('Attachments exceed 20 MB total.')
  }
  return Promise.all(files.map(readAttachment))
}

class ChatStore {
  config = $state<config.Config | null>(null)
  threads = $state<db.Thread[]>([])
  messages = $state<db.Message[]>([])
  models = $state<string[]>([])
  activeId = $state<string | null>(null)
  streaming = $state(false)
  streamText = $state('')
  streamReasoning = $state('')
  error = $state<string | null>(null)

  private unsubscribers: Array<() => void> = []
  private failedSend = $state<{ content: string; files: File[] } | null>(null)

  get activeThread(): db.Thread | null {
    return this.threads.find((thread) => thread.id === this.activeId) ?? null
  }

  get canRetry(): boolean {
    if (this.streaming) return false
    const last = this.messages[this.messages.length - 1]
    return last?.role === 'user' || this.failedSend !== null
  }

  get activeModel(): string {
    return this.activeThread?.model || this.config?.chatModel || ''
  }

  // contextWindow is the chat's own window, else the global default; 0 means
  // no warning.
  get contextWindow(): number {
    return this.activeThread?.contextWindow || this.config?.contextWindow || 0
  }

  // contextUsed is the token count of the latest reply's full exchange, which
  // approximates how much of the window the chat occupies.
  get contextUsed(): number {
    for (let i = this.messages.length - 1; i >= 0; i--) {
      const message = this.messages[i]
      if (message.role === 'assistant' && message.promptTokens > 0) {
        return message.promptTokens + message.completionTokens
      }
    }
    return 0
  }

  async init(): Promise<void> {
    this.config = await GetConfig()
    this.listen()
    this.threads = await ListThreads()
    // Open on a fresh chat; reuse the latest thread if it is still empty so
    // launches don't accumulate blank threads.
    const first = this.threads[0]
    if (first && (await GetMessages(first.id)).length === 0) {
      await this.select(first.id)
    } else {
      await this.createThread()
    }
    void this.loadModels()
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
        this.streamReasoning += payload.reasoning
      }),
      EventsOn('chat:done', async (payload: ChatDone) => {
        if (payload.threadId !== this.activeId) return
        this.streamText = ''
        this.streamReasoning = ''
        this.messages = await GetMessages(payload.threadId)
        this.threads = await ListThreads()
      }),
      EventsOn('chat:title', async (payload: ChatTitle) => {
        this.threads = this.threads.map((thread) =>
          thread.id === payload.threadId
            ? db.Thread.createFrom({ ...thread, title: payload.title })
            : thread
        )
        // A reply refresh may still be in flight with the old title; reconcile
        // once more now that the title is committed.
        this.threads = await ListThreads()
      })
    )
  }

  async loadModels(): Promise<void> {
    const cfg = this.config
    if (!cfg || !cfg.baseUrl.trim()) {
      this.models = []
      return
    }
    try {
      this.models = await ListModels(cfg.baseUrl, cfg.apiKey)
    } catch {
      this.models = []
    }
  }

  async createThread(): Promise<void> {
    if (this.streaming) return
    const thread = await CreateThread()
    this.threads = [thread, ...this.threads]
    this.activeId = thread.id
    this.messages = []
    this.error = null
    this.failedSend = null
  }

  async select(id: string): Promise<void> {
    if (this.streaming || id === this.activeId) return
    this.activeId = id
    this.error = null
    this.streamText = ''
    this.streamReasoning = ''
    this.failedSend = null
    this.messages = await GetMessages(id)
  }

  async send(content: string, files: File[] = []): Promise<void> {
    content = content.trim()
    if ((!content && files.length === 0) || this.streaming || !this.activeId) return

    const threadId = this.activeId
    this.streaming = true
    this.error = null
    this.streamText = ''
    this.streamReasoning = ''
    this.failedSend = null

    let attachments: db.AttachmentInput[]
    try {
      attachments = await readAttachments(files)
    } catch (err) {
      this.streaming = false
      this.error = err instanceof Error ? err.message : String(err)
      return
    }

    const optimisticAttachments = attachments.map((attachment) =>
      db.Attachment.createFrom({
        id: crypto.randomUUID(),
        messageId: '',
        name: attachment.name,
        mimeType: attachment.mimeType,
        size: Math.floor((attachment.data.length * 3) / 4),
        dataUrl: attachment.mimeType.startsWith('image/')
          ? `data:${attachment.mimeType};base64,${attachment.data}`
          : undefined,
        createdAt: Date.now()
      })
    )

    this.messages = [
      ...this.messages,
      db.Message.createFrom({
        id: crypto.randomUUID(),
        threadId,
        role: 'user',
        content,
        attachments: optimisticAttachments,
        createdAt: Date.now()
      })
    ]

    try {
      await SendMessage(threadId, content, attachments)
      this.failedSend = null
      this.messages = await GetMessages(threadId)
      this.threads = await ListThreads()
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
      this.failedSend = { content, files }
      this.streamText = ''
      this.streamReasoning = ''
      this.messages = await GetMessages(threadId)
    } finally {
      this.streaming = false
    }
  }

  // cancel stops the in-flight reply. The partial reply is kept by the backend.
  async cancel(): Promise<void> {
    if (!this.streaming) return
    try {
      await CancelReply()
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
    }
  }

  // retry re-runs the failed turn. When the user message was already stored,
  // only the assistant reply is missing, so regenerate covers it; otherwise the
  // original prompt and attachments are sent again.
  async retry(): Promise<void> {
    if (this.streaming || !this.activeId) return
    const last = this.messages[this.messages.length - 1]
    if (last?.role === 'user') {
      await this.regenerate()
      return
    }
    const failed = this.failedSend
    if (!failed) return
    this.failedSend = null
    await this.send(failed.content, failed.files)
  }

  // regenerate drops the trailing assistant reply and asks for a new one.
  async regenerate(): Promise<void> {
    if (this.streaming || !this.activeId) return

    const threadId = this.activeId
    this.streaming = true
    this.error = null
    this.streamText = ''
    this.streamReasoning = ''

    const messages = [...this.messages]
    while (messages.length > 0 && messages[messages.length - 1].role === 'assistant') {
      messages.pop()
    }
    this.messages = messages

    try {
      await RegenerateReply(threadId)
      this.failedSend = null
      this.messages = await GetMessages(threadId)
      this.threads = await ListThreads()
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
      this.streamText = ''
      this.streamReasoning = ''
      this.messages = await GetMessages(threadId)
    } finally {
      this.streaming = false
    }
  }

  // edit replaces a user message, discards everything that followed it and
  // streams a fresh reply. Attachments are kept by the backend.
  async edit(messageId: string, content: string): Promise<void> {
    content = content.trim()
    if (!content || this.streaming || !this.activeId) return

    const threadId = this.activeId
    this.streaming = true
    this.error = null
    this.streamText = ''
    this.streamReasoning = ''

    const index = this.messages.findIndex((message) => message.id === messageId)
    if (index >= 0) {
      const next = this.messages.slice(0, index + 1)
      next[index] = db.Message.createFrom({ ...next[index], content })
      this.messages = next
    }

    try {
      await EditMessage(threadId, messageId, content)
      this.failedSend = null
      this.messages = await GetMessages(threadId)
      this.threads = await ListThreads()
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
      this.streamText = ''
      this.streamReasoning = ''
      this.messages = await GetMessages(threadId)
    } finally {
      this.streaming = false
    }
  }

  async saveConfig(next: config.Config): Promise<void> {
    await SaveConfig(next)
    this.config = await GetConfig()
    void this.loadModels()
  }

  async setThreadModel(model: string): Promise<void> {
    if (!this.activeId || this.streaming) return
    const threadId = this.activeId
    try {
      await SetThreadModel(threadId, model)
      this.threads = this.threads.map((thread) =>
        thread.id === threadId ? db.Thread.createFrom({ ...thread, model }) : thread
      )
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
    }
  }

  async setThreadSettings(settings: db.ThreadSettings): Promise<void> {
    if (!this.activeId) return
    const threadId = this.activeId
    await SetThreadSettings(threadId, settings)
    this.threads = this.threads.map((thread) =>
      thread.id === threadId ? db.Thread.createFrom({ ...thread, ...settings }) : thread
    )
  }

  async renameThread(threadId: string, title: string): Promise<void> {
    title = title.trim()
    if (!title) return
    try {
      await RenameThread(threadId, title)
      this.threads = this.threads.map((thread) =>
        thread.id === threadId ? db.Thread.createFrom({ ...thread, title }) : thread
      )
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
    }
  }

  async deleteThread(threadId: string): Promise<void> {
    if (this.streaming) return
    try {
      await DeleteThread(threadId)
      const wasActive = this.activeId === threadId
      this.threads = this.threads.filter((thread) => thread.id !== threadId)
      if (!wasActive) return

      this.activeId = null
      this.messages = []
      this.streamText = ''
      this.streamReasoning = ''
      const next = this.threads[0]
      if (next) {
        await this.select(next.id)
      } else {
        await this.createThread()
      }
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
    }
  }
}

export const chat = new ChatStore()
