# Scribe

A deliberately small Markdown desktop editor built with **Go + Wails v2**,
**Svelte 5**, and **CodeMirror 6**.

Scribe opens a Markdown file passed by the operating system ("Open With"),
lets you edit it with Markdown syntax highlighting, and saves it with
`Ctrl+S` / `Cmd+S`. The **Preview** button in the top bar toggles a live,
side-by-side Markdown preview.

The project is intentionally boring: file I/O lives in Go, UI state and
keyboard shortcuts live in Svelte, and there is no state-management library.

## How it works

```text
OS launch (os.Args[1]) ──┐
                         ├──> Go App.GetStartupFile() ──> Svelte editor
macOS Apple Event ───────┘                                   │
                                                             │ Ctrl/Cmd+S
                                                             v
                                              Go App.SaveFile(path, content)
                                                             │
                                                             v
                                                          disk
```

- `main.go` reads the first non-flag argument, makes it absolute, and hands it
  to the `App` struct. On macOS it also wires Wails' `OnFileOpen` callback,
  because macOS delivers "Open With" as an Apple Event rather than `os.Args`.
- `app.go` owns file reads and writes. `GetStartupFile` returns a small
  `StartupFile` struct (`path` + `content`) and an error. Wails v2 bindings
  support one return value plus `error`, so returning three values directly is
  not supported by the binding generator.
- `frontend/src/App.svelte` creates the CodeMirror editor, loads the startup
  file from Go, mirrors the document into Svelte state, and handles the save
  shortcut.
