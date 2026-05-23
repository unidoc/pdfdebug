import '@testing-library/jest-dom/vitest';
import { vi } from 'vitest';

// @testing-library/dom's waitFor probes `jest` to detect fake timers. Vitest's
// fake timers are compatible at the API level but the probe returns false
// without this bridge, so waitFor falls into a polling loop that never
// advances under vi.useFakeTimers() -- every waitFor() call after a fake-time
// jump hits the 5s vitest test timeout. Polyfilling `jest` here makes
// jestFakeTimersAreEnabled() see vitest's mocked setTimeout and use
// advanceTimersToNextTimer() to drain pending timers between callback
// retries. Story 10-1 PlainTextView.async tests depend on this path.
//
// Story 10-1 dev-step bridge.
type JestLike = {
  advanceTimersByTime: typeof vi.advanceTimersByTime;
  runAllTicks: typeof vi.runAllTicks;
  runAllTimers: typeof vi.runAllTimers;
  runOnlyPendingTimers: typeof vi.runOnlyPendingTimers;
  advanceTimersToNextTimer: typeof vi.advanceTimersToNextTimer;
};
(globalThis as { jest?: JestLike }).jest = {
  advanceTimersByTime: vi.advanceTimersByTime.bind(vi),
  runAllTicks: vi.runAllTicks.bind(vi),
  runAllTimers: vi.runAllTimers.bind(vi),
  runOnlyPendingTimers: vi.runOnlyPendingTimers.bind(vi),
  advanceTimersToNextTimer: vi.advanceTimersToNextTimer.bind(vi),
};

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
