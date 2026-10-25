<script>
  import { onMount } from 'svelte'
  import { Marked } from 'marked'

  // CodeMirror is assembled from a few official packages instead of a wrapper
  // component. The editor lifecycle then lives in exactly one place and there
  // is no hidden state to debug.
  import { EditorState } from '@codemirror/state'
  import {
    EditorView,
    drawSelection,
    dropCursor,
    highlightActiveLineGutter,
    highlightSpecialChars,
    keymap,
    lineNumbers
  } from '@codemirror/view'
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
  import { markdown } from '@codemirror/lang-markdown'
  import { oneDark } from '@codemirror/theme-one-dark'

  // Wails generates these files from the bound Go methods. Importing the
  // generated functions keeps the frontend contract in sync with app.go and
  // avoids scattering `window.go...` calls through the UI code.
  import { GetStartupFile, SaveFile } from '../wailsjs/go/main/App.js'
  import { BrowserOpenURL, EventsOff, EventsOn } from '../wailsjs/runtime/runtime.js'

  // Non-reactive DOM/editor handles. They are deliberately plain variables
  // because the template never renders them.
  let editorHost
  let view

  // Svelte 5 runes are the native reactivity here: no stores, no state
  // manager. `content` is the mirror of the CodeMirror document.
  let content = $state('')
  let savedContent = $state('')
  let filePath = $state('')
  let saving = $state(false)
  let loadError = $state('')
  let saveError = $state('')
  let previewOpen = $state(false)

  // Everything the status bar needs is derived from the state above.
  let dirty = $derived(content !== savedContent)
  let fileName = $derived(filePath ? filePath.split(/[\\/]/).pop() : 'No file open')
  let errorMessage = $derived(loadError || saveError)

  // The preview is only rendered while the split view is open. Keeping the
  // parse inside a $derived means Svelte caches it until `content` changes.
  let renderedHtml = $derived(previewOpen ? renderMarkdown(content) : '')

  // Marked is the only extra runtime dependency. Raw HTML is dropped and
  // unsafe URL schemes are removed so a Markdown file cannot inject script
  // into the Wails webview (which has access to the bound Go methods).
  const md = new Marked({
    gfm: true,
    breaks: false,
    renderer: {
      html() {
        return ''
      }
    },
    walkTokens(token) {
      if (
        (token.type === 'link' || token.type === 'image') &&
        !isSafeHref(token.href)
      ) {
        token.href = ''
      }
    }
  })
  let status = $derived(
    loadError
      ? 'Open failed'
      : saveError
        ? 'Save failed'
        : saving
          ? 'Saving…'
          : !filePath
            ? 'No file open'
            : dirty
              ? 'Unsaved changes'
              : 'Saved'
  )

  // A small theme layer on top of oneDark. It only controls spacing and
  // typography; colours come from the standard theme.
  const scribeTheme = EditorView.theme(
    {
      '&': { height: '100%', fontSize: '15px' },
      '.cm-scroller': {
        fontFamily:
          'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
        lineHeight: '1.65'
      },
      '.cm-content': { padding: '28px 0' },
      '.cm-line': { padding: '0 32px' },
      '.cm-gutters': { paddingLeft: '12px', border: 'none' },
      '&.cm-focused': { outline: 'none' }
    },
    { dark: true }
  )

  onMount(() => {
    view = new EditorView({
      state: EditorState.create({
        doc: '',
        extensions: [
          lineNumbers(),
          highlightActiveLineGutter(),
          highlightSpecialChars(),
          history(),
          drawSelection(),
          dropCursor(),
          EditorState.allowMultipleSelections.of(true),
          EditorState.tabSize.of(2),
          markdown(),
          oneDark,
          scribeTheme,
          EditorView.lineWrapping,
          keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),

          // CodeMirror owns the document while the user types. The listener
          // copies it into Svelte state so the status bar can react.
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              content = update.state.doc.toString()
            }
          })
        ]
      }),
      parent: editorHost
    })

    // Load the command-line / macOS "Open With" file once the editor exists.
    void loadStartupFile()

    // macOS can send another file while the window is already open. The Go
    // side emits only the path; we re-read it through the same bound method.
    EventsOn('file:opened', () => {
      void loadStartupFile()
    })

    // Ctrl+S on Windows/Linux, Cmd+S on macOS. A window-level listener means
    // the shortcut still works when focus is outside the editor (for example
    // after clicking the status bar).
    const onKeyDown = (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
        event.preventDefault()
        void saveFile()
      }
    }
    window.addEventListener('keydown', onKeyDown)

    // Svelte runs this cleanup when the component is destroyed. The root
    // component normally lives for the whole process, but keeping cleanup here
    // makes the component safe to reuse and avoids duplicate listeners during
    // hot reload.
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      EventsOff('file:opened')
      view?.destroy()
      view = undefined
    }
  })

  /**
   * Read the file selected by the Go backend. Called on mount and whenever
   * macOS reports that another document was opened.
   */
  async function loadStartupFile() {
    loadError = ''
    saveError = ''

    try {
      const file = await GetStartupFile()
      const text = file.content ?? ''

      filePath = file.path ?? ''

      // Mark the incoming text as saved *before* dispatching it into the
      // editor. Otherwise the update listener briefly sees a dirty buffer and
      // the status bar flashes "Unsaved changes" during every load.
      savedContent = text
      setEditorContent(text)
      view?.focus()
    } catch (err) {
      loadError = errorText(err)
    }
  }

  /**
   * Replace the whole document without recreating the editor. Dispatching a
   * change keeps undo history and selection handling inside CodeMirror.
   */
  function setEditorContent(text) {
    if (!view) {
      content = text
      return
    }

    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: text }
    })
  }

  /** Toggle the split editor + preview layout. */
  function togglePreview() {
    previewOpen = !previewOpen

    // The editor gets narrower when the preview appears. Ask CodeMirror to
    // re-measure after the browser has applied the new grid columns.
    requestAnimationFrame(() => view?.requestMeasure())
  }

  /** Convert Markdown to HTML for the preview pane. */
  function renderMarkdown(source) {
    return md.parse(source)
  }

  /**
   * Allow only URLs that are safe in a preview. Relative links resolve against
   * a fake https base, so `#anchor` and `./notes.md` still work.
   */
  function isSafeHref(href) {
    if (!href) return false

    try {
      const url = new URL(href, 'https://scribe.invalid/')
      return (
        url.protocol === 'http:' ||
        url.protocol === 'https:' ||
        url.protocol === 'mailto:'
      )
    } catch {
      return false
    }
  }

  /**
   * Keep link clicks from navigating the Wails webview away from the editor.
   * In-page anchors are allowed to scroll the preview; everything else is
   * handed to the operating system's default browser.
   */
  function onPreviewClick(event) {
    const link = event.target.closest?.('a')
    if (!link) return

    const href = link.getAttribute('href') ?? ''
    if (href.startsWith('#')) return

    event.preventDefault()

    try {
      const url = new URL(href, window.location.href)
      if (
        url.protocol === 'http:' ||
        url.protocol === 'https:' ||
        url.protocol === 'mailto:'
      ) {
        BrowserOpenURL(url.href)
      }
    } catch {
      // Ignore links that cannot be parsed.
    }
  }

  /**
   * Persist the current buffer through the bound Go method. The UI never
   * touches the filesystem directly.
   */
  async function saveFile() {
    if (saving) return

    if (!filePath) {
      saveError = 'No file is open. Launch Scribe with a Markdown file.'
      return
    }

    saving = true
    saveError = ''

    // Snapshot the content at the moment the save starts. The user may keep
    // typing while the disk write is in flight; in that case the buffer should
    // remain marked as unsaved.
    const snapshot = content

    try {
      await SaveFile(filePath, snapshot)
      savedContent = snapshot
    } catch (err) {
      saveError = errorText(err)
    } finally {
      saving = false
    }
  }

  // Wails rejects with the Go error string. Accept Error objects too so this
  // helper also behaves sensibly during browser-only development.
  function errorText(err) {
    return err?.message ?? String(err)
  }
