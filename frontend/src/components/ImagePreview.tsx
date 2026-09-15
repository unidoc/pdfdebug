/**
 * @file Presentational component for image preview in the detail panel.
 * Renders a base64-encoded downsampled preview with metadata, warnings, error
 * states, a "reduced preview" label when the pixels are downsampled, and an
 * unconditional "Save image..." action. The metadata always reports the REAL
 * image; only the pixels in the preview are reduced.
 */

/** Props for the ImagePreview component. */
interface ImagePreviewProps {
  base64: string;
  mimeType: string;
  width: number;
  height: number;
  colorSpace: string;
  bitsPerComponent: number;
  filter: string;
  warning: string;
  error: string;
  /** Preview pixel width; when smaller than width the preview is downsampled. */
  thumbWidth?: number;
  /** Preview pixel height; when smaller than height the preview is downsampled. */
  thumbHeight?: number;
  /** Backend-direct save of the full-resolution image. Absent = no save action. */
  onSave?: () => void;
  /** Inline save-failure message shown under the save action. */
  saveError?: string;
}

/** Renders an image preview with metadata, reduced-preview label, save action,
 *  warning, and error display. */
export function ImagePreview({
  base64,
  mimeType,
  width,
  height,
  colorSpace,
  bitsPerComponent,
  filter,
  warning,
  error,
  thumbWidth = 0,
  thumbHeight = 0,
  onSave,
  saveError,
}: ImagePreviewProps) {
  const hasError = error !== '';
  const showImage = base64 !== '';
  const isReduced =
    thumbWidth > 0 && thumbHeight > 0 && (thumbWidth < width || thumbHeight < height);

  return (
    <div className="flex-1 min-h-0 flex flex-col">
      {warning && (
        <div
          className="px-3 py-1.5 text-warning text-xs"
          data-testid="image-preview-warning"
        >
          {warning}
        </div>
      )}

      {hasError && (
        <div
          className="p-3 text-error text-sm"
          data-testid="image-preview-error"
        >
          {error}
        </div>
      )}

      {showImage && (
        <>
          <div className="flex-1 min-h-0 flex items-center justify-center p-3">
            <img
              src={`data:${mimeType};base64,${base64}`}
              alt="Image preview"
              className="max-w-full max-h-full object-contain"
              data-testid="image-preview-img"
            />
          </div>
          {isReduced && (
            <div
              className="px-3 pb-1 text-text-muted text-xs"
              data-testid="image-preview-reduced"
            >
              Reduced preview: {thumbWidth} x {thumbHeight} px
            </div>
          )}
        </>
      )}

      <div
        className="border-t border-border p-3 text-xs flex-shrink-0"
        data-testid="image-preview-metadata"
      >
        <div className="flex items-center justify-between mb-1">
          <span className="text-text-secondary font-medium">Image Metadata</span>
          {onSave && (
            <button
              type="button"
              className="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover"
              data-testid="image-preview-save"
              onClick={onSave}
            >
              Save image...
            </button>
          )}
        </div>
        <div className="flex flex-col gap-1 text-text-muted font-mono">
          <div>
            <span className="text-text-secondary">Dimensions: </span>
            {width} x {height} px
          </div>
          <div>
            <span className="text-text-secondary">Color Space: </span>
            {colorSpace || '-'}
          </div>
          <div>
            <span className="text-text-secondary">Bits/Component: </span>
            {bitsPerComponent || '-'}
          </div>
          <div>
            <span className="text-text-secondary">Filter: </span>
            {filter || '-'}
          </div>
        </div>
        {saveError && (
          <div className="mt-2 text-error text-xs" data-testid="image-preview-save-error">
            {saveError}
          </div>
        )}
      </div>
    </div>
  );
}
