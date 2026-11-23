/**
 * @file The background pdfcpu check and the Validate-run request. Every new
 * tab id that enters app state starts one Validate(tabId, 'pdfcpu') call; the
 * result is kept per tab id until the tab is released, with a per-tab
 * dismissed flag for the banner. A run request asks the detail panel to show
 * the Validate tab and run a profile there. Both live outside AppState, like
 * the reload toast's store.
 */
import { useSyncExternalStore } from 'react';
import { Validate } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import type { ValidateResult } from '../components/ValidateView';

/** The profile the background check runs. */
export const PDFCPU_PROFILE = 'pdfcpu';

/** A tab's background check state: its result once it arrived, and whether its banner was dismissed. */
export interface PdfcpuCheckEntry {
  result: ValidateResult | null;
  dismissed: boolean;
}

/** A request to show the Validate tab for tabId and run profile there. */
export interface ValidateRunRequest {
  tabId: string;
  profile: string;
}

type Listener = () => void;
const listeners = new Set<Listener>();
let entries = new Map<string, PdfcpuCheckEntry>();
// Tab ids whose check started, and tab ids released since. Tab ids are never
// reused, so a released id never starts a check and its late result is dropped.
const started = new Set<string>();
const released = new Set<string>();
let request: ValidateRunRequest | null = null;

function notify() {
  for (const l of listeners) l();
}

function subscribe(l: Listener): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

function update(tabId: string, patch: Partial<PdfcpuCheckEntry>) {
  const next = new Map(entries);
  next.set(tabId, { ...(entries.get(tabId) ?? { result: null, dismissed: false }), ...patch });
  entries = next;
  notify();
}

/**
 * Starts the background pdfcpu check for tabId, once per tab id. A failed
 * call leaves no result, so it shows no banner.
 */
export function startPdfcpuCheck(tabId: string): void {
  if (started.has(tabId) || released.has(tabId)) return;
  started.add(tabId);
  Promise.resolve(Validate(tabId, PDFCPU_PROFILE))
    .then((res: unknown) => {
      if (!released.has(tabId)) update(tabId, { result: res as ValidateResult });
    })
    .catch(() => {});
}

/** Drops tabId's check state; a result that arrives later is ignored. */
export function releasePdfcpuCheck(tabId: string): void {
  released.add(tabId);
  if (!entries.has(tabId)) return;
  const next = new Map(entries);
  next.delete(tabId);
  entries = next;
  notify();
}

/** Hides the banner of tabId. */
export function dismissPdfcpuBanner(tabId: string): void {
  if (!released.has(tabId)) update(tabId, { dismissed: true });
}

/** Returns the background check state of tabId, or null before its result arrived. */
export function usePdfcpuCheck(tabId: string | null): PdfcpuCheckEntry | null {
  const snapshot = () => (tabId ? entries.get(tabId) ?? null : null);
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}

/** Asks the detail panel to show the Validate tab for tabId and run profile. */
export function requestValidateRun(tabId: string, profile: string): void {
  request = { tabId, profile };
  notify();
}

/** Clears the pending run request once it was handled. */
export function clearValidateRequest(): void {
  if (!request) return;
  request = null;
  notify();
}

function getRequest(): ValidateRunRequest | null {
  return request;
}

/** Returns the pending Validate run request, or null. */
export function useValidateRequest(): ValidateRunRequest | null {
  return useSyncExternalStore(subscribe, getRequest, getRequest);
}
