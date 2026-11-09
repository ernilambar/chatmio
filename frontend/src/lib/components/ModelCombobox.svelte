<script lang="ts">
  import { Check, ChevronsUpDown } from 'lucide-svelte'

  let {
    value = $bindable(''),
    options,
    placeholder = 'Select a model',
    allowEmpty = false,
    emptyLabel = 'Same as chat model'
  }: {
    value?: string
    options: string[]
    placeholder?: string
    allowEmpty?: boolean
    emptyLabel?: string
  } = $props()

  let open = $state(false)
  let query = $state('')
  let root: HTMLDivElement | undefined = $state()
  let searchInput: HTMLInputElement | undefined = $state()

  const filtered = $derived(
    options.filter((model) => model.toLowerCase().includes(query.trim().toLowerCase()))
  )

  $effect(() => {
    if (open && searchInput) searchInput.focus()
  })

  function choose(model: string): void {
    value = model
    open = false
    query = ''
  }

  function toggle(): void {
    open = !open
    if (open) query = ''
  }

  function handleWindowClick(event: MouseEvent): void {
    if (!open || !root) return
    if (!root.contains(event.target as Node)) open = false
  }

  function handleSearchKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault()
      open = false
    } else if (event.key === 'Enter') {
      event.preventDefault()
      if (filtered.length > 0) choose(filtered[0])
    }
  }
</script>

<svelte:window onclick={handleWindowClick} />

<div bind:this={root} class="relative">
  <button
    type="button"
    class="flex w-full items-center justify-between gap-2 rounded-md border border-line bg-panel px-2 py-1.5 text-left text-sm outline-none focus:border-accent"
    onclick={toggle}
  >
    <span class="truncate {value ? '' : 'text-muted'}">{value || placeholder}</span>
    <ChevronsUpDown size={14} class="shrink-0 text-muted" />
  </button>

  {#if open}
    <div class="absolute z-30 mt-1 w-full rounded-md border border-line bg-surface p-1 shadow-xl">
      <input
        bind:this={searchInput}
        bind:value={query}
        onkeydown={handleSearchKeydown}
        class="mb-1 w-full rounded-md border border-line bg-panel px-2 py-1.5 text-sm outline-none focus:border-accent"
        placeholder="Search models…"
      />
      <div class="max-h-56 overflow-y-auto">
        {#if allowEmpty}
          <button
            type="button"
            class="flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-panel"
            onclick={() => choose('')}
          >
            <span>{emptyLabel}</span>
            {#if value === ''}<Check size={14} />{/if}
          </button>
        {/if}
        {#each filtered as model (model)}
          <button
            type="button"
            class="flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-panel"
            onclick={() => choose(model)}
          >
            <span class="truncate">{model}</span>
            {#if value === model}<Check size={14} />{/if}
          </button>
        {/each}
        {#if filtered.length === 0}
          <p class="px-2 py-1.5 text-xs text-muted">No matching models</p>
        {/if}
      </div>
    </div>
  {/if}
</div>
