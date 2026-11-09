<script lang="ts">
  import {
    Check,
    Copy,
    MessageSquare,
    Paperclip,
    Pencil,
    Send,
    Trash2,
    X
  } from 'lucide-svelte'
  import type { db } from '../wailsjs/go/models'

  let {
    thread,
    messages,
    streaming,
    streamText,
    error,
    models,
    defaultModel,
    onsend,
    onrename,
    ondelete,
    onmodelchange
  }: {
    thread: db.Thread | null
    messages: db.Message[]
    streaming: boolean
    streamText: string
    error: string | null
    models: string[]
    defaultModel: string
    onsend: (content: string, files: File[]) => void
    onrename: (threadId: string, title: string) => Promise<void> | void
    ondelete: (threadId: string) => Promise<void> | void
    onmodelchange: (model: string) => Promise<void> | void
  } = $props()

  let draft = $state('')
  let files = $state<File[]>([])
  let fileInput: HTMLInputElement | undefined = $state()
  let scroller: HTMLDivElement | undefined = $state()
  let renaming = $state(false)
  let renameInput: HTMLInputElement | undefined = $state()
  let titleDraft = $state('')
  let confirmingDelete = $state(false)
  let copiedId = $state<string | null>(null)
  let copyTimer: ReturnType<typeof setTimeout> | undefined

  const selectedModel = $derived(thread?.model || defaultModel)
  const modelOptions = $derived.by(() => {
    const values = new Set(models)
    if (defaultModel) values.add(defaultModel)
    if (selectedModel) values.add(selectedModel)
    return [...values]
  })

  let lastThreadId: string | null | undefined = undefined

  $effect(() => {
    const id = thread?.id ?? null
    if (id !== lastThreadId) {
      lastThreadId = id
      draft = ''
      files = []
      renaming = false
      confirmingDelete = false
    }
  })

  $effect(() => {
    if (renaming && renameInput) renameInput.focus()
  })

  $effect(() => {
    messages
    streamText
    if (scroller) scroller.scrollTop = scroller.scrollHeight
  })

  function submit(): void {
    const content = draft.trim()
    if ((!content && files.length === 0) || streaming) return
    const selected = files
    draft = ''
    files = []
    onsend(content, selected)
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      submit()
    }
  }

  function addFiles(next: Iterable<File>): void {
    files = [...files, ...Array.from(next)]
  }

  function pickFiles(event: Event): void {
    const input = event.currentTarget as HTMLInputElement
    addFiles(input.files ?? [])
    input.value = ''
  }

  function removeFile(index: number): void {
    files = files.filter((_, i) => i !== index)
  }

  function handleDrop(event: DragEvent): void {
    event.preventDefault()
    if (event.dataTransfer?.files) addFiles(event.dataTransfer.files)
  }

  function handleDragOver(event: DragEvent): void {
    event.preventDefault()
  }

  function startRename(): void {
    if (!thread) return
    titleDraft = thread.title
    renaming = true
  }

  async function commitRename(): Promise<void> {
    if (!renaming || !thread) return
    renaming = false
    const title = titleDraft.trim()
    if (title && title !== thread.title) {
      await onrename(thread.id, title)
    }
  }

  function handleRenameKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      event.preventDefault()
      void commitRename()
    } else if (event.key === 'Escape') {
      event.preventDefault()
      renaming = false
    }
  }

  function removeThread(): void {
    if (!thread) return
    confirmingDelete = false
    void ondelete(thread.id)
  }

  async function copyMessage(message: db.Message): Promise<void> {
    if (!message.content) return
    try {
      await navigator.clipboard.writeText(message.content)
    } catch {
      const area = document.createElement('textarea')
      area.value = message.content
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.appendChild(area)
      area.select()
      document.execCommand('copy')
      area.remove()
    }
    copiedId = message.id
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copiedId = null
    }, 1500)
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }
</script>

