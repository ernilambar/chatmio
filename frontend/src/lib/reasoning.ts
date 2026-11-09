// splitThinking separates thinking text from a reply. Providers that stream
// reasoning in a separate field fill `reasoning`; local models often inline it
// as a leading <think>…</think> block instead, which may still be open while
// streaming.
export function splitThinking(
  content: string,
  reasoning = ''
): { reasoning: string; content: string } {
  const trimmed = content.trimStart()
  if (reasoning || !trimmed.startsWith('<think>')) return { reasoning, content }
  const body = trimmed.slice('<think>'.length)
  const end = body.indexOf('</think>')
  if (end < 0) return { reasoning: body.trim(), content: '' }
  return {
    reasoning: body.slice(0, end).trim(),
    content: body.slice(end + '</think>'.length).trimStart()
  }
}

export function formatTokens(count: number): string {
  if (count < 1000) return String(count)
  if (count < 1_000_000) return `${(count / 1000).toFixed(count < 10_000 ? 1 : 0)}k`
  return `${(count / 1_000_000).toFixed(1)}M`
}
