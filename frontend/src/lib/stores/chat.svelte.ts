import type { Thread } from '../types'

class ChatStore {
  threads = $state<Thread[]>([])
  activeId = $state<string | null>(null)

  get activeThread(): Thread | null {
    return this.threads.find((thread) => thread.id === this.activeId) ?? null
  }

  createThread(): Thread {
    const now = Date.now()
    const thread: Thread = {
      id: crypto.randomUUID(),
      title: 'New chat',
      messages: [],
      createdAt: now,
      updatedAt: now
    }
    this.threads = [thread, ...this.threads]
    this.activeId = thread.id
    return thread
  }

  select(id: string): void {
    this.activeId = id
  }
}

export const chat = new ChatStore()
