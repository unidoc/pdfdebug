# GUI automation coverage

What the Wails MCP server can reach in `bin/pdfdebug-mcp`, from a pass with `go run ./tools/guictl` on macOS 26.6 (2026-11-10, Wails v3.0.0). Every feature reachable from the main window, the menus and the dialogs has a row.

How it is reached: click (`mouse_click` / `mouse_drag` on a selector or point), key (`keyboard_press` / `keyboard_type`), second launch (`guictl open`), stub (`guictl pick` then the control that opens the file dialog), event (`emit_event` with the event the native menu item sends), js (`js_eval`).

Results: works; works with a workaround (named in the row); cannot be automated (cause named: native UI, `isTrusted`, timing, missing selector).

Two things hold across rows. A synthetic click on a `<select>` opens WebKit's native popup, which MCP cannot close; while it, or any native menu, is open the app does not take second-launch files, so `guictl open` and the suite's second launches stall until a real key press or `guictl stop`. And synthetic keys reach the page's own `keydown` handlers but never native menu accelerators, so a shortcut works only where the frontend handles it too.

`mouse_click` and the other mouse tools are not reliable for unattended runs. Their in-page code animates the cursor and waits on `setTimeout` and `requestAnimationFrame` (about four waits per click), and WebKit throttles both while the window is not visible on screen. The click then lands late or not at all within the 30 s eval timeout and the call fails with `context deadline exceeded`, while `dom_query` and `js_eval`, which wait on no timers, keep answering. With rAF disabled and timers clamped to 10 s in the page, `mouse_click` timed out at 30 s every time. The suite therefore clicks with one synchronous `js_eval` that fires the same pointerdown, mousedown, pointerup, mouseup and click sequence on the element under the point (`click` and `clickAt` in `launcher_test.go`). `guictl call mouse_click` is fine for hand-driven passes with the window in front.

## Main window

