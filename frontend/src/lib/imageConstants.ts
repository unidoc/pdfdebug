/**
 * @file Image preview UX constants, centralized.
 *
 * These are frontend UX thresholds, not memory bounds. The memory/decode limits
 * live on the Go side in internal/pdfcore/imagelimits.go (maxImageDecodeBytes,
 * maxThumbnailEdge, jpegThumbnailQuality, ...); keep the two in mind together
 * when tuning, but they are not shared at build time.
 */

/**
 * Estimated-decoded-bytes above which an image is announced (consent prompt)
 * before decoding instead of loaded immediately. A UX threshold, not a memory
 * bound - proceeding always attempts the load.
 */
export const IMAGE_WARN_THRESHOLD_BYTES = 64 * 1024 * 1024;

/** Seconds after which the loading indicator escalates its copy. */
export const IMAGE_SLOW_THRESHOLD_SECONDS = 5;
