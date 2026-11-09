export type Role = 'system' | 'user' | 'assistant'

export interface Message {
  id: string
  role: Role
  content: string
  createdAt: number
}

export interface Thread {
  id: string
  title: string
  messages: Message[]
  createdAt: number
  updatedAt: number
}
