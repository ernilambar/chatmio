const ATTRIBUTES: Array<[string, string]> = [
  ['spellcheck', 'false'],
  ['autocorrect', 'off'],
  ['autocapitalize', 'off'],
  ['autocomplete', 'off']
]

function applyToElement(element: Element): void {
  if (!(element instanceof HTMLInputElement) && !(element instanceof HTMLTextAreaElement)) {
    return
  }
  for (const [name, value] of ATTRIBUTES) {
    element.setAttribute(name, value)
  }
}

function applyWithin(root: ParentNode): void {
  if (root instanceof Element) applyToElement(root)
  for (const element of root.querySelectorAll('input, textarea')) {
    applyToElement(element)
  }
}

// Remove macOS autocompletion, autocorrection, and autofill from every text
// field, including fields rendered later.
export function disableTextAssist(): () => void {
  applyWithin(document)

  const observer = new MutationObserver((records) => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType === Node.ELEMENT_NODE) applyWithin(node as Element)
      }
    }
  })
  observer.observe(document.documentElement, { childList: true, subtree: true })

  return () => observer.disconnect()
}
