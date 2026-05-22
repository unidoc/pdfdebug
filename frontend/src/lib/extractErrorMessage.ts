/**
 * Extracts a human-readable message from an unknown error value, unwrapping
 * the Wails v3 JSON envelope `{message, cause, kind}` when present. Wails
 * v3 stringifies Go errors into that envelope inside `Error.message`, so a
 * naive `err.message` check sees the full JSON blob instead of the Go error
 * text. Falls back to the raw string when JSON.parse throws (non-envelope
 * Errors, plain strings).
 */
export function extractErrorMessage(err: unknown): string {
  const raw = err instanceof Error ? err.message : typeof err === 'string' ? err : 'Unknown error';
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed.message === 'string') return parsed.message;
  } catch {
    /* not a JSON envelope, fall through */
  }
  return raw;
}
