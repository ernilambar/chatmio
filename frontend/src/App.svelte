<script lang="ts">
  import { onMount } from 'svelte'
  import Sidebar from './lib/components/Sidebar.svelte'
  import ChatWindow from './lib/components/ChatWindow.svelte'
  import SearchPalette from './lib/components/SearchPalette.svelte'
  import SettingsModal from './lib/components/SettingsModal.svelte'
  import AboutModal from './lib/components/AboutModal.svelte'
  import ChatSettingsModal from './lib/components/ChatSettingsModal.svelte'
  import { EventsOn } from './lib/wailsjs/runtime/runtime'
  import { chat } from './lib/stores/chat.svelte'
  import { disableTextAssist } from './lib/disable-autosuggest'

  const SIDEBAR_KEY = 'chattu.sidebar.visible'

  function readSidebarVisible(): boolean {
    try {
      return localStorage.getItem(SIDEBAR_KEY) !== 'false'
    } catch {
      return true
    }
  }

  let settingsOpen = $state(false)
  let searchOpen = $state(false)
  let aboutOpen = $state(false)
  let chatSettingsOpen = $state(false)
  let composerFocus = $state(0)
  let sidebarOpen = $state(readSidebarVisible())

  const activeThread = $derived(chat.activeThread)

  function toggleSidebar(): void {
    sidebarOpen = !sidebarOpen
    try {
      localStorage.setItem(SIDEBAR_KEY, String(sidebarOpen))
    } catch {
      // Persistence is best-effort; the toggle still works for this session.
    }
  }

  onMount(() => {
    const stopTextAssist = disableTextAssist()
    const stopAbout = EventsOn('app:about', () => {
      aboutOpen = true
    })
    chat.init().catch((err) => {
      chat.error = err instanceof Error ? err.message : String(err)
    })
    return () => {
      stopAbout()
      stopTextAssist()
      chat.destroy()
    }
  })

  function handleKeydown(event: KeyboardEvent): void {
    const mod = event.metaKey || event.ctrlKey
    const key = event.key.toLowerCase()

    if (mod && key === 'k') {
      event.preventDefault()
      searchOpen = !searchOpen
      return
    }
    if (mod && key === 'n') {
      event.preventDefault()
      void chat.createThread()
      return
    }
    if (mod && event.key === ',') {
      event.preventDefault()
      settingsOpen = true
      return
    }
    if (mod && key === 'l') {
      event.preventDefault()
      composerFocus += 1
      return
    }
    if (mod && key === 'b') {
      event.preventDefault()
      toggleSidebar()
      return
    }
    if (
      event.key === 'Escape' &&
      !event.defaultPrevented &&
      !settingsOpen &&
      !searchOpen &&
      !aboutOpen &&
      !chatSettingsOpen &&
      chat.streaming
    ) {
      void chat.cancel()
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="flex h-full">
  {#if sidebarOpen}
    <Sidebar
      threads={chat.threads}
      activeId={chat.activeId}
      oncreate={() => chat.createThread()}
      onselect={(id) => chat.select(id)}
      onsettings={() => (settingsOpen = true)}
      onsearch={() => (searchOpen = true)}
      onhide={toggleSidebar}
    />
  {/if}
  <ChatWindow
    thread={activeThread}
    messages={chat.messages}
    streaming={chat.streaming}
    streamText={chat.streamText}
    streamReasoning={chat.streamReasoning}
    streamTools={chat.streamTools}
    webToolsAvailable={!!chat.config?.ollamaApiKey}
    onwebtoolschange={(enabled) => chat.setThreadWebTools(enabled)}
    contextUsed={chat.contextUsed}
    contextWindow={chat.contextWindow}
    error={chat.error}
    models={chat.models}
    defaultModel={chat.config?.chatModel ?? ''}
    sidebarHidden={!sidebarOpen}
    onshowsidebar={toggleSidebar}
    onsend={(content, files) => chat.send(content, files)}
    onretry={() => chat.retry()}
    canRetry={chat.canRetry}
    onregenerate={() => chat.regenerate()}
    onedit={(id, content) => chat.edit(id, content)}
    onrename={(id, title) => chat.renameThread(id, title)}
    ondelete={(id) => chat.deleteThread(id)}
    onmodelchange={(model) => chat.setThreadModel(model)}
    oncancel={() => void chat.cancel()}
    onopensettings={() => (chatSettingsOpen = true)}
    focusNonce={composerFocus}
  />
</div>

<SearchPalette
  open={searchOpen}
  threads={chat.threads}
  onselect={(id) => void chat.select(id)}
  onclose={() => (searchOpen = false)}
/>

<SettingsModal
  open={settingsOpen}
  cfg={chat.config}
  onclose={() => (settingsOpen = false)}
  onsave={(next) => chat.saveConfig(next)}
/>

<ChatSettingsModal
  open={chatSettingsOpen}
  thread={activeThread}
  globalSystemPrompt={chat.config?.systemPrompt ?? ''}
  globalContextWindow={chat.config?.contextWindow ?? 0}
  onclose={() => (chatSettingsOpen = false)}
  onsave={(settings) => chat.setThreadSettings(settings)}
/>

<AboutModal open={aboutOpen} onclose={() => (aboutOpen = false)} />
