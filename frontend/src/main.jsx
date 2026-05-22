/**
 * @file Application entry point. Mounts the React root into the DOM.
 */
import React from 'react'
import ReactDOM from 'react-dom/client'
import { Events } from '@wailsio/runtime'
import './style.css'
import App from './App'

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('Root element #root not found in document');
}

// Story 9.13 AC5: the main window's #root starts at opacity 0 (set in
// index.html) so the splash crossfade is not defeated by an opaque first
// paint. The .splash-dismissed class flips opacity to 1 via the
// 200ms CSS transition. The class is added either when Go emits
// splash:dismissed at the end of the splash scheduler's dismissal
// handler, OR after a 1500ms fallback guard so a missed/dropped event
// never leaves the user staring at a blank window. The fallback covers
// the dev-mode (Vite + `wails3 dev`) case where the Go-side splash code
// may not be wired (no splash window at all) and the initial opacity-0
// would otherwise persist.
const SPLASH_DISMISS_FALLBACK_MS = 1500;
const rootElForReveal = rootEl;
function revealRoot() {
  if (!rootElForReveal.classList.contains('splash-dismissed')) {
    rootElForReveal.classList.add('splash-dismissed');
  }
}
try {
  const off = Events.On('splash:dismissed', () => {
    revealRoot();
    if (typeof off === 'function') off();
  });
  // Fallback: reveal regardless after the guard window so a missed
  // event does not strand the UI in opacity-0.
  setTimeout(revealRoot, SPLASH_DISMISS_FALLBACK_MS);
} catch (_) {
  // Wails runtime unavailable (e.g. unit tests under jsdom). Reveal
  // immediately so the component tree is visible.
  revealRoot();
}

ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
