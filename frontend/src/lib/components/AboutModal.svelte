<script lang="ts">
  import { onMount } from 'svelte'
  import { X } from 'lucide-svelte'
  import { GetAboutInfo } from '../wailsjs/go/main/App'
  import { main } from '../wailsjs/go/models'
  import { BrowserOpenURL } from '../wailsjs/runtime/runtime'
  import { trapFocus } from '../focus'

  let { open, onclose }: { open: boolean; onclose: () => void } = $props()

  let info = $state<main.AboutInfo | null>(null)
  let error = $state<string | null>(null)

  onMount(() => {
    GetAboutInfo()
      .then((next) => (info = next))
      .catch((err) => (error = err instanceof Error ? err.message : String(err)))
  })

  function handleKeydown(event: KeyboardEvent): void {
    if (open && event.key === 'Escape' && !event.defaultPrevented) {
      event.preventDefault()
      onclose()
    }
  }

  function openLink(url: string | undefined): void {
    if (url) BrowserOpenURL(url)
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
    role="presentation"
    onclick={(event) => {
      if (event.target === event.currentTarget) onclose()
    }}
  >
    <div
      use:trapFocus
      class="relative w-full max-w-sm rounded-xl border border-line bg-surface px-6 pb-6 pt-8 text-center shadow-xl"
      role="dialog"
      aria-modal="true"
      aria-labelledby="about-title"
    >
      <button
        type="button"
        class="absolute right-3 top-3 rounded-md p-1 text-muted hover:text-fg"
        title="Close"
        onclick={onclose}
      >
        <X size={16} />
      </button>

      {#if info}
        <img src={info.icon} alt="" class="mx-auto size-28 select-none" draggable="false" />
        <h2 id="about-title" class="mt-2 text-lg font-semibold">{info.title}</h2>
        <p class="mt-1 text-xs font-medium text-muted">Version {info.version}</p>
        {#if info.description}
          <p class="mx-auto mt-4 max-w-64 text-sm leading-relaxed text-muted">{info.description}</p>
        {/if}
        <p class="mt-4 flex flex-wrap items-center justify-center gap-x-3 gap-y-1 text-xs">
          {#if info.docsUrl}
            <a
              href={info.docsUrl}
              onclick={(event) => {
                event.preventDefault()
                openLink(info?.docsUrl)
              }}
              class="text-muted underline-offset-2 hover:text-fg hover:underline">Documentation</a
            >
          {/if}
          {#if info.issuesUrl}
            <a
              href={info.issuesUrl}
              onclick={(event) => {
                event.preventDefault()
                openLink(info?.issuesUrl)
              }}
              class="text-muted underline-offset-2 hover:text-fg hover:underline">Report an Issue</a
            >
          {/if}
        </p>
        {#if info.copyright}
          <p class="mt-5 text-[11px] text-muted">{info.copyright}</p>
        {/if}
      {:else if error}
        <p class="text-sm text-red-500">{error}</p>
      {:else}
        <p class="text-sm text-muted">Loading…</p>
      {/if}

      <button
        type="button"
        class="mt-6 rounded-md border border-line px-4 py-1.5 text-sm text-muted hover:text-fg"
        onclick={onclose}
      >
        Close
      </button>
    </div>
  </div>
{/if}
