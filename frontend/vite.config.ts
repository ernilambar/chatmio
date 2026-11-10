import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// wails.json is the single source of the app name; Vite exposes it as
// import.meta.env.VITE_APP_NAME and %VITE_APP_NAME% in index.html.
const wails = JSON.parse(readFileSync(new URL('../wails.json', import.meta.url), 'utf8'))
process.env.VITE_APP_NAME = wails.info.productName

export default defineConfig({
  plugins: [tailwindcss(), svelte()]
})
