import '@testing-library/jest-dom/vitest';

// @wailsio/runtime auto-registers a document dragenter listener that reads
// event.dataTransfer.types.includes('Files'). jsdom's synthetic drag events
// (from @testing-library/react fireEvent) build dataTransfer without a real
// types DOMStringList, so the listener throws "Cannot read properties of
// undefined (reading 'includes')" during test teardown. Swallow these.
interface NodeProcessLike {
  on: (event: 'uncaughtException', handler: (err: unknown) => void) => void;
}
const proc: NodeProcessLike | undefined = (globalThis as { process?: NodeProcessLike }).process;
if (proc && typeof proc.on === 'function') {
  proc.on('uncaughtException', (err: unknown) => {
    const msg = err instanceof Error ? err.message : String(err);
    const stack = err instanceof Error ? err.stack ?? '' : '';
    if (msg.includes("reading 'includes'") && stack.includes('@wailsio/runtime')) {
      return; // ignore
    }
    throw err;
  });
}

// Node 25+ provides a broken globalThis.localStorage when --localstorage-file
// is not set. Vitest's jsdom environment may not override it. Provide a
// standards-compliant in-memory implementation for tests.
//
// Node 26+ leaves window.localStorage as `undefined`, so the property access
// itself throws before reading .setItem. Guard with an explicit existence
// check before touching the slot.
if (
  typeof window !== 'undefined' &&
  (!window.localStorage || typeof window.localStorage.setItem !== 'function')
) {
  const store = new Map<string, string>();
  const storage: Storage = {
    get length() {
      return store.size;
    },
    clear() {
      store.clear();
    },
    getItem(key: string) {
      return store.get(key) ?? null;
    },
    key(index: number) {
      return [...store.keys()][index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
  };
  Object.defineProperty(window, 'localStorage', { value: storage, writable: true });
}
