<script module lang="ts">
  import DOMPurify from 'dompurify'
  import { BrowserOpenURL } from '../wailsjs/runtime/runtime'

  // Open links outside the app window instead of navigating the webview away.
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'A') {
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener noreferrer')
    }
  })

  // Delegate once for every rendered message instead of one listener per instance.
  document.addEventListener('click', (event) => {
    const anchor = (event.target as Element | null)?.closest?.('.markdown a')
    if (!anchor) return
    const href = anchor.getAttribute('href') ?? ''
    event.preventDefault()
    if (/^https?:\/\//i.test(href)) BrowserOpenURL(href)
  })
</script>

<script lang="ts">
  import { marked } from 'marked'

  let { content, class: className = '' }: { content: string; class?: string } = $props()

  marked.setOptions({ gfm: true, breaks: true })

  const html = $derived(DOMPurify.sanitize(marked.parse(content) as string))
</script>

<div class="markdown {className}">{@html html}</div>
