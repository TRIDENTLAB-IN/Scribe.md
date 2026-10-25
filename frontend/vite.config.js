import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Vite serves the Svelte app in development and produces the static files that
// main.go embeds for production builds.
export default defineConfig({
  plugins: [svelte()]
})