</script>

<div class="app">
  <header class="bar">
    <div class="file" title={filePath || 'No file open'}>
      <span class="dot" class:active={filePath}></span>
      <span class="name">{fileName}</span>
    </div>

    <div class="hint">Ctrl/Cmd+S to save</div>

    <button
      class="toggle"
      class:active={previewOpen}
      type="button"
      onclick={togglePreview}
      aria-pressed={previewOpen}
      title={previewOpen ? 'Hide preview' : 'Show preview'}
    >
      Preview
    </button>

    <div
      class="status"
      class:error={errorMessage}
      class:dirty={dirty}
      aria-live="polite"
    >
      {status}
    </div>
  </header>

  {#if errorMessage}
    <div class="error" role="alert">{errorMessage}</div>
  {/if}

  <!-- The workspace is a grid so the preview can share the height with the
       editor without any manual resizing logic. -->
  <div class="workspace" class:split={previewOpen}>
    <!-- CodeMirror mounts into this element and fills its grid cell. -->
    <main class="editor" bind:this={editorHost}></main>

    {#if previewOpen}
      <aside class="preview" aria-label="Markdown preview">
        <!-- Marked output is already limited to safe links and no raw HTML. -->
        <!-- The article is a document surface, not a control; the click handler
             only redirects link navigation. -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <article class="markdown" onclick={onPreviewClick}>
          {@html renderedHtml}
        </article>
      </aside>
    {/if}
  </div>
</div>

<style>
  .app {
    height: 100%;
    display: flex;
    flex-direction: column;
    background: #16181d;
  }

  .bar {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    gap: 16px;
    height: 38px;
    padding: 0 16px;
    border-bottom: 1px solid #2a2e37;
    background: #1b1e25;
    font-size: 12px;
    color: #a9afba;
    user-select: none;
  }

  .file {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    color: #e3e6eb;
    font-weight: 500;
  }

  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #555b66;
    flex: 0 0 auto;
  }

  .dot.active {
    background: #6fb3ff;
  }

  .hint {
    margin-left: auto;
    color: #6f7683;
    white-space: nowrap;
  }

  .toggle {
    flex: 0 0 auto;
    height: 24px;
    padding: 0 10px;
    border: 1px solid #343a46;
    border-radius: 6px;
    background: #22262f;
    color: #a9afba;
    font: inherit;
    font-size: 11px;
    cursor: pointer;
  }

  .toggle:hover {
    background: #2a2f3a;
    color: #e3e6eb;
  }

  .toggle.active {
    border-color: #3f6a97;
    background: #2d4a6b;
    color: #dceaff;
  }

  .toggle:focus-visible {
    outline: 2px solid #6fb3ff;
    outline-offset: 1px;
  }

  .status {
    min-width: 118px;
    text-align: right;
    color: #7fd18b;
    white-space: nowrap;
  }

  .status.dirty {
    color: #e2b341;
  }

  .status.error {
    color: #ff7a7a;
  }

  .error {
    flex: 0 0 auto;
    padding: 8px 16px;
    border-bottom: 1px solid #4a2626;
    background: #2a1719;
    color: #ffb3b3;
    font-size: 12px;
    white-space: pre-wrap;
  }

  .workspace {
    flex: 1 1 auto;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }

  .workspace.split {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  .editor {
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }

  .preview {
    min-width: 0;
    min-height: 0;
    overflow: auto;
    border-left: 1px solid #2a2e37;
    background: #1a1d23;
  }

  .markdown {
    max-width: 820px;
    padding: 28px 32px 64px;
    color: #d6dae1;
    font-size: 15px;
    line-height: 1.7;
    overflow-wrap: break-word;
  }

  /* Marked output is injected with {@html}, so its elements do not receive
     Svelte's scoping class. Every selector therefore needs :global(). */
  .markdown :global(:first-child) {
    margin-top: 0;
  }

  .markdown :global(h1),
  .markdown :global(h2),
  .markdown :global(h3),
  .markdown :global(h4),
  .markdown :global(h5),
  .markdown :global(h6) {
    margin: 1.6em 0 0.6em;
    color: #f0f2f5;
    font-weight: 600;
    line-height: 1.25;
  }

  .markdown :global(h1) {
    padding-bottom: 0.3em;
    border-bottom: 1px solid #2f3542;
    font-size: 1.9em;
  }

  .markdown :global(h2) {
    padding-bottom: 0.25em;
    border-bottom: 1px solid #2a2f3a;
    font-size: 1.45em;
  }

  .markdown :global(h3) {
    font-size: 1.2em;
  }

  .markdown :global(h4) {
    font-size: 1.05em;
  }

  .markdown :global(p),
  .markdown :global(ul),
  .markdown :global(ol),
  .markdown :global(blockquote),
  .markdown :global(pre),
  .markdown :global(table) {
    margin: 0 0 1em;
  }

  .markdown :global(ul),
  .markdown :global(ol) {
    padding-left: 1.5em;
  }

  .markdown :global(li + li) {
    margin-top: 0.25em;
  }

  .markdown :global(a) {
    color: #6fb3ff;
    text-decoration: none;
  }

  .markdown :global(a:hover) {
    text-decoration: underline;
  }

  .markdown :global(code) {
    padding: 0.15em 0.35em;
    border-radius: 4px;
    background: #262b35;
    font-family:
      ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
    font-size: 0.9em;
  }

  .markdown :global(pre) {
    padding: 14px 16px;
    border: 1px solid #262b35;
    border-radius: 8px;
    background: #12141a;
    overflow: auto;
  }

  .markdown :global(pre code) {
    padding: 0;
    background: transparent;
    font-size: 0.88em;
    line-height: 1.6;
  }

  .markdown :global(blockquote) {
    padding: 0.2em 0 0.2em 1em;
    border-left: 3px solid #3a4150;
    color: #9aa2af;
  }

  .markdown :global(table) {
    border-collapse: collapse;
    display: block;
    overflow: auto;
  }

  .markdown :global(th),
  .markdown :global(td) {
    padding: 6px 12px;
    border: 1px solid #2f3542;
    text-align: left;
  }

  .markdown :global(th) {
    background: #22262f;
    color: #e3e6eb;
  }

  .markdown :global(img) {
    max-width: 100%;
  }

  .markdown :global(hr) {
    margin: 2em 0;
    border: none;
    border-top: 1px solid #2f3542;
  }

  /* CodeMirror's own elements are outside Svelte's scoping, so these rules
     need :global(). */
  :global(.cm-editor) {
    height: 100%;
  }

  :global(.cm-scroller) {
    overflow: auto;
  }

  @media (max-width: 720px) {
    .hint {
      display: none;
    }

    /* On narrow windows the panes stack instead of becoming unusably thin. */
    .workspace.split {
      grid-template-columns: minmax(0, 1fr);
      grid-template-rows: minmax(0, 1fr) minmax(0, 1fr);
    }

    .preview {
      border-top: 1px solid #2a2e37;
      border-left: none;
    }

    .markdown {
      padding: 20px 20px 48px;
    }
  }
</style>
