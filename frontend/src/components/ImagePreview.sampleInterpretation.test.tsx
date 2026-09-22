/**
 * Sample-interpretation rows in the image metadata block.
 *
 * The verdict is unconditional: "checked, nothing here" is a real answer and a
 * reader has to be able to tell it apart from "the tool did not look". The
 * structural rows are conditional, because nobody opens this panel asking about
 * them and a permanent "none" row trains readers to skim.
 *
 * Run: cd frontend && npx vitest run src/components/ImagePreview.sampleInterpretation.test.tsx
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { ImagePreview } from './ImagePreview';

const TINY_PNG_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==';

// An image with none of the new entries set: no /Decode, no /SMask, not a
// stencil mask, and a JPEG that carries no Adobe record.
const bareImage = {
  base64: TINY_PNG_BASE64,
  mimeType: 'image/png',
  width: 320,
  height: 240,
  colorSpace: 'DeviceRGB',
  bitsPerComponent: 8,
  filter: 'DCTDecode',
  warning: '',
  error: '',
  decode: null,
  imageMask: false,
  smask: null,
  adobeMarker: 'absent',
  adobeTransform: null,
  sampleInterpretation: 'Normal (default)',
};

describe('the verdict row is unconditional', () => {
  test('renders for an image that carries none of the entries', () => {
    render(<ImagePreview {...bareImage} />);

    const verdict = screen.getByTestId('image-preview-interpretation');
    expect(verdict).toHaveTextContent('Normal (default)');
  });

  test('renders beside an error, where the metadata block still draws', () => {
    render(
      <ImagePreview
        {...bareImage}
        base64=""
        error="failed to decode image stream: invalid JPEG format: missing SOF marker"
        sampleInterpretation="Inverted: /Decode inverts, no Adobe APP14 marker"
      />
    );

    expect(screen.getByTestId('image-preview-interpretation')).toHaveTextContent(
      'Inverted: /Decode inverts, no Adobe APP14 marker'
    );
  });

  test('renders an unknown verdict as the answer it is', () => {
    render(
      <ImagePreview
        {...bareImage}
        adobeMarker="unparseable"
        sampleInterpretation="Unknown: JPEG marker chain unreadable"
      />
    );

    expect(screen.getByTestId('image-preview-interpretation')).toHaveTextContent(
      'Unknown: JPEG marker chain unreadable'
    );
  });
});

describe('the decode array is evidence beside the verdict', () => {
  test('is shown when the key is present', () => {
    render(
      <ImagePreview
        {...bareImage}
        decode={[1, 0, 1, 0, 1, 0]}
        sampleInterpretation="Inverted by /Decode"
      />
    );

    const row = screen.getByTestId('image-preview-decode');
    expect(row.textContent).toMatch(/1\D+0\D+1\D+0\D+1\D+0/);
  });

  test('is hidden when the key is absent', () => {
    render(<ImagePreview {...bareImage} />);

    expect(screen.queryByTestId('image-preview-decode')).toBeNull();
  });
});

describe('the Adobe transform row follows the marker outcome', () => {
  test('is shown with its meaning and its number when a record was found', () => {
    render(<ImagePreview {...bareImage} adobeMarker="present" adobeTransform={2} />);

    expect(screen.getByTestId('image-preview-adobe-transform')).toHaveTextContent(
      'YCCK (transform 2)'
    );
  });

  test('names the transform that applies no colour transform', () => {
    render(<ImagePreview {...bareImage} adobeMarker="present" adobeTransform={0} />);

    expect(screen.getByTestId('image-preview-adobe-transform')).toHaveTextContent(
      'None (transform 0)'
    );
  });

  test.each([
    { transform: 1, want: 'YCbCr (transform 1)' },
    { transform: 7, want: 'Unknown (transform 7)' },
  ])('renders transform $transform with its meaning beside the number', ({ transform, want }) => {
    render(<ImagePreview {...bareImage} adobeMarker="present" adobeTransform={transform} />);

    expect(screen.getByTestId('image-preview-adobe-transform')).toHaveTextContent(want);
  });

  test.each(['absent', 'not-applicable', 'unparseable', 'not-examined'])(
    'is hidden when the outcome is %s, which has no transform to show',
    (outcome) => {
      render(<ImagePreview {...bareImage} adobeMarker={outcome} adobeTransform={null} />);

      expect(screen.queryByTestId('image-preview-adobe-transform')).toBeNull();
    }
  );
});

describe('the structural rows appear only when they carry something', () => {
  test('the soft mask row shows the object reference', () => {
    render(<ImagePreview {...bareImage} smask="12 0 R" />);

    expect(screen.getByTestId('image-preview-smask')).toHaveTextContent('12 0 R');
  });

  test('the soft mask row states presence in words when there is no reference', () => {
    render(<ImagePreview {...bareImage} smask="" />);

    // A direct stream, or the /None name writers borrow from the ExtGState
    // entry. The panel says the same thing the CLI writer says, so the two
    // surfaces do not teach two vocabularies for one object.
    expect(screen.getByTestId('image-preview-smask')).toHaveTextContent(
      'present (no reference)'
    );
  });

  test('the soft mask row is hidden when the key is absent', () => {
    render(<ImagePreview {...bareImage} />);

    expect(screen.queryByTestId('image-preview-smask')).toBeNull();
  });

  test('the stencil mask row appears for a mask', () => {
    render(<ImagePreview {...bareImage} imageMask colorSpace="" />);

    expect(screen.getByTestId('image-preview-image-mask')).toBeInTheDocument();
  });

  test('the stencil mask row is hidden for an image with a colour space', () => {
    render(<ImagePreview {...bareImage} />);

    expect(screen.queryByTestId('image-preview-image-mask')).toBeNull();
  });
});