- The preview uses [`marked`](https://marked.js.org/) with raw HTML disabled
  and unsafe link schemes removed. It is only parsed while the split view is
  open, and the top bar's **Preview** button (or `aria-pressed`) shows its
  state.

## Project layout

```text
.
├── app.go                  # bound Go methods and file I/O
├── app_test.go             # small tests for args parsing and file I/O
├── main.go                 # Wails window setup + os.Args parsing
├── wails.json              # Wails build config and macOS file associations
├── build/                  # icons / platform packaging assets
└── frontend/
    ├── src/App.svelte      # editor, preview, status bar, shortcuts
    ├── src/main.js         # mounts the Svelte app
    ├── src/style.css       # global reset
    └── wailsjs/            # generated Go bindings (created by Wails)
```

## Requirements

- Go **1.22+**
- Node.js **20.19+** (or 22.12+)
- The Wails v2 CLI. This project is pinned to Wails **v2.12.0** so it builds
  with Go 1.22:

  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
  wails doctor
  ```

  `wails doctor` also tells you which platform libraries are missing.

Platform dependencies:

- **Linux:** `libgtk-3-dev` and `libwebkit2gtk-4.1-dev` (or the 4.0 package,
  depending on your distro).
- **Windows:** WebView2 runtime (already present on current Windows 10/11).
- **macOS:** Xcode command-line tools.

## Develop

```bash
# From the project root. Wails installs npm dependencies, starts Vite,
# generates frontend/wailsjs, and opens the desktop window.
wails dev

# Simulate "Open With" during development:
wails dev -appargs "/absolute/path/to/notes.md"
```

## Build

```bash
wails build
```

`wails.json` already sets `"build:tags": "webkit2_41"` because this machine
(and Ubuntu 24.04+) has WebKitGTK 4.1 rather than 4.0. The tag is harmless on
Windows and macOS. If your Linux distribution still provides
`webkit2gtk-4.0`, remove that line from `wails.json` (or override the tag) and
rebuild.

The binary is written to `build/bin/`:

- Linux: `build/bin/scribe`
- Windows: `build/bin/scribe.exe`
- macOS: `build/bin/Scribe.app`

The frontend is built with `npm run build` and embedded into the Go binary, so
the final artifact is self-contained (apart from the OS webview runtime).

## Test

```bash
go test ./...
```

The tests cover command-line argument parsing, reading a startup file, saving,
and the error returned for a missing file.

## OS "Open With" setup

Scribe reads the first non-flag command-line argument at launch. The examples
below point the operating system at the compiled binary and pass one file path
to it.

> **Note:** Windows and Linux launch a new Scribe process for each file. macOS
> reuses the running app and sends an Apple Event, which is why `main.go` wires
> `mac.Options.OnFileOpen` in addition to `os.Args`.

### Linux: `.desktop` file

Create `~/.local/share/applications/scribe.desktop` (replace the `Exec` path
with the absolute path to your binary):

```ini
[Desktop Entry]
Type=Application
Name=Scribe
Comment=Lean Markdown editor
Exec=/home/you/.local/bin/scribe %f
Icon=/home/you/.local/share/icons/scribe.png
Terminal=false
Categories=Utility;TextEditor;
MimeType=text/markdown;text/x-markdown;
StartupNotify=true
```

Then copy the app icon, refresh the desktop database, and make Scribe the
default handler:

```bash
mkdir -p ~/.local/share/icons
cp build/appicon.png ~/.local/share/icons/scribe.png

chmod +x ~/.local/share/applications/scribe.desktop
update-desktop-database ~/.local/share/applications
xdg-mime default scribe.desktop text/markdown
xdg-mime default scribe.desktop text/x-markdown
```

`%f` passes a single file path. Scribe currently handles one file per process,
so do not use `%F` (which can pass several files).

Verify the association:

```bash
xdg-mime query default text/markdown
```

On desktops that do not ship `update-desktop-database`, install
`desktop-file-utils`. You can also right-click a `.md` file and choose
**Open With → Scribe → Always**.

### Windows: registry association

Save the following as `scribe-md.reg`, replace the executable path, and
double-click it to import. This uses `HKEY_CURRENT_USER` so it does not need
administrator rights and only affects the current user.

```reg
Windows Registry Editor Version 5.00

[HKEY_CURRENT_USER\Software\Classes\.md]
@="Scribe.md"

[HKEY_CURRENT_USER\Software\Classes\.markdown]
@="Scribe.md"

[HKEY_CURRENT_USER\Software\Classes\Scribe.md]
@="Markdown Document"

[HKEY_CURRENT_USER\Software\Classes\Scribe.md\DefaultIcon]
@="\"C:\\Program Files\\Scribe\\scribe.exe\",0"

[HKEY_CURRENT_USER\Software\Classes\Scribe.md\shell\open\command]
@="\"C:\\Program Files\\Scribe\\scribe.exe\" \"%1\""
```

Notes:

- Keep the escaped quotes around the executable path and around `%1`. Without
  them, paths containing spaces break.
- Use `HKEY_CLASSES_ROOT` instead of `HKEY_CURRENT_USER` only if you need the
  association for every user; that requires administrator rights.
- Windows 10/11 may keep a cached choice. If the association does not appear,
  open **Settings → Apps → Default apps → Choose defaults by file type**, or
  right-click a `.md` file and use **Open with → Choose another app → Always**.
- To test the command directly, run:
  `"C:\Program Files\Scribe\scribe.exe" "C:\docs\notes.md"`

### macOS: document types in the app bundle

macOS does not use `os.Args` for Finder "Open With". Scribe handles this in two
parts:

1. `wails.json` declares the Markdown document types. Wails writes these into
   the generated `Info.plist` when packaging `Scribe.app`:

   ```json
   "fileAssociations": [
     { "ext": "md", "name": "Markdown Document", "role": "Editor", "iconName": "appicon" },
     { "ext": "markdown", "name": "Markdown Document", "role": "Editor", "iconName": "appicon" }
   ]
   ```

2. `main.go` passes `app.openFileFromOS` to `mac.Options.OnFileOpen`. Wails
   receives the Apple Event and emits a `file:opened` event to Svelte; the
   frontend then reloads the file through `GetStartupFile`.

After `wails build`, move `build/bin/Scribe.app` to `/Applications`, then
right-click a `.md` file and choose **Open With → Scribe → Always Open**.

If LaunchServices has cached an older association, force a refresh:

```bash
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister \
  -f /Applications/Scribe.app
```

If you need extra `Info.plist` keys, edit `build/darwin/Info.plist` (Wails
creates it from its template on the first build) and rebuild. Keep the
`CFBundleDocumentTypes` entries in sync with `wails.json`.

## Design notes and limitations

- **One file at a time.** There is no file picker or "Save As" yet. Launch the
  app with a file, or open one through the OS. Saving with no path shows a
  clear error instead of writing somewhere unexpected.
- **No unsaved-change prompt on close.** The status bar shows
  `Unsaved changes`, but closing the window does not block. Add a Wails
  `OnBeforeClose` dialog later if you need that guard.
- **No state manager.** CodeMirror owns the document while editing; Svelte
  mirrors it for the status bar and save action. `$state` / `$derived` are
  enough for an app this size.
- **Wails binding shape.** `GetStartupFile` returns `(StartupFile, error)`
  rather than `(path, content, error)` because Wails v2's binding layer only
  handles zero, one, or one-plus-error return values. The generated JavaScript
  would otherwise resolve to `null`.

## License

No license has been chosen yet. Add one before distributing the app.
