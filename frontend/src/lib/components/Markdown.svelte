<script module lang="ts">
  import DOMPurify from 'dompurify'
  import hljs from 'highlight.js/lib/common'
  import { marked, type Tokens } from 'marked'
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

  function escapeHtml(value: string): string {
    return value
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
  }

  marked.setOptions({ gfm: true, breaks: true })
  marked.use({
    renderer: {
      code({ text, lang }: Tokens.Code) {
        const requested = (lang ?? '').trim().split(/\s+/)[0]
        const language = requested && hljs.getLanguage(requested) ? requested : ''
        const body = language ? hljs.highlight(text, { language }).value : escapeHtml(text)
        const className = language
          ? `hljs language-${language.replace(/[^\w+-]/g, '')}`
          : 'hljs'
        return `<pre><code class="${className}">${body}</code></pre>\n`
      }
    }
  })
</script>

<script lang="ts">
  import { copyText } from '../clipboard'

  let { content, class: className = '' }: { content: string; class?: string } = $props()

  let root: HTMLDivElement | undefined = $state()

  const html = $derived(DOMPurify.sanitize(marked.parse(content) as string))

  // Attach a copy button to every code block after the HTML is rendered.
  $effect(() => {
    html
    const element = root
    if (!element) return
    for (const pre of element.querySelectorAll('pre')) {
      if (pre.parentElement?.classList.contains('code-block')) continue
      const wrapper = document.createElement('div')
      wrapper.className = 'code-block'
      pre.replaceWith(wrapper)
      wrapper.appendChild(pre)

      const button = document.createElement('button')
      button.type = 'button'
      button.className = 'code-copy'
      button.textContent = 'Copy'
      button.setAttribute('aria-label', 'Copy code')
      button.addEventListener('click', () => {
        const code = pre.querySelector('code')?.textContent ?? ''
        void copyText(code).then(() => {
          button.textContent = 'Copied'
          setTimeout(() => {
            button.textContent = 'Copy'
          }, 1500)
        })
      })
      wrapper.appendChild(button)
    }
  })
</script>

<div bind:this={root} class="markdown {className}">{@html html}</div>
