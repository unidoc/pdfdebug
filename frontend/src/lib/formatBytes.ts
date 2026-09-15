/**
 * @file Shared binary-base byte formatter (JEDEC labels: KB/MB/GB, not
 * KiB/MiB/GiB). Used for file sizes and the image consent estimate so the same
 * bytes never render differently across views.
 */

/**
 * Formats a byte count with `decimals` places (default 1). Non-finite or
 * negative inputs collapse to "0 B". Promotes across a unit boundary when
 * rounding would otherwise render the full count of the lower unit (e.g.
 * "1024.0 KB" -> "1.0 MB"). Pass decimals=0 for a coarse "about N MB" estimate.
 */
export function formatBytes(n: number, decimals = 1): string {
  if (!Number.isFinite(n) || n < 0) {
    return '0 B';
  }
  if (n < 1024) {
    return `${n} B`;
  }
  if (n < 1024 * 1024 && n / 1024 < 1023.95) {
    return `${(n / 1024).toFixed(decimals)} KB`;
  }
  if (n < 1024 * 1024 * 1024 && n / (1024 * 1024) < 1023.95) {
    return `${(n / (1024 * 1024)).toFixed(decimals)} MB`;
  }
  return `${(n / (1024 * 1024 * 1024)).toFixed(decimals)} GB`;
}
