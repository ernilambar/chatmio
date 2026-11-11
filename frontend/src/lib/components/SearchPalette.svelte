<script lang="ts">
  import { Search, X } from 'lucide-svelte'
  import type { db } from '../wailsjs/go/models'
  import { SearchThreads } from '../wailsjs/go/main/App'
  import { trapFocus } from '../focus'

  let {
    open,
    threads,
    onselect,
    onclose
  }: {
    open: boolean
    threads: db.Thread[]
    onselect: (id: string) => void
    onclose: () => void
  } = $props()

  interface Item {
    id: string
    title: string
    snippet: string
  }

  let query = $state('')
  let results = $state<db.SearchResult[]>([])
  let active = $state(0)
  let inputEl: HTMLInputElement | undefined = $state()
  let requestId = 0

  const items = $derived<Item[]>(
    query.trim()
      ? results.map((result) => ({
          id: result.threadId,
          title: result.title,
          snippet: result.snippet
        }))
      : threads.slice(0, 8).map((thread) => ({
          id: thread.id,
          title: thread.title,
          snippet: ''
        }))
  )

  $effect(() => {
    if (!open) return
    query = ''
    results = []
    active = 0
    requestId += 1
    queueMicrotask(() => inputEl?.focus())
  })

  $effect(() => {
    if (!open) return
    const value = query.trim()
    if (!value) {
      results = []
      return
    }

    const id = ++requestId
    const timer = setTimeout(async () => {
      try {
        const next = await SearchThreads(value)
        if (id !== requestId) return
        results = next
        active = 0
      } catch {
        if (id === requestId) results = []
      }
    }, 120)
    return () => clearTimeout(timer)
  })

  function choose(item: Item | undefined): void {
    if (!item) return
    onclose()
    onselect(item.id)
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      active = Math.min(active + 1, items.length - 1)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      active = Math.max(active - 1, 0)
    } else if (event.key === 'Enter') {
      event.preventDefault()
      choose(items[active])
    } else if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      onclose()
    }
  }
</script>

{#if open}
  <div class="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[12vh]">
    <button
      type="button"
      class="absolute inset-0 bg-black/40"
      aria-label="Close search"
      onclick={onclose}
    ></button>
    <div
      use:trapFocus
      class="relative w-full max-w-lg overflow-hidden rounded-lg border border-line bg-surface shadow-xl"
      role="dialog"
      aria-modal="true"
      aria-label="Search chats"
    >
      <div class="flex items-center gap-2 border-b border-line px-3 py-2">
        <Search size={15} class="shrink-0 text-muted" />
        <input
          bind:this={inputEl}
          bind:value={query}
          onkeydown={handleKeydown}
          placeholder="Search chats and messages…"
          aria-label="Search chats and messages"
          class="flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
        />
        <button
          type="button"
          class="rounded p-0.5 text-muted transition-colors hover:text-fg"
          title="Close search"
          onclick={onclose}
        >
          <X size={14} />
        </button>
      </div>

      <div class="max-h-80 overflow-y-auto p-1">
        {#if items.length === 0}
          <p class="px-3 py-6 text-center text-xs text-muted">
            {query.trim() ? 'No matching chats.' : 'No chats yet.'}
          </p>
        {:else}
          {#each items as item, index (item.id)}
            <button
              type="button"
              class="flex w-full flex-col gap-0.5 rounded-md px-3 py-2 text-left {index === active
                ? 'bg-panel'
                : ''}"
              onclick={() => choose(item)}
              onmouseenter={() => (active = index)}
            >
              <span class="truncate text-sm">{item.title}</span>
              {#if item.snippet}
                <span class="line-clamp-2 text-xs leading-snug text-muted">{item.snippet}</span>
              {/if}
            </button>
          {/each}
        {/if}
      </div>

      <div class="border-t border-line px-3 py-1.5 text-[11px] text-muted">
        ↑↓ navigate · ↵ open · esc close
      </div>
    </div>
  </div>
{/if}
