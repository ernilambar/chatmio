<script lang="ts">
  import { MessageSquare, Send } from 'lucide-svelte'
  import type { db } from '../wailsjs/go/models'

  let {
    thread,
    messages,
    streaming,
    streamText,
    error,
    onsend
  }: {
    thread: db.Thread | null
    messages: db.Message[]
    streaming: boolean
    streamText: string
    error: string | null
    onsend: (content: string) => void
  } = $props()

  let draft = $state('')
  let scroller: HTMLDivElement | undefined = $state()

  $effect(() => {
    messages
    streamText
    if (scroller) scroller.scrollTop = scroller.scrollHeight
  })

  function submit(): void {
    const content = draft.trim()
    if (!content || streaming) return
    draft = ''
    onsend(content)
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      submit()
    }
  }
</script>

<main class="flex min-w-0 flex-1 flex-col">
  <header class="border-b border-line px-6 py-3">
    <h1 class="truncate text-sm font-medium">{thread?.title ?? 'Chattu'}</h1>
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
          <div class="flex {message.role === 'user' ? 'justify-end' : 'justify-start'}">
            <div
              class="max-w-[80%] whitespace-pre-wrap rounded-lg px-3 py-2 text-sm leading-relaxed {message.role ===
              'user'
                ? 'bg-accent text-white'
                : 'bg-panel'}"
            >
              {message.content}
            </div>
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
    <div class="flex items-end gap-2 rounded-lg border border-line bg-panel px-3 py-2">
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
        disabled={!draft.trim() || streaming}
        onclick={submit}
      >
        <Send size={16} />
      </button>
    </div>
  </div>
</main>
