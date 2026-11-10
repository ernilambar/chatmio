<script lang="ts">
  import { untrack } from 'svelte'
  import { Monitor, Moon, RefreshCw, Sun, X } from 'lucide-svelte'
  import { ListModels } from '../wailsjs/go/main/App'
  import { config } from '../wailsjs/go/models'
  import { theme, type ThemeMode } from '../stores/theme.svelte'
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

  type Section = 'appearance' | 'chat' | 'provider' | 'web'

  const sections: { id: Section; label: string }[] = [
    { id: 'provider', label: 'Provider' },
    { id: 'chat', label: 'Chat defaults' },
    { id: 'web', label: 'Web tools' },
    { id: 'appearance', label: 'Appearance' }
  ]

  const themeOptions: { id: ThemeMode; label: string; icon: typeof Monitor }[] = [
    { id: 'system', label: 'System', icon: Monitor },
    { id: 'light', label: 'Light', icon: Sun },
    { id: 'dark', label: 'Dark', icon: Moon }
  ]

  let section = $state<Section>('provider')
  let baseUrl = $state('')
  let apiKey = $state('')
  let chatModel = $state('')
  let titleModel = $state('')
  let systemPrompt = $state('')
  let contextWindow = $state<number | null>(null)
  let temperature = $state<number | null>(null)
  let topP = $state<number | null>(null)
  let maxTokens = $state<number | null>(null)
  let frequencyPenalty = $state<number | null>(null)
  let braveApiKey = $state('')
  let ollamaApiKey = $state('')
  let searchMode = $state<'brave' | 'ollama' | 'disabled'>('brave')
  let themeMode = $state<ThemeMode>('system')
  let initialTheme = $state<ThemeMode>('system')
  let models = $state<string[]>([])
  let modelsLoading = $state(false)
  let modelsError = $state<string | null>(null)
  let saving = $state(false)
  let error = $state<string | null>(null)

  const sectionTitle = $derived(sections.find((item) => item.id === section)?.label ?? 'Settings')
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
      contextWindow = cfg.contextWindow || null
      temperature = cfg.temperature ?? null
      topP = cfg.topP ?? null
      maxTokens = cfg.maxTokens ?? null
      frequencyPenalty = cfg.frequencyPenalty ?? null
      braveApiKey = cfg.braveApiKey
      ollamaApiKey = cfg.ollamaApiKey
      searchMode = !cfg.webToolsEnabled
        ? 'disabled'
        : cfg.searchProvider === 'ollama'
          ? 'ollama'
          : 'brave'
      const savedTheme = cfg.theme === 'light' || cfg.theme === 'dark' ? cfg.theme : 'system'
      themeMode = savedTheme
      initialTheme = savedTheme
      section = 'provider'
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

  function selectTheme(next: ThemeMode): void {
    themeMode = next
    theme.mode = next
  }

  function cancel(): void {
    theme.mode = initialTheme
    onclose()
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault()
    saving = true
    error = null
    try {
      await onsave(
        config.Config.createFrom({
          baseUrl,
          apiKey,
          chatModel,
          titleModel,
          systemPrompt,
          contextWindow: contextWindow && contextWindow > 0 ? Math.floor(contextWindow) : 0,
          temperature: temperature ?? undefined,
          topP: topP ?? undefined,
          maxTokens: maxTokens ?? undefined,
          frequencyPenalty: frequencyPenalty ?? undefined,
          braveApiKey,
          ollamaApiKey,
          searchProvider: searchMode === 'ollama' ? 'ollama' : 'brave',
          webToolsEnabled: searchMode !== 'disabled',
          theme: themeMode
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
      cancel()
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
    <div
      class="flex h-[560px] max-h-[85vh] w-full max-w-3xl overflow-hidden rounded-lg border border-line bg-surface shadow-xl"
    >
      <aside class="flex w-48 shrink-0 flex-col gap-0.5 border-r border-line bg-panel p-2">
        <p class="px-2 pb-2 pt-1 text-xs font-semibold text-muted">Settings</p>
        {#each sections as item (item.id)}
          <button
            type="button"
            class="rounded-md px-2 py-1.5 text-left text-sm transition-colors {section ===
            item.id
              ? 'bg-surface font-medium text-fg shadow-sm'
              : 'text-muted hover:text-fg'}"
            onclick={() => (section = item.id)}
          >
            {item.label}
          </button>
        {/each}
      </aside>

      <form class="flex min-w-0 flex-1 flex-col" onsubmit={submit}>
        <div class="flex items-center justify-between border-b border-line px-5 py-3">
          <h2 class="text-sm font-semibold">{sectionTitle}</h2>
          <button
            type="button"
            class="rounded-md p-1 text-muted hover:text-fg"
            title="Close"
            aria-label="Close settings"
            onclick={cancel}
          >
            <X size={16} />
          </button>
        </div>

        <div class="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {#if section === 'appearance'}
            <div class="space-y-3">
              <p class="text-xs text-muted">Theme</p>
              <div class="grid grid-cols-3 gap-2">
                {#each themeOptions as option (option.id)}
                  {@const Icon = option.icon}
                  <button
                    type="button"
                    aria-pressed={themeMode === option.id}
                    class="flex flex-col items-center gap-1.5 rounded-md border px-2 py-4 text-xs transition-colors {themeMode ===
                    option.id
                      ? 'border-accent bg-panel text-fg'
                      : 'border-line text-muted hover:text-fg'}"
                    onclick={() => selectTheme(option.id)}
                  >
                    <Icon size={16} />
                    {option.label}
                  </button>
                {/each}
              </div>
              <p class="text-[11px] text-muted">
                System follows the macOS appearance. Changes preview immediately; Cancel restores the
                saved theme.
              </p>
            </div>
          {:else if section === 'chat'}
            <div class="space-y-3">
              <label class="block space-y-1">
                <span class="text-xs text-muted">System prompt</span>
                <textarea
                  bind:value={systemPrompt}
                  rows="3"
                  class="w-full resize-none rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                  placeholder="Optional instructions for the assistant"
                ></textarea>
              </label>

              <label class="block space-y-1">
                <span class="text-xs text-muted">Context window (tokens)</span>
                <input
                  bind:value={contextWindow}
                  type="number"
                  min="1"
                  step="1"
                  class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                  placeholder="128000"
                />
                <span class="text-[11px] text-muted">
                  Used for the context warning. Chats can override it.
                </span>
              </label>

              <p class="pt-1 text-xs text-muted">Generation defaults</p>
              <div class="grid grid-cols-2 gap-3">
                <label class="block space-y-1">
                  <span class="text-xs text-muted">Temperature</span>
                  <input
                    bind:value={temperature}
                    type="number"
                    min="0"
                    max="2"
                    step="0.1"
                    class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                    placeholder="Provider default"
                  />
                </label>
                <label class="block space-y-1">
                  <span class="text-xs text-muted">Top P</span>
                  <input
                    bind:value={topP}
                    type="number"
                    min="0.01"
                    max="1"
                    step="0.05"
                    class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                    placeholder="Provider default"
                  />
                </label>
                <label class="block space-y-1">
                  <span class="text-xs text-muted">Max output tokens</span>
                  <input
                    bind:value={maxTokens}
                    type="number"
                    min="1"
                    step="1"
                    class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                    placeholder="Provider default"
                  />
                </label>
                <label class="block space-y-1">
                  <span class="text-xs text-muted">Frequency penalty</span>
                  <input
                    bind:value={frequencyPenalty}
                    type="number"
                    min="-2"
                    max="2"
                    step="0.1"
                    class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                    placeholder="Provider default"
                  />
                </label>
              </div>
              <p class="text-[11px] text-muted">
                Copied into new chats. Existing chats keep their settings; clear a value in chat
                settings to use the provider default.
              </p>
            </div>
          {:else if section === 'provider'}
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
                  <ModelCombobox
                    bind:value={chatModel}
                    options={modelOptions}
                    placeholder="Select a chat model"
                  />
                {:else}
                  <input
                    bind:value={chatModel}
                    class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                    placeholder="gpt-4o-mini"
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
                <p class="text-[11px] text-amber-500">
                  {modelsError} Enter the model ID manually.
                </p>
              {:else}
                <p class="text-[11px] text-muted">{models.length} models loaded.</p>
              {/if}
            </div>
          {:else}
            <div class="space-y-3">
              <label class="block space-y-1">
                <span class="text-xs text-muted">Search provider</span>
                <select
                  bind:value={searchMode}
                  class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                >
                  <option value="disabled">Disabled</option>
                  <option value="brave">Brave Search</option>
                  <option value="ollama">Ollama</option>
                </select>
                <span class="text-[11px] text-muted">
                  Choose a provider and enter its API key, or disable web search everywhere.
                </span>
              </label>

              {#if searchMode === 'brave'}
                <label class="block space-y-1">
                  <span class="text-xs text-muted">Brave Search API key</span>
                  <input
                    bind:value={braveApiKey}
                    type="password"
                    class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                    placeholder="Required"
                  />
                  <span class="text-[11px] text-muted">
                    Enables the web search toggle in chats. Queries are sent to
                    api.search.brave.com. The chat model must support tool calling.
                  </span>
                </label>
              {:else if searchMode === 'ollama'}
                <label class="block space-y-1">
                  <span class="text-xs text-muted">Ollama API key</span>
                  <input
                    bind:value={ollamaApiKey}
                    type="password"
                    class="w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
                    placeholder="Required"
                  />
                  <span class="text-[11px] text-muted">
                    Enables the web search toggle in chats. Queries and fetched URLs are sent
                    to ollama.com. The chat model must support tool calling.
                  </span>
                </label>
              {:else}
                <p class="text-[11px] text-muted">
                  Web search and page fetch are off for every chat. Your provider and key are
                  kept for when you turn them back on.
                </p>
              {/if}
            </div>
          {/if}
        </div>

        <div class="flex items-center justify-between gap-3 border-t border-line px-5 py-3">
          <p class="min-w-0 truncate text-xs text-red-500">{error ?? ''}</p>
          <div class="flex shrink-0 gap-2">
            <button
              type="button"
              class="rounded-md px-3 py-1.5 text-sm text-muted hover:text-fg"
              onclick={cancel}
            >
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
        </div>
      </form>
    </div>
  </div>
{/if}
