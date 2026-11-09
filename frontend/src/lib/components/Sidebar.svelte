<script lang="ts">
  import { Ghost, MessageSquare, PanelLeftClose, Plus, Search, Settings } from 'lucide-svelte'
  import type { db } from '../wailsjs/go/models'

  let {
    threads,
    activeId,
    temporaryActive,
    oncreate,
    oncreatetemporary,
    onselect,
    onsettings,
    onsearch,
    onhide
  }: {
    threads: db.Thread[]
    activeId: string | null
    temporaryActive: boolean
    oncreate: () => void
    oncreatetemporary: () => void
    onselect: (id: string) => void
    onsettings: () => void
    onsearch: () => void
    onhide: () => void
  } = $props()
</script>

<aside class="flex w-64 shrink-0 flex-col border-r border-line bg-panel">
  <div class="flex items-center justify-between px-3 py-3">
    <span class="px-1 text-sm font-semibold tracking-wide">Chattu</span>
    <div class="flex items-center gap-0.5">
      <button
        class="rounded-md p-1.5 text-muted transition-colors hover:bg-surface hover:text-fg"
        title="Hide sidebar (⌘B)"
        onclick={onhide}
      >
        <PanelLeftClose size={16} />
      </button>
      <button
        class="rounded-md p-1.5 text-muted transition-colors hover:bg-surface hover:text-fg"
        title="Search chats (⌘K)"
        onclick={onsearch}
      >
        <Search size={16} />
      </button>
      <button
        class="rounded-md p-1.5 transition-colors {temporaryActive
          ? 'bg-surface text-fg'
          : 'text-muted hover:bg-surface hover:text-fg'}"
        title="Temporary chat (⌘⇧N, not saved)"
        aria-pressed={temporaryActive}
        onclick={oncreatetemporary}
      >
        <Ghost size={16} />
      </button>
      <button
        class="rounded-md p-1.5 text-muted transition-colors hover:bg-surface hover:text-fg"
        title="New chat (⌘N)"
        onclick={oncreate}
      >
        <Plus size={16} />
      </button>
    </div>
  </div>

  <nav class="flex-1 overflow-y-auto px-2 pb-2">
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
  </nav>

  <div class="border-t border-line p-2">
    <button
      class="flex w-full items-center gap-2 rounded-md px-2 py-2 text-sm text-muted transition-colors hover:bg-surface hover:text-fg"
      onclick={onsettings}
    >
      <Settings size={14} />
      Settings
    </button>
  </div>
</aside>
