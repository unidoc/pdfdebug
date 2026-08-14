/**
 * @file Consolidates the repeated "mirror the latest render value into a ref"
 * pattern (finding #28). A ref whose `.current` always reflects the
 * most recent render's value lets event handlers and async callbacks read fresh
 * state without re-binding on every render.
 */
import { useRef, type MutableRefObject } from 'react';

/**
 * Returns a stable ref whose `.current` is updated, during render, to the
 * latest `value`. The ref object identity is preserved across renders; only
 * `.current` changes. Read it inside callbacks/effects to avoid stale closures.
 */
export function useLatest<T>(value: T): MutableRefObject<T> {
  const ref = useRef(value);
  ref.current = value;
  return ref;
}