<main class="flex min-w-0 flex-1 flex-col">
  <header class="flex items-start justify-between gap-3 border-b border-line px-6 py-3">
    <div class="min-w-0 flex-1">
      {#if renaming}
        <input
          bind:this={renameInput}
          bind:value={titleDraft}
          onkeydown={handleRenameKeydown}
          onblur={() => void commitRename()}
          class="w-full rounded-md border border-line bg-panel px-2 py-1 text-sm outline-none focus:border-accent"
        />
      {:else}
        <div class="flex items-center gap-1.5">
          <h1 class="truncate text-sm font-medium">{thread?.title ?? 'Chattu'}</h1>
        </div>
      {/if}

      <div class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-muted">
        <span>{messages.length} {messages.length === 1 ? 'message' : 'messages'}</span>
        {#if thread}
          <span>·</span>
          <span>Model</span>
          <select
            class="max-w-56 rounded-md border border-line bg-panel px-1.5 py-0.5 text-xs outline-none focus:border-accent"
            value={selectedModel}
            onchange={(event) => void onmodelchange((event.currentTarget as HTMLSelectElement).value)}
            disabled={streaming}
          >
            {#if !selectedModel}
              <option value="" disabled>Select model</option>
            {/if}
            {#each modelOptions as model (model)}
              <option value={model}>{model}</option>
            {/each}
          </select>
        {/if}
      </div>
    </div>

    {#if thread}
      <div class="flex items-center gap-1">
        <button
          class="rounded-md p-1.5 text-muted hover:bg-panel hover:text-fg"
          title="Rename chat"
          onclick={startRename}
        >
          <Pencil size={15} />
        </button>
        {#if confirmingDelete}
          <button
            class="rounded-md bg-red-500/10 p-1.5 text-red-500 hover:bg-red-500/20"
            title="Delete chat"
            onclick={removeThread}
          >
            <Trash2 size={15} />
          </button>
          <button
            class="rounded-md p-1.5 text-muted hover:bg-panel hover:text-fg"
            title="Cancel delete"
            onclick={() => (confirmingDelete = false)}
          >
            <X size={15} />
          </button>
        {:else}
          <button
            class="rounded-md p-1.5 text-muted hover:bg-panel hover:text-red-500"
            title="Delete chat"
            onclick={() => (confirmingDelete = true)}
          >
            <Trash2 size={15} />
          </button>
        {/if}
      </div>
    {/if}
  </header>

  <div bind:this={scroller} class="flex-1 overflow-y-auto px-6 py-4">
    {#if messages.length === 0 && !streaming}
      <div class="flex h-full flex-col items-center justify-center gap-2 text-muted">
        <MessageSquare size={28} />
        <p class="text-sm">Send a message to start the conversation.</p>
      </div>
    {:else}
      <div class="space-y-3">
        {#each messages as message (message.id)}
          <div class="group flex {message.role === 'user' ? 'justify-end' : 'justify-start'}">
            <div
              class="max-w-[80%] rounded-lg px-3 py-2 text-sm leading-relaxed {message.role ===
              'user'
                ? 'bg-accent text-white'
                : 'bg-panel'}"
            >
              {#if message.attachments?.length}
                <div class="mb-2 flex flex-wrap gap-2">
                  {#each message.attachments as attachment (attachment.id)}
                    {#if attachment.mimeType.startsWith('image/') && attachment.dataUrl}
                      <img
                        src={attachment.dataUrl}
                        alt={attachment.name}
                        class="max-h-56 max-w-full rounded-md object-contain"
                      />
                    {:else}
                      <span class="inline-flex max-w-full items-center gap-1 rounded-md bg-black/10 px-2 py-1 text-xs">
                        <Paperclip size={12} class="shrink-0" />
                        <span class="truncate">{attachment.name}</span>
                        <span class="shrink-0 opacity-60">{formatSize(attachment.size)}</span>
                      </span>
                    {/if}
                  {/each}
                </div>
              {/if}
              {#if message.content}
                <div class="whitespace-pre-wrap">{message.content}</div>
              {/if}
            </div>
            {#if message.content}
              <button
                class="ml-1 self-start rounded-md p-1 text-muted opacity-0 transition-opacity hover:text-fg group-hover:opacity-100"
                title="Copy message"
                onclick={() => void copyMessage(message)}
              >
                {#if copiedId === message.id}
                  <Check size={13} />
                {:else}
                  <Copy size={13} />
                {/if}
              </button>
            {/if}
          </div>
        {/each}

        {#if streaming}
          <div class="flex justify-start">
            <div class="max-w-[80%] whitespace-pre-wrap rounded-lg bg-panel px-3 py-2 text-sm leading-relaxed">
              {streamText || '…'}
            </div>
          </div>
        {/if}
      </div>
    {/if}

    {#if error}
      <p class="mt-3 rounded-md border border-red-500/40 bg-red-500/10 px-3 py-2 text-xs text-red-500">
        {error}
      </p>
    {/if}
  </div>

  <div class="border-t border-line p-4">
    {#if files.length > 0}
      <div class="mb-2 flex flex-wrap gap-2">
        {#each files as file, index (file.name + file.size + index)}
          <span class="inline-flex max-w-full items-center gap-1 rounded-md bg-panel px-2 py-1 text-xs">
            <Paperclip size={12} class="shrink-0" />
            <span class="truncate">{file.name}</span>
            <span class="shrink-0 text-muted">{formatSize(file.size)}</span>
            <button
              type="button"
              class="shrink-0 rounded p-0.5 hover:text-red-500"
              title="Remove attachment"
              onclick={() => removeFile(index)}
            >
              <X size={12} />
            </button>
          </span>
        {/each}
      </div>
    {/if}

    <div
      class="flex items-end gap-2 rounded-lg border border-line bg-panel px-3 py-2"
      role="group"
      aria-label="Message composer"
      ondragover={handleDragOver}
      ondrop={handleDrop}
    >
      <input
        bind:this={fileInput}
        type="file"
        multiple
        class="hidden"
        accept="image/*,text/*,.md,.markdown,.json,.jsonl,.csv,.tsv,.yaml,.yml,.toml,.xml,.html,.htm,.css,.js,.mjs,.cjs,.jsx,.ts,.tsx,.go,.py,.rb,.java,.c,.h,.cpp,.hpp,.rs,.sh,.bash,.zsh,.sql,.log,.ini,.conf,.env"
        onchange={pickFiles}
      />
      <button
        type="button"
        class="rounded-md p-1.5 text-muted transition-colors hover:text-fg disabled:opacity-40"
        title="Attach files"
        disabled={streaming}
        onclick={() => fileInput?.click()}
      >
        <Paperclip size={16} />
      </button>
      <textarea
        bind:value={draft}
        onkeydown={handleKeydown}
        rows="1"
        placeholder="Send a message…"
        class="max-h-40 flex-1 resize-none bg-transparent text-sm outline-none placeholder:text-muted"
      ></textarea>
      <button
        class="rounded-md bg-accent p-1.5 text-white transition-opacity disabled:opacity-40"
        title="Send"
        disabled={(!draft.trim() && files.length === 0) || streaming}
        onclick={submit}
      >
        <Send size={16} />
      </button>
    </div>
  </div>
</main>
