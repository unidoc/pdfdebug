/**
 * Story 9.9: Font Inspection View -- FontPreview Component Tests
 *
 * Test IDs follow the convention where NNN groups by AC.
 * Run: cd frontend && npx vitest run src/components/FontPreview.test.tsx
 */
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { FontPreview } from './FontPreview';

// ---------------------------------------------------------------------------
// Test fixtures -- FontDetail shapes mirroring the backend FontDetail struct
// (Task 1.2). These literals MUST stay in sync with model.go's JSON tags so
// the IPC contract is exercised against the same shape the frontend will
// actually receive from Wails bindings at runtime.
// ---------------------------------------------------------------------------

type FontDescriptorInfoFixture = {
  nodeId: string;
  objectRef: string;
  fontName: string;
  flags: number;
  flagNames: string[];
  italicAngle: number;
  ascent: number;
  descent: number;
  capHeight: number;
  stemV: number;
  fontBBox: number[];
  fontFileFormat: string;
  fontFileSize: number;
};

type EncodingDifferenceFixture = {
  code: number;
  glyphName: string;
};

type ToUnicodeMappingFixture = {
  code: number;
  unicode: string;
  glyph: string;
};

type CIDSystemInfoFixture = {
  registry: string;
  ordering: string;
  supplement: number;
};

type FontDetailFixture = {
  nodeId: string;
  objectRef: string;
  subtype: string;
  baseFont: string;
  firstChar: number;
  lastChar: number;
  encodingName: string;
  baseEncoding: string;
  differences: EncodingDifferenceFixture[];
  toUnicodeMappings: ToUnicodeMappingFixture[];
  toUnicodeError: string;
  embedded: boolean;
  fontDescriptor: FontDescriptorInfoFixture | null;
  descendant: FontDetailFixture | null;
  cidSystemInfo: CIDSystemInfoFixture | null;
  cidToGIDMap: string;
  defaultWidth: number;
};

const baseFontDescriptor: FontDescriptorInfoFixture = {
  nodeId: 'obj:0:10',
  objectRef: '10 0 R',
  fontName: 'Helvetica-Bold',
  flags: 32, // bit 6 (Nonsymbolic)
  flagNames: ['Nonsymbolic'],
  italicAngle: 0,
  ascent: 718,
  descent: -207,
  capHeight: 718,
  stemV: 140,
  fontBBox: [-170, -228, 1003, 962],
  fontFileFormat: 'TrueType',
  fontFileSize: 14592,
};

const baseTrueTypeFont: FontDetailFixture = {
  nodeId: 'obj:0:5',
  objectRef: '5 0 R',
  subtype: 'TrueType',
  baseFont: '/Helvetica-Bold',
  firstChar: 32,
  lastChar: 126,
  encodingName: '/WinAnsiEncoding',
  baseEncoding: '',
  differences: [],
  toUnicodeMappings: [],
  toUnicodeError: '',
  embedded: true,
  fontDescriptor: baseFontDescriptor,
  descendant: null,
  cidSystemInfo: null,
  cidToGIDMap: '',
  defaultWidth: 0,
};

const unembeddedHelvetica: FontDetailFixture = {
  ...baseTrueTypeFont,
  baseFont: '/Helvetica',
  embedded: false,
  fontDescriptor: {
    ...baseFontDescriptor,
    fontName: 'Helvetica',
    fontFileFormat: '',
    fontFileSize: 0,
  },
};

const builtInEncodingFont: FontDetailFixture = {
  ...baseTrueTypeFont,
  encodingName: '',
  baseEncoding: '',
  differences: [],
};

const differencesFont: FontDetailFixture = {
  ...baseTrueTypeFont,
  encodingName: '',
  baseEncoding: '/WinAnsiEncoding',
  differences: [
    { code: 32, glyphName: '/space' },
    { code: 33, glyphName: '/exclam' },
    { code: 34, glyphName: '/quotedbl' },
  ],
};

