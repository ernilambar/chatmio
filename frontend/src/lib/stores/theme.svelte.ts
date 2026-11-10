const STORAGE_KEY = 'theme'

export type ThemeMode = 'system' | 'light' | 'dark'

function readStoredMode(): ThemeMode {
  try {
    const value = localStorage.getItem(STORAGE_KEY)
    if (value === 'light' || value === 'dark') return value
  } catch {
    // The cache is best-effort; the backend config stays authoritative.
  }
  return 'system'
}

class ThemeStore {
  mode = $state<ThemeMode>(readStoredMode())
  systemDark = $state(window.matchMedia('(prefers-color-scheme: dark)').matches)
  resolved = $derived(this.mode === 'system' ? (this.systemDark ? 'dark' : 'light') : this.mode)

  constructor() {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (event) => {
      this.systemDark = event.matches
    })

    $effect.root(() => {
      $effect(() => {
        document.documentElement.dataset.theme = this.resolved
      })
      $effect(() => {
        try {
          localStorage.setItem(STORAGE_KEY, this.mode)
        } catch {
          // Storage may be unavailable; the theme still applies this session.
        }
      })
    })
  }

}

export const theme = new ThemeStore()
