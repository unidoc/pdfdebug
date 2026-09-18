/**
 * @file Update-notification UX constants, centralized.
 *
 * Backend config (GitHub base URL, timeout, page cap) lives on the Go side in
 * internal/updatecheck; these are frontend-only UX values.
 */

/** localStorage key holding the "check for updates automatically" preference. */
export const UPDATE_PREF_STORAGE_KEY = 'unidoc-pdf-debugger:update-prefs';

/**
 * Consecutive download verify-failures after which the dialog steers the user to
 * the releases page instead of looping the Retry button.
 */
export const UPDATE_VERIFY_ESCALATE_AFTER = 3;
