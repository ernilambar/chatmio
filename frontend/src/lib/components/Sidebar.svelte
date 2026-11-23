<script lang="ts">
  import { Ghost, MessageSquare, PanelLeftClose, Pencil, Plus, Search, Settings, Trash2 } from 'lucide-svelte'
  import type { db } from '../wailsjs/go/models'

  let {
    threads,
    activeId,
    temporaryActive,
    canCreate,
    newChatOpen,
    newTemporaryOpen,
    oncreate,
    oncreatetemporary,
    onselect,
    onsettings,
    onsearch,
    onhide,
    ondelete,
    onrename
  }: {
    threads: db.Thread[]
    activeId: string | null
    temporaryActive: boolean
    canCreate: boolean
    newChatOpen: boolean
    newTemporaryOpen: boolean
    oncreate: () => void
    oncreatetemporary: () => void
    onselect: (id: string) => void
    onsettings: () => void
    onsearch: () => void
    onhide: () => void
    ondelete: (id: string) => Promise<void> | void
    onrename: (id: string, title: string) => Promise<void> | void
  } = $props()

  let menu = $state<{ threadId: string; x: number; y: number } | null>(null)
  let menuEl: HTMLDivElement | undefined = $state()
  let renamingId = $state<string | null>(null)
  let renameDraft = $state('')
  let renameInput: HTMLInputElement | undefined = $state()
  const menuThread = $derived(
    menu ? threads.find((thread) => thread.id === menu?.threadId) : undefined
  )

  function openMenu(event: MouseEvent, thread: db.Thread): void {
    event.preventDefault()
    event.stopPropagation()
    const anchor = (event.currentTarget as HTMLElement | null)?.getBoundingClientRect()
    menu = {
      threadId: thread.id,
      x: Math.max(4, Math.min(event.clientX || anchor?.left || 4, window.innerWidth - 172)),
      y: Math.max(4, Math.min(event.clientY || anchor?.bottom || 4, window.innerHeight - 52))
    }
  }

  function closeMenu(): void {
    menu = null
  }

  function deleteFromMenu(): void {
    if (!menu) return
    const id = menu.threadId
    closeMenu()
    void ondelete(id)
  }

  function renameFromMenu(): void {
    const thread = menuThread
    if (!thread) return
    closeMenu()
    renamingId = thread.id
    renameDraft = thread.title
    queueMicrotask(() => {
      renameInput?.focus()
      renameInput?.select()
    })
  }

  async function commitRename(): Promise<void> {
    const id = renamingId
    if (!id) return
    renamingId = null
    const title = renameDraft.trim()
    const thread = threads.find((candidate) => candidate.id === id)
    if (title && thread && title !== thread.title) await onrename(id, title)
  }

  function handleRenameKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      event.preventDefault()
      void commitRename()
    } else if (event.key === 'Escape') {
      event.preventDefault()
      renamingId = null
    }
  }

  $effect(() => {
    if (!menu) return
    queueMicrotask(() => menuEl?.focus())
    const onPointerDown = (event: PointerEvent) => {
      if (menuEl && event.target instanceof Node && menuEl.contains(event.target)) return
      closeMenu()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeMenu()
    }
    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('resize', closeMenu)
    window.addEventListener('scroll', closeMenu, true)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('resize', closeMenu)
      window.removeEventListener('scroll', closeMenu, true)
    }
  })

  $effect(() => {
    if (menu && !threads.some((thread) => thread.id === menu?.threadId)) closeMenu()
  })

  $effect(() => {
    if (renamingId && !threads.some((thread) => thread.id === renamingId)) renamingId = null
  })
</script>

<aside class="flex w-64 shrink-0 flex-col border-r border-line bg-panel">
  <div class="flex items-center justify-between px-3 py-3">
    <span class="px-1 text-sm font-semibold tracking-wide">{import.meta.env.VITE_APP_NAME}</span>
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
      <span
        class="inline-flex"
        title={!canCreate
          ? 'Set a chat model in settings'
          : newTemporaryOpen
            ? 'Temporary chat already open'
            : 'Temporary chat (⌘⇧N, not saved)'}
      >
        <button
          class="rounded-md p-1.5 transition-colors disabled:opacity-40 disabled:pointer-events-none {temporaryActive
            ? 'bg-surface text-fg'
            : 'text-muted hover:bg-surface hover:text-fg'}"
          aria-pressed={temporaryActive}
          disabled={!canCreate || newTemporaryOpen}
          onclick={oncreatetemporary}
        >
          <Ghost size={16} />
        </button>
      </span>
      <span
        class="inline-flex"
        title={!canCreate ? 'Set a chat model in settings' : newChatOpen ? 'New chat already open' : 'New chat (⌘N)'}
      >
        <button
          class="rounded-md p-1.5 text-muted transition-colors hover:bg-surface hover:text-fg disabled:opacity-40 disabled:pointer-events-none"
          disabled={!canCreate || newChatOpen}
          onclick={oncreate}
        >
          <Plus size={16} />
        </button>
      </span>
    </div>
  </div>

  <nav class="flex-1 overflow-y-auto px-2 pb-2">
    {#each threads as thread (thread.id)}
      {#if renamingId === thread.id}
        <div class="mb-0.5 flex w-full items-center gap-2 rounded-md bg-surface px-2 py-1.5">
          <MessageSquare size={14} class="shrink-0 text-muted" />
          <input
            bind:this={renameInput}
            bind:value={renameDraft}
            onkeydown={handleRenameKeydown}
            onblur={() => void commitRename()}
            class="min-w-0 flex-1 rounded border border-line bg-panel px-1.5 py-0.5 text-sm text-fg outline-none focus:border-accent"
            aria-label="Chat title"
          />
        </div>
      {:else}
        <button
          class="mb-0.5 flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors {thread.id === activeId
            ? 'bg-surface text-fg'
            : 'text-muted hover:bg-surface hover:text-fg'}"
          onclick={() => onselect(thread.id)}
          oncontextmenu={(event) => openMenu(event, thread)}
        >
          <MessageSquare size={14} class="shrink-0" />
          <span class="truncate">{thread.title}</span>
        </button>
      {/if}
    {/each}
  </nav>

  {#if menu && menuThread}
    <div
      bind:this={menuEl}
      class="fixed z-50 min-w-40 rounded-lg border border-line bg-panel p-1 shadow-lg outline-none"
      role="menu"
      tabindex="-1"
      aria-label="Chat actions"
      style="left: {menu.x}px; top: {menu.y}px"
      oncontextmenu={(event) => event.preventDefault()}
    >
      <button
        class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-fg transition-colors hover:bg-surface"
        role="menuitem"
        onclick={renameFromMenu}
      >
        <Pencil size={14} />
        Rename
      </button>
      <button
        class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-red-500 transition-colors hover:bg-surface"
        role="menuitem"
        onclick={deleteFromMenu}
      >
        <Trash2 size={14} />
        Delete
      </button>
    </div>
  {/if}

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
