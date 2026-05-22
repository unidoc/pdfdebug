import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), wails("./bindings")],
  // Bind to IPv4 explicitly. Vite's default `host: 'localhost'` resolves to
  // ::1 (IPv6) on modern macOS, but Wails dev dials `tcp4 127.0.0.1`, so the
  // proxy returns 502/ECONNREFUSED and the WebView stays blank.
  server: { host: '127.0.0.1' },
  // Pre-bundle lucide-react so adding new icon imports does not trigger a
  // mid-session Vite restart (which kills the Wails dev-server proxy and
  // leaves the WebView blank). Empirically reproduced 2026-05-07.
  optimizeDeps: { include: ['lucide-react'] },
});
