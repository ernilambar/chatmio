<script lang="ts">
  import { MessageSquare } from 'lucide-svelte'
  import type { Thread } from '../types'

  let { thread }: { thread: Thread | null } = $props()
</script>

<main class="flex min-w-0 flex-1 flex-col">
  <header class="border-b border-line px-6 py-3">
    <h1 class="truncate text-sm font-medium">{thread?.title ?? 'Chattu'}</h1>
  </header>

  {#if thread === null}
    <div class="flex flex-1 flex-col items-center justify-center gap-2 text-muted">
      <MessageSquare size={28} />
      <p class="text-sm">Start a new chat to begin.</p>
    </div>
  {:else if thread.messages.length === 0}
    <div class="flex flex-1 flex-col items-center justify-center gap-2 text-muted">
      <MessageSquare size={28} />
      <p class="text-sm">No messages yet.</p>
    </div>
  {:else}
    <div class="flex-1 space-y-4 overflow-y-auto px-6 py-4">
      {#each thread.messages as message (message.id)}
        <article class="space-y-1">
          <p class="text-xs font-medium uppercase tracking-wide text-muted">{message.role}</p>
          <p class="whitespace-pre-wrap text-sm leading-relaxed">{message.content}</p>
        </article>
      {/each}
    </div>
  {/if}
</main>
