/**
 * @file Application entry point. Mounts the React root into the DOM.
 */
import React from 'react'
import ReactDOM from 'react-dom/client'
import './style.css'
import App from './App'

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('Root element #root not found in document');
}
ReactDOM.createRoot(rootEl).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
