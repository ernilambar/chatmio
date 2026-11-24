<script module lang="ts">
  import DOMPurify from 'dompurify'
  import hljs from 'highlight.js/lib/common'
  import { marked, type Tokens } from 'marked'
  import { BrowserOpenURL } from '../wailsjs/runtime/runtime'

  // Replies are untrusted (prompt injection via web tools), so only the tags
  // Markdown produces survive. Images, forms, styles and media are dropped:
  // a remote image loads without a click and can exfiltrate chat content.
  const SANITIZE = {
    ALLOWED_TAGS: [
      'a', 'blockquote', 'br', 'code', 'del', 'em', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
      'hr', 'input', 'li', 'ol', 'p', 'pre', 'span', 'strong', 'table', 'tbody', 'td',
      'th', 'thead', 'tr', 'ul'
    ],
    ALLOWED_ATTR: ['href', 'title', 'class', 'align', 'start', 'type', 'checked', 'disabled']
  }

  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    // Open links outside the app window instead of navigating the webview away.
    if (node.tagName === 'A') {
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener noreferrer')
    }
    // GFM task lists are the only inputs Markdown emits; keep them inert.
    if (node.tagName === 'INPUT') {
      node.setAttribute('type', 'checkbox')
      node.setAttribute('disabled', '')
    }
    // Only highlight.js classes; app utility classes could overlay the UI.
    const classes = node.getAttribute('class')
    if (classes !== null) {
      const kept = classes.split(/\s+/).filter((name) => /^(hljs|language-)/.test(name))
      if (kept.length) node.setAttribute('class', kept.join(' '))
      else node.removeAttribute('class')
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
      },
      // Show images as links so nothing remote loads without a click.
      image({ href, text }: Tokens.Image) {
        return `<a href="${escapeHtml(href)}">${escapeHtml(text || href)}</a>`
      }
    }
  })
</script>

<script lang="ts">
  import { copyText } from '../clipboard'

  let { content, class: className = '' }: { content: string; class?: string } = $props()

  let root: HTMLDivElement | undefined = $state()

  const html = $derived(DOMPurify.sanitize(marked.parse(content) as string, SANITIZE))

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
