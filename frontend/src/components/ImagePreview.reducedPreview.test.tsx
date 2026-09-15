/**
 * Reduced-preview labelling -- ImagePreview component test.
 *
 * The preview pixels are downsampled while the metadata keeps reporting the real
 * image. The panel must label the preview as reduced (using the thumbnail's own
 * dimensions) so a smaller image is not mistaken for the document's real one.
 *
 * Run: cd frontend && npx vitest run src/components/ImagePreview.reducedPreview.test.tsx
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { ImagePreview } from './ImagePreview';

const TINY_PNG_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==';

const reducedProps = {
  base64: TINY_PNG_BASE64,
  mimeType: 'image/png',
  width: 8192,
  height: 8192,
  thumbWidth: 2048,
  thumbHeight: 2048,
  colorSpace: 'DeviceRGB',
  bitsPerComponent: 8,
  filter: 'FlateDecode',
  warning: '',
  error: '',
};

describe('ImagePreview reduced-preview label', () => {
  test('labels the preview as reduced while metadata reports the real image', () => {
    render(<ImagePreview {...reducedProps} />);

    const label = screen.getByTestId('image-preview-reduced');
    expect(label.textContent).toMatch(/reduced/i);
    expect(label.textContent).toMatch(/2048/);

    // The reduced pixels must not overwrite the real geometry.
    const metadata = screen.getByTestId('image-preview-metadata');
    expect(metadata).toHaveTextContent('8192 x 8192 px');
  });
});
