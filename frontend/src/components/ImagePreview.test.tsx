/**
 * Story 6.2: Image Preview in Detail Panel -- ImagePreview Component Tests
 *
 * TDD RED PHASE: Tests MUST fail until ImagePreview.tsx is created.
 *
 * Test IDs: 6.2-UNIT-001, 6.2-UNIT-002, 6.2-UNIT-003, 6.2-UNIT-006,
 *           6.2-UNIT-007, 6.2-UNIT-010 (Vitest)
 * Run: cd frontend && npx vitest run src/components/ImagePreview.test.tsx
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
// RED PHASE: This import will fail until ImagePreview.tsx is created.
import { ImagePreview } from './ImagePreview';

// --- Test data fixtures ---

// Minimal 1x1 PNG for test rendering
const TINY_PNG_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==';

const defaultProps = {
  base64: TINY_PNG_BASE64,
  mimeType: 'image/png',
  width: 320,
  height: 240,
  colorSpace: 'DeviceRGB',
  bitsPerComponent: 8,
  filter: 'DCTDecode',
  warning: '',
  error: '',
};

// ---------------------------------------------------------------------------
// 6.2-UNIT-001 [P0]: ImagePreview renders base64 image in img tag
// AC#1: Given an XObject image node is selected, Then the DetailPanel shows
//       the rendered image via a data:${mimeType};base64,${base64} URI.
// ---------------------------------------------------------------------------

describe('6.2-UNIT-001: ImagePreview renders base64 image', () => {
  test('renders img element with correct data URI src', () => {
    render(<ImagePreview {...defaultProps} />);

    const img = screen.getByTestId('image-preview-img');
    expect(img).toBeInTheDocument();
    expect(img.tagName).toBe('IMG');
    expect(img).toHaveAttribute(
      'src',
      `data:image/png;base64,${TINY_PNG_BASE64}`
    );
  });

  test('img element has alt text "Image preview"', () => {
    render(<ImagePreview {...defaultProps} />);

    const img = screen.getByTestId('image-preview-img');
    expect(img).toHaveAttribute('alt', 'Image preview');
  });
});

// ---------------------------------------------------------------------------
// 6.2-UNIT-002 [P0]: ImagePreview displays metadata below image
// AC#1: Image metadata displayed below the image (dimensions, color space,
//       encoding filter, bits per component).
// ---------------------------------------------------------------------------

describe('6.2-UNIT-002: ImagePreview metadata display', () => {
  test('displays dimensions as "width x height px"', () => {
    render(<ImagePreview {...defaultProps} />);

    const metadata = screen.getByTestId('image-preview-metadata');
    expect(metadata).toHaveTextContent('320 x 240 px');
  });

  test('displays color space', () => {
    render(<ImagePreview {...defaultProps} />);

    const metadata = screen.getByTestId('image-preview-metadata');
    expect(metadata).toHaveTextContent('DeviceRGB');
  });

  test('displays bits per component', () => {
    render(<ImagePreview {...defaultProps} />);

    const metadata = screen.getByTestId('image-preview-metadata');
    expect(metadata).toHaveTextContent('8');
  });

  test('displays filter', () => {
    render(<ImagePreview {...defaultProps} />);

    const metadata = screen.getByTestId('image-preview-metadata');
    expect(metadata).toHaveTextContent('DCTDecode');
  });
});

// ---------------------------------------------------------------------------
// 6.2-UNIT-003 [P0]: ImagePreview shows error when base64 is empty
// AC#3: Given an image that cannot be rendered, Then the DetailPanel shows
//       the error message with error styling, And no img element is present.
// ---------------------------------------------------------------------------

describe('6.2-UNIT-003: ImagePreview error display', () => {
  const errorProps = {
    ...defaultProps,
    base64: '',
    error: 'unsupported image format: JBIG2',
  };

  test('shows error message when error is set and base64 is empty', () => {
    render(<ImagePreview {...errorProps} />);

    const errorEl = screen.getByTestId('image-preview-error');
    expect(errorEl).toBeInTheDocument();
    expect(errorEl).toHaveTextContent('unsupported image format: JBIG2');
  });

  test('does not render img element when in error state', () => {
    render(<ImagePreview {...errorProps} />);

    expect(screen.queryByTestId('image-preview-img')).not.toBeInTheDocument();
  });

  test('error element uses text-error styling', () => {
    render(<ImagePreview {...errorProps} />);

    const errorEl = screen.getByTestId('image-preview-error');
    expect(errorEl.className).toMatch(/text-error/);
  });

  test('still shows metadata when available even on error', () => {
    const errorWithMeta = {
      ...errorProps,
      width: 640,
      height: 480,
      filter: 'JBIG2Decode',
    };
    render(<ImagePreview {...errorWithMeta} />);

    const metadata = screen.getByTestId('image-preview-metadata');
    expect(metadata).toHaveTextContent('640 x 480');
    expect(metadata).toHaveTextContent('JBIG2Decode');
  });
});

// ---------------------------------------------------------------------------
// 6.2-UNIT-006 [P1]: CSS constraints on img element for scaling
// AC#2: Large images are scaled to fit within the panel using
//       object-fit: contain and max-width: 100% constraints.
// ---------------------------------------------------------------------------

describe('6.2-UNIT-006: ImagePreview CSS constraints', () => {
  test('img element has object-contain class', () => {
    render(<ImagePreview {...defaultProps} />);

    const img = screen.getByTestId('image-preview-img');
    expect(img.className).toMatch(/object-contain/);
  });

  test('img element has max-w-full class', () => {
    render(<ImagePreview {...defaultProps} />);

    const img = screen.getByTestId('image-preview-img');
    expect(img.className).toMatch(/max-w-full/);
  });

  test('img element has max-h-full class', () => {
    render(<ImagePreview {...defaultProps} />);

    const img = screen.getByTestId('image-preview-img');
    expect(img.className).toMatch(/max-h-full/);
  });
});

// ---------------------------------------------------------------------------
// 6.2-UNIT-007 [P1]: Original dimensions shown for large images
// AC#2: The original dimensions are shown in the metadata (e.g., "4000 x 6000 px").
// ---------------------------------------------------------------------------

describe('6.2-UNIT-007: ImagePreview large image dimensions', () => {
  test('shows original dimensions for large images', () => {
    const largeProps = { ...defaultProps, width: 4000, height: 6000 };
    render(<ImagePreview {...largeProps} />);

    const metadata = screen.getByTestId('image-preview-metadata');
    expect(metadata).toHaveTextContent('4000 x 6000 px');
  });
});

// ---------------------------------------------------------------------------
// 6.2-UNIT-010 [P2]: ImagePreview handles missing/partial metadata
// AC: No crash when metadata fields are zero/empty.
// ---------------------------------------------------------------------------

describe('6.2-UNIT-010: ImagePreview missing metadata', () => {
  const partialProps = {
    ...defaultProps,
    width: 0,
    height: 0,
    colorSpace: '',
    bitsPerComponent: 0,
    filter: '',
  };

  test('does not crash with zero/empty metadata', () => {
    expect(() => render(<ImagePreview {...partialProps} />)).not.toThrow();
  });

  test('metadata section still renders with partial data', () => {
    render(<ImagePreview {...partialProps} />);

    const metadata = screen.getByTestId('image-preview-metadata');
    expect(metadata).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Warning display (AC#4): Warning text shown above metadata
// AC#4: When ImageData.warning is non-empty, an amber-colored notice is shown.
// ---------------------------------------------------------------------------

describe('6.2-UNIT-012: ImagePreview warning display', () => {
  const warningProps = {
    ...defaultProps,
    warning: 'Image uses CMYK color space (colors may be inaccurate)',
  };

  test('displays warning text when warning is non-empty', () => {
    render(<ImagePreview {...warningProps} />);

    const warningEl = screen.getByTestId('image-preview-warning');
    expect(warningEl).toBeInTheDocument();
    expect(warningEl).toHaveTextContent(
      'Image uses CMYK color space (colors may be inaccurate)'
    );
  });

  test('warning uses text-warning styling', () => {
    render(<ImagePreview {...warningProps} />);

    const warningEl = screen.getByTestId('image-preview-warning');
    expect(warningEl.className).toMatch(/text-warning/);
  });

  test('warning is not rendered when warning is empty', () => {
    render(<ImagePreview {...defaultProps} />);

    expect(
      screen.queryByTestId('image-preview-warning')
    ).not.toBeInTheDocument();
  });
});
