<script lang="ts">
  import Sidebar from './lib/components/Sidebar.svelte'
  import ChatWindow from './lib/components/ChatWindow.svelte'
  import { chat } from './lib/stores/chat.svelte'

  const activeThread = $derived(chat.activeThread)
</script>

<div class="flex h-full">
  <Sidebar
    threads={chat.threads}
    activeId={chat.activeId}
    oncreate={() => chat.createThread()}
    onselect={(id) => chat.select(id)}
  />
  <ChatWindow thread={activeThread} />
</div>
