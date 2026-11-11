const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])'
].join(',')

// trapFocus keeps keyboard focus inside a modal and restores it on close.
export function trapFocus(node: HTMLElement): { destroy(): void } {
  const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null
  if (!node.hasAttribute('tabindex')) node.tabIndex = -1
  node.focus()

  function focusables(): HTMLElement[] {
    return Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
      (element) => element.getClientRects().length > 0
    )
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Tab') return
    const items = focusables()
    if (items.length === 0) {
      event.preventDefault()
      node.focus()
      return
    }
    const first = items[0]
    const last = items[items.length - 1]
    if (event.shiftKey && (document.activeElement === first || document.activeElement === node)) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  node.addEventListener('keydown', handleKeydown)
  return {
    destroy() {
      node.removeEventListener('keydown', handleKeydown)
      if (previous?.isConnected) previous.focus()
    }
  }
}
