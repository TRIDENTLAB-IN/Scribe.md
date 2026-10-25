import { mount } from 'svelte'
import './style.css'
import App from './App.svelte'

// Svelte 5 mounts components with mount(). The old `new App()` form is the
// legacy API and is unnecessary here.
const app = mount(App, {
  target: document.getElementById('app')
})

export default app
