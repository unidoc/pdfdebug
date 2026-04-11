import '@testing-library/jest-dom/vitest';

// Node 25+ provides a broken globalThis.localStorage when --localstorage-file
// is not set. Vitest's jsdom environment may not override it. Provide a
// standards-compliant in-memory implementation for tests.
if (typeof window !== 'undefined' && typeof window.localStorage.setItem !== 'function') {
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
