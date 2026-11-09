<script lang="ts">
  import { MessageSquare, Plus } from 'lucide-svelte'
  import type { Thread } from '../types'

  let {
    threads,
    activeId,
    oncreate,
    onselect
  }: {
    threads: Thread[]
    activeId: string | null
    oncreate: () => void
    onselect: (id: string) => void
  } = $props()
</script>

<aside class="flex w-64 shrink-0 flex-col border-r border-line bg-panel">
  <div class="flex items-center justify-between px-3 py-3">
    <span class="px-1 text-sm font-semibold tracking-wide">Chattu</span>
    <button
      class="rounded-md p-1.5 text-muted transition-colors hover:bg-surface hover:text-fg"
      title="New chat"
      onclick={oncreate}
    >
      <Plus size={16} />
    </button>
  </div>

  <nav class="flex-1 overflow-y-auto px-2 pb-2">
    {#if threads.length === 0}
      <p class="px-2 py-4 text-xs text-muted">No chats yet.</p>
    {:else}
      {#each threads as thread (thread.id)}
        <button
          class="mb-0.5 flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors {thread.id === activeId
            ? 'bg-surface text-fg'
            : 'text-muted hover:bg-surface hover:text-fg'}"
          onclick={() => onselect(thread.id)}
        >
          <MessageSquare size={14} class="shrink-0" />
          <span class="truncate">{thread.title}</span>
        </button>
      {/each}
    {/if}
  </nav>
</aside>
