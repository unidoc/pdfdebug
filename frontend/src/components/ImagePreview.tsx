/**
 * @file Presentational component for image preview in the detail panel.
 * Renders a base64-encoded image with metadata, warnings, and error states.
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
}

/** Renders an image preview with metadata, warning, and error display. */
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
}: ImagePreviewProps) {
  const hasError = error !== '';
  const showImage = base64 !== '';

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
        <div className="flex-1 min-h-0 flex items-center justify-center p-3">
          <img
            src={`data:${mimeType};base64,${base64}`}
            alt="Image preview"
            className="max-w-full max-h-full object-contain"
            data-testid="image-preview-img"
          />
        </div>
      )}

      <div
        className="border-t border-border p-3 text-xs flex-shrink-0"
        data-testid="image-preview-metadata"
      >
        <div className="text-text-secondary font-medium mb-1">Image Metadata</div>
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
      </div>
    </div>
  );
}