const toUnicodeFont: FontDetailFixture = {
  ...baseTrueTypeFont,
  toUnicodeMappings: [
    { code: 0x41, unicode: 'U+0041', glyph: 'A' },
    { code: 0x42, unicode: 'U+0042', glyph: 'B' },
    // Ligature ffi (three Unicode codepoints, joined glyph)
    { code: 0xfb03, unicode: 'U+0066 U+0066 U+0069', glyph: 'ffi' },
    // C0 control -- glyph cell MUST be blank or U+25CC (NOT a literal tab)
    { code: 0x09, unicode: 'U+0009', glyph: '' },
    // PUA -- glyph cell blanked
    { code: 0xe000, unicode: 'U+E000', glyph: '' },
    // Unpaired surrogate -- glyph blanked
    { code: 0xd800, unicode: 'U+D800', glyph: '' },
  ],
};

const cidFontType2Descendant: FontDetailFixture = {
  nodeId: 'obj:0:8',
  objectRef: '8 0 R',
  subtype: 'CIDFontType2',
  baseFont: '/NotoSansCJK-Regular',
  firstChar: 0,
  lastChar: 0,
  encodingName: '',
  baseEncoding: '',
  differences: [],
  toUnicodeMappings: [],
  toUnicodeError: '',
  embedded: true,
  fontDescriptor: {
    ...baseFontDescriptor,
    fontName: 'NotoSansCJK-Regular',
    fontFileFormat: 'TrueType',
    fontFileSize: 2_456_788,
  },
  descendant: null,
  cidSystemInfo: {
    registry: 'Adobe',
    ordering: 'Identity',
    supplement: 0,
  },
  cidToGIDMap: 'Identity',
  defaultWidth: 1000,
};

const compositeFont: FontDetailFixture = {
  nodeId: 'obj:0:7',
  objectRef: '7 0 R',
  subtype: 'Type0',
  baseFont: '/NotoSansCJK-Regular',
  firstChar: 0,
  lastChar: 0,
  encodingName: '/Identity-H',
  baseEncoding: '',
  differences: [],
  toUnicodeMappings: [],
  toUnicodeError: '',
  embedded: true, // Reflects descendant.fontDescriptor.fontFileFormat
  fontDescriptor: null, // Type0 parent dicts typically have no FontDescriptor
  descendant: cidFontType2Descendant,
};

const fontWithToUnicodeError: FontDetailFixture = {
  ...baseTrueTypeFont,
  toUnicodeMappings: [],
  toUnicodeError: 'bfchar: unexpected end of stream at offset 142',
};

// ---------------------------------------------------------------------------
// Metadata header surfaces the core identity fields
// ---------------------------------------------------------------------------

