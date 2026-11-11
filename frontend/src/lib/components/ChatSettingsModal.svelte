<script lang="ts">
  import { X } from 'lucide-svelte'
  import { db } from '../wailsjs/go/models'
  import { trapFocus } from '../focus'

  let {
    open,
    thread,
    globalSystemPrompt,
    globalContextWindow,
    onclose,
    onsave
  }: {
    open: boolean
    thread: db.Thread | null
    globalSystemPrompt: string
    globalContextWindow: number
    onclose: () => void
    onsave: (settings: db.ThreadSettings) => Promise<void>
  } = $props()

  // Number inputs bind to null when empty; empty means provider default.
  let systemPrompt = $state('')
  let temperature = $state<number | null>(null)
  let topP = $state<number | null>(null)
  let maxTokens = $state<number | null>(null)
  let frequencyPenalty = $state<number | null>(null)
  let reasoningEffort = $state('')
  let contextWindow = $state<number | null>(null)
  let saving = $state(false)
  let error = $state<string | null>(null)

  $effect(() => {
    if (open && thread) {
      systemPrompt = thread.systemPrompt
      temperature = thread.temperature ?? null
      topP = thread.topP ?? null
      maxTokens = thread.maxTokens ?? null
      frequencyPenalty = thread.frequencyPenalty ?? null
      reasoningEffort = thread.reasoningEffort
      contextWindow = thread.contextWindow ?? null
      error = null
    }
  })

  function optional(value: number | null): number | undefined {
    return value === null || Number.isNaN(value) ? undefined : value
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault()
    saving = true
    error = null
    try {
      await onsave(
        db.ThreadSettings.createFrom({
          systemPrompt,
          temperature: optional(temperature),
          topP: optional(topP),
          maxTokens: optional(maxTokens),
          frequencyPenalty: optional(frequencyPenalty),
          reasoningEffort,
          contextWindow: optional(contextWindow)
        })
      )
      onclose()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      saving = false
    }
  }

  function handleWindowKeydown(event: KeyboardEvent): void {
    if (open && event.key === 'Escape' && !event.defaultPrevented) {
      event.preventDefault()
      onclose()
    }
  }

  const inputClass =
    'w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent'
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if open && thread}
  <div
    use:trapFocus
    role="dialog"
    aria-modal="true"
    aria-labelledby="chat-settings-title"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
  >
    <form
      class="max-h-full w-full max-w-md overflow-y-auto rounded-lg border border-line bg-surface p-5 shadow-xl"
      onsubmit={submit}
    >
      <div class="mb-4 flex items-center justify-between">
        <h2 id="chat-settings-title" class="text-sm font-semibold">Chat settings</h2>
        <button type="button" class="rounded-md p-1 text-muted hover:text-fg" title="Close" onclick={onclose}>
          <X size={16} />
        </button>
      </div>

      <div class="space-y-3">
        <label class="block space-y-1">
          <span class="text-xs text-muted">System prompt</span>
          <textarea
            bind:value={systemPrompt}
            rows="4"
            class="{inputClass} resize-none"
            placeholder={globalSystemPrompt || 'Optional instructions for this chat'}
          ></textarea>
          <span class="text-[11px] text-muted">Leave empty to use the global system prompt.</span>
        </label>

        <p class="text-[11px] text-muted">Leave a field empty to use the provider default.</p>

        <div class="grid grid-cols-2 gap-3">
          <label class="block space-y-1">
            <span class="text-xs text-muted">Temperature</span>
            <input bind:value={temperature} type="number" min="0" max="2" step="0.1" class={inputClass} placeholder="Default" />
          </label>
          <label class="block space-y-1">
            <span class="text-xs text-muted">Top P</span>
            <input bind:value={topP} type="number" min="0.01" max="1" step="0.05" class={inputClass} placeholder="Default" />
          </label>
          <label class="block space-y-1">
            <span class="text-xs text-muted">Max output tokens</span>
            <input bind:value={maxTokens} type="number" min="1" step="1" class={inputClass} placeholder="Default" />
          </label>
          <label class="block space-y-1">
            <span class="text-xs text-muted">Frequency penalty</span>
            <input
              bind:value={frequencyPenalty}
              type="number"
              min="-2"
              max="2"
              step="0.1"
              class={inputClass}
              placeholder="Default"
            />
          </label>
          <label class="block space-y-1">
            <span class="text-xs text-muted">Reasoning effort</span>
            <select bind:value={reasoningEffort} class={inputClass}>
              <option value="">Default</option>
              <option value="low">Low</option>
              <option value="medium">Medium</option>
              <option value="high">High</option>
            </select>
          </label>
          <label class="block space-y-1">
            <span class="text-xs text-muted">Context window</span>
            <input
              bind:value={contextWindow}
              type="number"
              min="1"
              step="1"
              class={inputClass}
              placeholder={globalContextWindow ? String(globalContextWindow) : 'Not set'}
            />
          </label>
        </div>
        <p class="text-[11px] text-muted">
          Reasoning effort applies to reasoning models only. Some reasoning models reject temperature
          and top P; leave them empty for those.
        </p>
      </div>

      {#if error}
        <p class="mt-3 text-xs text-red-500">{error}</p>
      {/if}

      <div class="mt-5 flex justify-end gap-2">
        <button type="button" class="rounded-md px-3 py-1.5 text-sm text-muted hover:text-fg" onclick={onclose}>
          Cancel
        </button>
        <button
          type="submit"
          class="rounded-md bg-accent px-3 py-1.5 text-sm text-white disabled:opacity-40"
          disabled={saving}
        >
          Save
        </button>
      </div>
    </form>
  </div>
{/if}