| Feature | How it is reached | Result | Suite | Limitation |
|---|---|---|---|---|
| Empty state, Open File... button | stub, click `open-file-button` | works (two queued files open as two tabs) | - | - |
| Drop zone (drag a PDF onto the empty state or a tab) | - | cannot be automated | - | native UI: drop comes from the OS (`WindowFilesDropped`), no seam |
| Open files into a running instance | second launch | works | `TestOpenFixturesThroughASecondLaunch` | - |
| Open a file from the command line on cold start | argv | Linux: not driven by hand, the suite flow runs in Linux CI; macOS opens nothing from argv | `TestOpenFixtureFromTheCommandLine` (Linux only) | - |
| Open error banner (encrypted, malformed) and dismiss | second launch, click `error-banner-dismiss` | works | - | - |
| Batch open dialog (progress, Cancel) | second launch with several files | cannot be automated | - | timing: committed fixtures open in milliseconds and the dialog never renders long enough to observe |
| Document tab bar: switch tabs | click `[data-testid="tab-list"] [role="tab"]` | works | `TestSwitchDocuments` | - |
| Document tab close button | click `tab-close-<id>` | works | - | - |
| Close document with Cmd/Ctrl+W | key | works (frontend handler) | - | - |
| Tab disk-change dot and tooltip | rewrite an opened copy on disk, poll `tab-disk-dot-<id>` | works (10-20 s while the window is unfocused) | - | - |
| Reload toast ("reloaded - view reset") | after a reload, read `[role="status"]` | works | - | - |
| Left rail: Structure, Pages, Images | click `[aria-label="Navigators"] [role="tab"][aria-label="..."]` | works | - | no `data-testid`; role and aria-label are stable |
| Panel sash resize | `mouse_drag` on `sash` | works | - | - |
| Window size and position persistence | - | cannot be automated | - | native UI: geometry comes from OS window moves; `window_control set_size` exists but restore across launches was not exercised |
| Structure tree: expand a row | click the row's arrow by coordinates | works | `TestExpandTreeNodeShowsItsChildren` | - |
| Structure tree: select a row, detail updates | click `tree-node[data-node-id]` | works | - | - |
| Structure tree: keyboard navigation | key (ArrowDown, ArrowRight) after selecting a row | works | - | - |
| Tree row context menus (Copy object reference, Copy path), structure tree, Pages, Images | - | cannot be automated | - | native UI: Wails native context menu. `emit_event row-menu:action` is rejected because the event is registered with the Go type `RowMenuAction`, and after a synthetic `contextmenu` event second launches stopped arriving until `guictl stop`, as with an open native popup |
| Object Source panel | select a row, read `object-source-body` | works | - | - |
| Referenced by (reverse refs) | select a row, read `reverse-refs-section` | works | - | - |
| Reference links in the detail pane (`7 0 R`) | click `span[role="button"]` in `detail-pane-object` | works | - | no `data-testid`; role and class used |
| Navigate back / forward buttons | click `[aria-label="Navigate back"]` | works | - | - |
| Pages navigator | left rail Pages, rows `pages-panel tree-node` | works | - | - |
| Go to Page (Cmd/Ctrl+G, Navigate > Go to Page...) | key or event `navigate:goToPage` | works (rail switches to Pages, jump field focused) | - | - |
| Images navigator and image preview | left rail Images, click `image-row` | works (`image-preview-img` decodes) | - | - |
| Image save | click `image-preview-save` | cannot be automated | - | native UI: save dialog |
| Large-image decode consent (`image-preview-consent`) | - | not exercised | - | no committed fixture is large enough to ask |
| Command palette (Cmd/Ctrl+K, Navigate > Find Object...) | key, `keyboard_type` into `command-palette-input`, Enter | works (search, jump to the match) | - | - |
| Detail tabs: Object, XREF, Plain Text, Embedded, Metadata, Validate, Diff, Signatures | click `detail-tab-<name>` | works | `TestSwitchDetailTabs` (XREF, Plain Text) | - |
| Content stream viewer, Formatted/Raw | follow `/Contents`, click `view-mode-raw` | works | - | - |
| Content stream operator tooltip | `mouse_move` to an operator's coordinates (no `data-testid` on the token), read `operator-tooltip` | works (`Tf` shows "Set text font and size") | - | - |
| Font preview | palette `/Font`, read `font-preview` | works | - | - |
| Font roster (a page's `/Resources /Font` map) | expand the page's Resources row, click its Font row, read `font-roster-preview` | works (`fonts-mixed.pdf`: four `font-roster-row` entries) | - | - |
| XREF table: row jumps to the object | click `xref-row-objnum-<n>` | works | - | - |
| XREF find bar | focus `xref-table-container` with js, key Cmd/Ctrl+F, type | works with a workaround | - | clicking the table to focus it lands on a row and jumps to the object; focus with `js_eval` first |
| Plain Text find bar (count, next, close) | click `plain-text-scroll`, key Cmd/Ctrl+F, `keyboard_type`, click `plain-text-find-next` | works | - | - |
| Object find bar (`object-find-*`) | click `detail-pane-object`, key Cmd/Ctrl+F, `keyboard_type`, key Escape | works (count, active match, Escape closes) | - | - |
| Embedded files list, detail, text preview, Reveal in tree | click `embedded-row-<ref>`, `embedded-reveal-in-tree` | works | - | - |
| Embedded file save | click `embedded-save` | cannot be automated | - | native UI: save dialog |
| Metadata (Info, XMP) | `detail-tab-metadata` | works | - | - |
| Validate: profile select | js: set the `<select>` value with the native setter and dispatch `change` | works with a workaround | - | native UI: a click opens WebKit's native popup, which MCP cannot close |
| Validate: Run checks, summary, Rules checked toggle | click `validate-run`, `validate-rules-toggle` | works | - | - |
| Signatures: card, certificate chain toggle | click `detail-tab-signatures`, `signature-cert-chain` | works | `TestSignaturesTabShowsTheCertificateChain` | - |
| Signatures: Reveal in tree | click `signature-reveal-in-tree` | works | - | - |
| Signatures: tab issue dot and problem notice | open `signed-badcontents.pdf`, read `tab-issue-signatures` and `signature-problem` | works (red dot titled "1 signature could not be decomposed", notice at the top of the card) | - | - |
| Compare: Pick file | stub, click `diff-pick-file` | works | `TestCompareThroughTheDialogStub` | the header does not name a picked file, so the suite checks the summary's page count; backlog `diff-view-names-picked-file` |
| Diff view: summary, trees, prev/next change, close | click `diff-next-change`, `diff-close` | works | - | - |
| Stale note: Document updated, Reload, Compare | rewrite an opened copy, click the note's buttons by text | works | - | no `data-testid` on the two buttons; select `[data-testid="stale-note"] button` by text |
| Compare with File on Disk, Reload in the Diff header | stale note Compare or event `document:compare-active`, click `diff-reload` | works | - | - |
| Reload from Disk (F5, Cmd/Ctrl+R) | key F5 or event `document:reload-active` | works | - | - |
| Disk change announcer (screen reader live region) | read `disk-change-announcer` | present; its announcement text was not checked | - | - |
| Panel error fallback (Try again) | - | cannot be automated | - | no trigger: the fallback renders only after a component throws, and no committed fixture makes one throw; Vitest covers it |

## Menus

Native menu items cannot be clicked over MCP (native UI). Where the item only emits an event to the page, `emit_event` with that event drives the same frontend path; the menu item itself, its enabled state and its accelerator stay manual.

| Feature | How it is reached | Result | Suite | Limitation |
|---|---|---|---|---|
| App menu (About, Services, Hide, Quit) on macOS | - | cannot be automated | - | native UI |
| File > Open... (Cmd/Ctrl+O) | - | cannot be automated | - | native UI: the handler builds its own native dialog in Go (`main.go`), not `PDFService.OpenFileDialog`, so the stub does not apply; the accelerator is native |
| File > Open Recent, Clear Menu | - | cannot be automated | - | native UI. The list itself is checked: `.guictl/config/pdfdebug/recent.json` records every opened fixture |
| File > Close Document | event `document:close-active` | works with a workaround | - | native UI for the item |
| File > Reload from Disk | event `document:reload-active` | works with a workaround | - | native UI for the item |
| File > Compare with File on Disk | event `document:compare-active` | works with a workaround | - | native UI for the item |
| File > Quit (Linux, Windows) | - | cannot be automated | - | native UI; `guictl stop` ends the app with SIGTERM instead |
| Edit menu (undo, cut, copy, paste, select all) | - | cannot be automated | - | native UI (role menu) |
| Navigate > Back, Forward | event `navigate:back`, `navigate:forward`; the pane's buttons | works with a workaround | - | native UI for the item; Cmd/Ctrl+[ and ] are native accelerators |
| Navigate > Go to Page... | event `navigate:goToPage` or key Cmd/Ctrl+G | works with a workaround | - | native UI for the item |
| Navigate > Find Object... | event `palette:open` or key Cmd/Ctrl+K | works with a workaround | - | native UI for the item |
| Navigate > Next Document, Previous Document | event `tab:next`, `tab:prev` | works with a workaround | - | native UI for the item; Cmd/Ctrl+Left/Right are native accelerators |
| Help > Check for Updates... | event `update:check-requested` | works with a workaround (dialog says up to date) | - | native UI for the item |
| Help > About (Linux, Windows) and the macOS app-menu About | event `about:open` with the version as data | works with a workaround | - | native UI for the item |
| Install Command Line Tool (macOS app menu) | - | cannot be automated | - | native UI: menu item and its native confirmation alert |

## Dialogs

| Feature | How it is reached | Result | Suite | Limitation |
|---|---|---|---|---|
| About dialog, close with Escape | event `about:open`, key Escape | works | - | - |
| Update dialog: up to date, auto-check toggle, Close | event `update:check-requested`, click `update-close-button` | works | - | - |
| Update dialog: release notes, download, progress, verify, keep, skip, release page | - | cannot be automated | - | the agent build is version `dev`, which skips the network check, so no newer release is ever offered |
| Update badge | - | cannot be automated | - | same as above |
| Native Open dialog (File > Open, empty state with nothing queued, Compare with nothing queued) | - | cannot be automated | - | native UI |
| Native Save dialogs (image save, embedded file save) | - | cannot be automated | - | native UI |
| Install CLI confirmation and result alerts | - | cannot be automated | - | native UI |
| Startup splash | - | cannot be automated | - | timing: it closes as soon as the main window is ready, before the MCP client can look at it |
| Batch open dialog | see Main window | cannot be automated | - | timing |

## Not covered by this pass

Windows: `build:mcp` and `guictl` are macOS and Linux only. Linux was not driven by hand; the suite runs there in CI under Xvfb.
