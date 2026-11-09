<script lang="ts">
  import { onMount } from 'svelte'
  import Sidebar from './lib/components/Sidebar.svelte'
  import ChatWindow from './lib/components/ChatWindow.svelte'
  import SettingsModal from './lib/components/SettingsModal.svelte'
  import { chat } from './lib/stores/chat.svelte'

  let settingsOpen = $state(false)

  const activeThread = $derived(chat.activeThread)

  onMount(() => {
    chat.init().catch((err) => {
      chat.error = err instanceof Error ? err.message : String(err)
    })
    return () => chat.destroy()
  })
</script>

<div class="flex h-full">
  <Sidebar
    threads={chat.threads}
    activeId={chat.activeId}
    oncreate={() => chat.createThread()}
    onselect={(id) => chat.select(id)}
    onsettings={() => (settingsOpen = true)}
  />
  <ChatWindow
    thread={activeThread}
    messages={chat.messages}
    streaming={chat.streaming}
    streamText={chat.streamText}
    error={chat.error}
    models={chat.models}
    defaultModel={chat.config?.chatModel ?? ''}
    onsend={(content, files) => chat.send(content, files)}
    onretry={() => chat.retry()}
    canRetry={chat.canRetry}
    onregenerate={() => chat.regenerate()}
    onedit={(id, content) => chat.edit(id, content)}
    onrename={(id, title) => chat.renameThread(id, title)}
    ondelete={(id) => chat.deleteThread(id)}
    onmodelchange={(model) => chat.setThreadModel(model)}
  />
</div>

<SettingsModal
  open={settingsOpen}
  cfg={chat.config}
  onclose={() => (settingsOpen = false)}
  onsave={(next) => chat.saveConfig(next)}
/>
