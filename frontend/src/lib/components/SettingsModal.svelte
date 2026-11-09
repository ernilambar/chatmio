<script lang="ts">
  import { untrack } from 'svelte'
  import { RefreshCw, X } from 'lucide-svelte'
  import { ListModels } from '../wailsjs/go/main/App'
  import { config } from '../wailsjs/go/models'
  import ModelCombobox from './ModelCombobox.svelte'

  let {
    open,
    cfg,
    onclose,
    onsave
  }: {
    open: boolean
    cfg: config.Config | null
    onclose: () => void
    onsave: (next: config.Config) => Promise<void>
  } = $props()

  let baseUrl = $state('')
  let apiKey = $state('')
  let chatModel = $state('')
  let titleModel = $state('')
  let systemPrompt = $state('')
  let models = $state<string[]>([])
  let modelsLoading = $state(false)
  let modelsError = $state<string | null>(null)
  let saving = $state(false)
  let error = $state<string | null>(null)

  const modelOptions = $derived(
    Array.from(new Set([chatModel, ...models].filter((model) => model !== '')))
  )
  const titleOptions = $derived(
    Array.from(new Set([titleModel, chatModel, ...models].filter((model) => model !== '')))
  )

  $effect(() => {
    if (open && cfg) {
      baseUrl = cfg.baseUrl
      apiKey = cfg.apiKey
      chatModel = cfg.chatModel
      titleModel = cfg.titleModel
      systemPrompt = cfg.systemPrompt
      error = null
      untrack(() => void loadModels())
    }
  })

  async function loadModels(): Promise<void> {
    const url = baseUrl.trim()
    modelsError = null
    if (!url) {
      models = []
      modelsError = 'Set a base URL to load models.'
      return
    }

    modelsLoading = true
    try {
      const next = await ListModels(url, apiKey)
      models = next
      if (next.length === 0) {
        modelsError = 'The provider returned no models.'
      }
    } catch (err) {
      models = []
      modelsError = err instanceof Error ? err.message : String(err)
    } finally {
      modelsLoading = false
    }
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault()
    if (!chatModel.trim()) {
      error = 'Chat model is required.'
      return
    }
    saving = true
    error = null
    try {
      await onsave(
        config.Config.createFrom({ baseUrl, apiKey, chatModel, titleModel, systemPrompt })
      )
      onclose()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      saving = false
    }
  }
</script>

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
    <form
      class="w-full max-w-md rounded-lg border border-line bg-surface p-5 shadow-xl"
      onsubmit={submit}
    >
      <div class="mb-4 flex items-center justify-between">
        <h2 class="text-sm font-semibold">Provider settings</h2>
        <button type="button" class="rounded-md p-1 text-muted hover:text-fg" title="Close" onclick={onclose}>
          <X size={16} />
        </button>
      </div>

      <div class="space-y-3">
        <label class="block space-y-1">
          <span class="text-xs text-muted">Base URL</span>
          <input
            bind:value={baseUrl}
            class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
            placeholder="https://api.openai.com/v1"
            required
          />
        </label>

        <label class="block space-y-1">
          <span class="text-xs text-muted">API key</span>
          <input
            bind:value={apiKey}
            type="password"
            class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
            placeholder="sk-…"
          />
        </label>

        <div class="flex items-center justify-between">
          <span class="text-xs text-muted">Models</span>
          <button
            type="button"
            class="flex items-center gap-1 rounded-md px-1.5 py-1 text-xs text-muted hover:text-fg disabled:opacity-40"
            onclick={() => void loadModels()}
            disabled={modelsLoading}
          >
            <RefreshCw size={12} class={modelsLoading ? 'animate-spin' : ''} />
            Refresh
          </button>
        </div>

        <label class="block space-y-1">
          <span class="text-xs text-muted">Chat model</span>
          {#if models.length > 0}
            <ModelCombobox bind:value={chatModel} options={modelOptions} placeholder="Select a chat model" />
          {:else}
            <input
              bind:value={chatModel}
              class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
              placeholder="gpt-4o-mini"
              required
            />
          {/if}
        </label>

        <label class="block space-y-1">
          <span class="text-xs text-muted">Title model</span>
          {#if models.length > 0}
            <ModelCombobox
              bind:value={titleModel}
              options={titleOptions}
              placeholder="Same as chat model"
              allowEmpty
              emptyLabel="Same as chat model"
            />
          {:else}
            <input
              bind:value={titleModel}
              class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
              placeholder="Same as chat model"
            />
          {/if}
          <span class="text-[11px] text-muted">Leave empty to use the chat model.</span>
        </label>

        {#if modelsLoading}
          <p class="text-[11px] text-muted">Loading models from the provider…</p>
        {:else if modelsError}
          <p class="text-[11px] text-amber-500">{modelsError} Enter the model ID manually.</p>
        {:else}
          <p class="text-[11px] text-muted">{models.length} models loaded.</p>
        {/if}

        <label class="block space-y-1">
          <span class="text-xs text-muted">System prompt</span>
          <textarea
            bind:value={systemPrompt}
            rows="3"
            class="w-full resize-none rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
            placeholder="Optional instructions for the assistant"
          ></textarea>
        </label>
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