describe('metadata header', () => {
  test('shows Subtype, BaseFont, FirstChar, LastChar', () => {
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={() => {}} />);
    // BaseFont (load-bearing for header label too -)
    expect(screen.getByText('/Helvetica-Bold')).toBeInTheDocument();
    // Subtype rendered verbatim (regression guard)
    expect(screen.getByText('TrueType')).toBeInTheDocument();
    // First/Last char range
    expect(screen.getByText(/32/)).toBeInTheDocument();
    expect(screen.getByText(/126/)).toBeInTheDocument();
  });

  test('shows /FontDescriptor and /ToUnicode presence indicators', () => {
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={() => {}} />);
    // The presence indicators are loose; whatever the component uses (a badge,
    // a "FontDescriptor: <ref>" row, etc.), the indirect-ref text MUST appear
    // so users can click into it.
    expect(screen.getByText(/10 0 R/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Embedded badge state + copy
// ---------------------------------------------------------------------------

describe('embedded badge', () => {
  test('green "Embedded" badge with format and size when embedded=true', () => {
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={() => {}} />);
    // Loose: the badge text MUST include "Embedded" + the format string + a
    // human size. We accept "Embedded (TrueType, 14.6 KB)" or "Embedded
    // (TrueType, 14592 bytes)" -- whatever dev picks, both substrings are
    // present.
    const embeddedText = screen.getByText(/Embedded/);
    expect(embeddedText).toBeInTheDocument();
    expect(embeddedText.textContent).toMatch(/TrueType/);
    // Some size representation -- either "14592" (bytes) or "14.6 KB" /
    // "14 KB" (KB).
    expect(embeddedText.textContent).toMatch(/14|14\.\d/);
  });

  test('red "Not embedded" badge when embedded=false', () => {
    render(<FontPreview detail={unembeddedHelvetica} onReferenceClick={() => {}} />);
    expect(
      screen.getByText(/Not embedded -- viewer falls back to system font/i)
    ).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Encoding name vs built-in encoding sentinel
// ---------------------------------------------------------------------------

describe('encoding -- named or built-in', () => {
  test('named encoding renders the name; no glyph table appears', () => {
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={() => {}} />);
    expect(screen.getByText('/WinAnsiEncoding')).toBeInTheDocument();
    // Difference-table column header MUST NOT appear when there are no
    // differences. The exact text varies; we assert no "Glyph name" column
    // header exists, since the only place that appears is a Differences table.
    expect(screen.queryByText(/Glyph name/i)).not.toBeInTheDocument();
  });

  test('built-in encoding sentinel when /Encoding is absent', () => {
    render(<FontPreview detail={builtInEncodingFont} onReferenceClick={() => {}} />);
    expect(
      screen.getByText(/Built-in encoding \(defined in font file\)/i)
    ).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// /Differences table with decimal, hex, glyph name
// ---------------------------------------------------------------------------

describe('/Differences encoding table', () => {
  test('renders one row per Differences entry with decimal + hex + name', () => {
    render(<FontPreview detail={differencesFont} onReferenceClick={() => {}} />);
    // Decimal codes
    expect(screen.getByText('32')).toBeInTheDocument();
    expect(screen.getByText('33')).toBeInTheDocument();
    // Hex codes (case-insensitive)
    expect(screen.getByText(/0x20|0X20/i)).toBeInTheDocument();
    expect(screen.getByText(/0x21|0X21/i)).toBeInTheDocument();
    // Glyph names
    expect(screen.getByText('/space')).toBeInTheDocument();
    expect(screen.getByText('/exclam')).toBeInTheDocument();
    expect(screen.getByText('/quotedbl')).toBeInTheDocument();
  });

  test('BaseEncoding header surfaces above the Differences table', () => {
    render(<FontPreview detail={differencesFont} onReferenceClick={() => {}} />);
    // BaseEncoding on this fixture is "/WinAnsiEncoding"; it must appear
    // somewhere in the Encoding section header so users see what the
    // Differences are diffing against.
    expect(screen.getByText('/WinAnsiEncoding')).toBeInTheDocument();
  });

  test('Encoding/Differences table uses table semantics', () => {
    render(<FontPreview detail={differencesFont} onReferenceClick={() => {}} />);
    // Requires real <table> semantics OR role="table" + row/cell roles so
    // screen readers can read by column. We accept either.
    const tables = screen.getAllByRole('table');
    expect(tables.length).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// /ToUnicode table: hex codes, U+XXXX form, glyph blanking for control /
// PUA / surrogate.
// ---------------------------------------------------------------------------

describe('/ToUnicode table', () => {
  test('renders one row per mapping with U+XXXX codepoint and literal glyph', () => {
    render(<FontPreview detail={toUnicodeFont} onReferenceClick={() => {}} />);
    expect(screen.getByText('U+0041')).toBeInTheDocument();
    expect(screen.getByText('U+0042')).toBeInTheDocument();
    // Literal glyph cell for A
    expect(screen.getAllByText('A').length).toBeGreaterThan(0);
  });

  test('multi-codepoint ligature renders joined literal in the glyph cell', () => {
    render(<FontPreview detail={toUnicodeFont} onReferenceClick={() => {}} />);
    // ffi -> "U+0066 U+0066 U+0069" + glyph "ffi"
    expect(screen.getByText('U+0066 U+0066 U+0069')).toBeInTheDocument();
    expect(screen.getByText('ffi')).toBeInTheDocument();
  });

  test('control codepoint glyph cell is blank or U+25CC -- NEVER a literal control char', () => {
    const { container } = render(
      <FontPreview detail={toUnicodeFont} onReferenceClick={() => {}} />
    );
    // No literal tab (0x09) anywhere in the rendered text.
    // (We scan the rendered text content; a literal '\t' character in any
    // visible cell would fail this assertion.)
    expect(container.textContent ?? '').not.toMatch(/\t/);
    // U+0009 row exists; glyph cell is blank or contains U+25CC dotted circle.
    expect(screen.getByText('U+0009')).toBeInTheDocument();
  });

  test('PUA codepoint glyph cell is blank or U+25CC -- NEVER a literal PUA char', () => {
    const { container } = render(
      <FontPreview detail={toUnicodeFont} onReferenceClick={() => {}} />
    );
    // No literal PUA character (U+E000) in the rendered text. The codepoint
    // string "U+E000" is fine; the actual character is not.
    expect(container.textContent ?? '').not.toMatch(//);
    expect(screen.getByText('U+E000')).toBeInTheDocument();
  });

  test('unpaired surrogate glyph cell is blank or U+25CC', () => {
    const { container } = render(
      <FontPreview detail={toUnicodeFont} onReferenceClick={() => {}} />
    );
    // No literal surrogate (U+D800) anywhere.
    expect(container.textContent ?? '').not.toMatch(/\uD800/);
    expect(screen.getByText('U+D800')).toBeInTheDocument();
  });

  test('/ToUnicode table uses table semantics', () => {
    render(<FontPreview detail={toUnicodeFont} onReferenceClick={() => {}} />);
    const tables = screen.getAllByRole('table');
    expect(tables.length).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// partial-success: toUnicodeError populated
// ---------------------------------------------------------------------------

describe('partial-success ToUnicode warning', () => {
  test('toUnicodeError renders inline warning; other sections still render', () => {
    render(
      <FontPreview detail={fontWithToUnicodeError} onReferenceClick={() => {}} />
    );
    // The warning text exactly matches the wording.
    expect(
      screen.getByText(/ToUnicode present but unparseable:/i)
    ).toBeInTheDocument();
    // Error detail comes through too -- truncated or not, the dev-provided
    // message string must surface so users can debug malformed CMaps.
    expect(
      screen.getByText(/bfchar: unexpected end of stream at offset 142/)
    ).toBeInTheDocument();
    // FontDescriptor section still renders (partial success).
    expect(screen.getByText('Helvetica-Bold')).toBeInTheDocument();
    // BaseFont still renders.
    expect(screen.getByText('/Helvetica-Bold')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// FontDescriptor card: name, flags decoded, metrics, FontFile format string.
// ---------------------------------------------------------------------------

describe('FontDescriptor card', () => {
  test('renders FontName, ItalicAngle, Ascent, Descent, CapHeight, StemV', () => {
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={() => {}} />);
    expect(screen.getByText('Helvetica-Bold')).toBeInTheDocument();
    // Ascent / Descent / CapHeight / StemV numbers
    expect(screen.getByText(/718/)).toBeInTheDocument();
    expect(screen.getByText(/-207/)).toBeInTheDocument();
    expect(screen.getByText(/140/)).toBeInTheDocument();
  });

  test('decoded flag names render as pills (FixedPitch=bit1, AllCap=bit17, ForceBold=bit19)', () => {
    // Synthetic descriptor that sets bits 1, 17, 19 simultaneously.
    // PDF 1.7 spec 9.8.2 Table 123: 1-indexed bit positions.
    // flags = (1<<0) | (1<<16) | (1<<18) = 1 + 65536 + 262144 = 327681.
    const triFlagDescriptor: FontDescriptorInfoFixture = {
      ...baseFontDescriptor,
      flags: 327681,
      flagNames: ['FixedPitch', 'AllCap', 'ForceBold'],
    };
    render(
      <FontPreview
        detail={{ ...baseTrueTypeFont, fontDescriptor: triFlagDescriptor }}
        onReferenceClick={() => {}}
      />
    );
    expect(screen.getByText('FixedPitch')).toBeInTheDocument();
    expect(screen.getByText('AllCap')).toBeInTheDocument();
    expect(screen.getByText('ForceBold')).toBeInTheDocument();
  });

  test('reserved bits 5, 8-16 MUST NOT render as flag names', () => {
    // PDF 1.7 spec: bits 5 and 8-16 are reserved. If the backend leaks them
    // into FlagNames (e.g. "Reserved5"), the FontPreview must not render that
    // string. Backend's responsibility, but we pin a frontend regression
    // guard so a buggy backend release does not silently surface garbage
    // pill labels.
    const dangerousDescriptor: FontDescriptorInfoFixture = {
      ...baseFontDescriptor,
      flagNames: ['Italic', 'Reserved5'],
    };
    render(
      <FontPreview
        detail={{ ...baseTrueTypeFont, fontDescriptor: dangerousDescriptor }}
        onReferenceClick={() => {}}
      />
    );
    // Real flag renders.
    expect(screen.getByText('Italic')).toBeInTheDocument();
    // Reserved bit name absent -- the FontPreview MUST NOT echo whatever
    // string the backend leaks for reserved bits. (Dev: implement the filter
    // in FontPreview OR in font.go's decode helper; whichever, the test
    // pins the observable invariant.)
    expect(screen.queryByText('Reserved5')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Composite (Type0) Descendant Font section
// ---------------------------------------------------------------------------

describe('Type0 Descendant Font section', () => {
  test('renders descendant Subtype, BaseFont, FontDescriptor metrics', () => {
    render(<FontPreview detail={compositeFont} onReferenceClick={() => {}} />);
    // Parent BaseFont still renders in the top section.
    expect(screen.getAllByText('/NotoSansCJK-Regular').length).toBeGreaterThan(0);
    // Descendant subtype renders verbatim.
    expect(screen.getByText('CIDFontType2')).toBeInTheDocument();
    // Descendant FontDescriptor file format surfaces in the embedded badge.
    expect(screen.getByText(/Embedded/i).textContent).toMatch(/TrueType/);
  });

  test('Embedded badge for Type0 reflects DESCENDANT FontDescriptor', () => {
    // Composite font whose parent FontDescriptor is nil; only the descendant
    // carries the FontFile. Embedded badge MUST read TrueType (from
    // descendant), NOT "Not embedded" (from absent parent FontDescriptor).
    render(<FontPreview detail={compositeFont} onReferenceClick={() => {}} />);
    const embedded = screen.getByText(/Embedded/);
    expect(embedded.textContent).toMatch(/TrueType/);
    expect(
      screen.queryByText(/Not embedded -- viewer falls back to system font/)
    ).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// ref-token click + keyboard activation dispatches the onReferenceClick
// prop. The component delegates to a handler passed by DetailPanel; we
// verify the handler receives the right target.
// ---------------------------------------------------------------------------

describe('ref-token click dispatches onReferenceClick', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('clicking the FontDescriptor ref calls onReferenceClick with target', () => {
    const handler = vi.fn();
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={handler} />);
    // The FontDescriptor's nodeId is "obj:0:10"; the ref-token text is
    // "10 0 R" (the objectRef). Either is acceptable as the click target,
    // but the handler MUST receive the NODE ID (obj:0:10), not the raw "10 0
    // R" -- DetailPanel dispatches NAVIGATE_TO_REF with the nodeId.
    const refToken = screen.getByText(/10 0 R/);
    fireEvent.click(refToken);
    expect(handler).toHaveBeenCalledWith('obj:0:10');
  });

  test('Enter on focused ref token dispatches onReferenceClick (keyboard)', () => {
    const handler = vi.fn();
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={handler} />);
    const refToken = screen.getByText(/10 0 R/);
    fireEvent.keyDown(refToken, { key: 'Enter' });
    expect(handler).toHaveBeenCalledWith('obj:0:10');
  });

  test('Space on focused ref token dispatches onReferenceClick (keyboard)', () => {
    const handler = vi.fn();
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={handler} />);
    const refToken = screen.getByText(/10 0 R/);
    fireEvent.keyDown(refToken, { key: ' ' });
    expect(handler).toHaveBeenCalledWith('obj:0:10');
  });

  test('a11y -- ref tokens are keyboard-focusable (tabIndex=0 or button role)', () => {
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={() => {}} />);
    const refToken = screen.getByText(/10 0 R/);
    // Either tabIndex=0 OR role="button" (the existing ValueDisplay pattern).
    // We assert AT LEAST ONE so dev can choose the consistent path.
    const isFocusable =
      refToken.getAttribute('tabindex') === '0' ||
      refToken.getAttribute('role') === 'button' ||
      refToken.tagName === 'BUTTON';
    expect(isFocusable).toBe(true);
  });

  test('Descendant BaseFont row navigates to the descendant font dict', () => {
    const handler = vi.fn();
    render(<FontPreview detail={compositeFont} onReferenceClick={handler} />);
    // Descendant's objectRef is "8 0 R" with nodeId "obj:0:8". Clicking the
    // descendant's ref token in the "Descendant Font" section must dispatch
    // with the descendant's nodeId.
    const descRefToken = screen.getByText(/8 0 R/);
    fireEvent.click(descRefToken);
    expect(handler).toHaveBeenCalledWith('obj:0:8');
  });
});

// ---------------------------------------------------------------------------
// unknown/missing subtype renders verbatim, no crash
// ---------------------------------------------------------------------------

describe('unknown subtype tolerated', () => {
  test('unknown Subtype (e.g. MMType1) renders verbatim with no special handling', () => {
    const exotic: FontDetailFixture = {
      ...baseTrueTypeFont,
      subtype: 'MMType1',
    };
    expect(() =>
      render(<FontPreview detail={exotic} onReferenceClick={() => {}} />)
    ).not.toThrow();
    expect(screen.getByText('MMType1')).toBeInTheDocument();
  });

  test('Type3 procedural font does not crash and renders Subtype verbatim', () => {
    const type3: FontDetailFixture = {
      ...baseTrueTypeFont,
      subtype: 'Type3',
      // Type3 commonly lacks a FontDescriptor.
      fontDescriptor: null,
    };
    expect(() =>
      render(<FontPreview detail={type3} onReferenceClick={() => {}} />)
    ).not.toThrow();
    expect(screen.getByText('Type3')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Defensive: nil FontDescriptor and nil descendant do not crash.
// nil-safety.
// ---------------------------------------------------------------------------

describe('nil-safety on optional fields', () => {
  test('fontDescriptor=null renders without crash; embedded badge reads "Not embedded"', () => {
    const noDescriptor: FontDetailFixture = {
      ...baseTrueTypeFont,
      embedded: false,
      fontDescriptor: null,
    };
    expect(() =>
      render(<FontPreview detail={noDescriptor} onReferenceClick={() => {}} />)
    ).not.toThrow();
    expect(
      screen.getByText(/Not embedded -- viewer falls back to system font/i)
    ).toBeInTheDocument();
  });

  test('non-Type0 with descendant=null does not render Descendant section', () => {
    render(<FontPreview detail={baseTrueTypeFont} onReferenceClick={() => {}} />);
    // No "Descendant Font" section label for non-composite fonts.
    expect(screen.queryByText(/Descendant Font/i)).not.toBeInTheDocument();
  });
});
