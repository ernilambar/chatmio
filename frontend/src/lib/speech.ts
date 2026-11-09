export const speechSupported = typeof window !== 'undefined' && 'speechSynthesis' in window

// Strip markdown so the synthesizer reads prose, not syntax.
function toPlainText(markdown: string): string {
  return markdown
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`([^`]*)`/g, '$1')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/<[^>]+>/g, ' ')
    .replace(/^\s{0,3}#{1,6}\s+/gm, '')
    .replace(/^\s*>\s?/gm, '')
    .replace(/^\s*([-*+]|\d+\.)\s+/gm, '')
    .replace(/^\s*\|?[\s:|-]+\|[\s:|-]*$/gm, ' ')
    .replace(/\|/g, ' ')
    .replace(/(\*\*|__|\*|_|~~)(\S[\s\S]*?)\1/g, '$2')
    .replace(/\s+/g, ' ')
    .trim()
}

export function speak(markdown: string, onend: () => void): void {
  speechSynthesis.cancel()
  const text = toPlainText(markdown)
  if (!text) {
    onend()
    return
  }
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.onend = onend
  utterance.onerror = onend
  speechSynthesis.speak(utterance)
}

export function stopSpeaking(): void {
  if (speechSupported) speechSynthesis.cancel()
}
