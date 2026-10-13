/**
 * Font CMap and Glyph-Mapping Inspection -- FontPreview tests.
 *
 * Contract under test (the shape Dev must implement; kept in lockstep with the
 * pdfcore FontMappingRow / FontHealth types and the CLI acceptance JSON):
 *
 *   FontDetailData.mappingRows: FontMappingRowData[]
 *   FontDetailData.health: FontHealthData | null
 *
 *   FontMappingRowData = { code, codeHex, glyphName, unicode, unicodeText }
 *   FontHealthData = {
 * declaredCodeCount, toUnicodeMissing, identityWithoutToUnicode,
 * encodingWithoutToUnicodeCodes
 *   }
 *
 * The new component must expose the joined table under
 * data-testid="font-mapping-table" and the banner under
 * data-testid="font-health-banner".
 *
 * Run: cd frontend && npx vitest run src/components/FontPreview.mapping.test.tsx
 */
import { render, screen, within } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { FontPreview } from './FontPreview';

// Local fixture shapes that include the mapping fields. Typed locally
// (not imported) so this file compiles before model.go / FontPreview.tsx grow
// the fields; the component is invoked through an `as never`-free structural
// cast at the call site.
type FontMappingRowData = {
  code: number;
  codeHex: string;
  glyphName: string;
  unicode: string;
  unicodeText: string;
};

type FontHealthData = {
  declaredCodeCount: number;
  toUnicodeMissing: boolean;
  identityWithoutToUnicode: boolean;
  encodingWithoutToUnicodeCodes: number[];
};

type FontDetail13_3 = {
  nodeId: string;
  objectRef: string;
  subtype: string;
  baseFont: string;
  firstChar: number;
  lastChar: number;
  encodingName: string;
  baseEncoding: string;
  differences: { code: number; glyphName: string }[];
  toUnicodeMappings: { code: number; unicode: string; glyph: string }[];
  toUnicodeError: string;
  embedded: boolean;
  fontDescriptor: null;
  descendant: null;
  cidSystemInfo: null;
  cidToGIDMap: string;
  defaultWidth: number;
  // Mapping-table and health-signal additions.
  mappingRows: FontMappingRowData[];
  health: FontHealthData | null;
};

const base: FontDetail13_3 = {
  nodeId: 'obj:0:5',
  objectRef: '5 0 R',
  subtype: 'Type1',
  baseFont: '/CustomFont',
  firstChar: 32,
  lastChar: 34,
  encodingName: '',
  baseEncoding: '/WinAnsiEncoding',
  differences: [
    { code: 32, glyphName: '/space' },
    { code: 33, glyphName: '/exclam' },
  ],
  toUnicodeMappings: [],
  toUnicodeError: '',
  embedded: false,
  fontDescriptor: null,
  descendant: null,
  cidSystemInfo: null,
  cidToGIDMap: '',
  defaultWidth: 0,
  mappingRows: [],
  health: null,
};

/** Render helper: FontPreview's prop type does not yet know the 13.3 fields,
 *  so cast through unknown to pass the extended fixture without `any`. */
function renderFont(detail: FontDetail13_3) {
  const Comp = FontPreview as unknown as (props: {
    detail: FontDetail13_3;
    onReferenceClick: (t: string) => void;
  }) => React.ReactElement;
  return render(<Comp detail={detail} onReferenceClick={() => {}} />);
}

// ---------------------------------------------------------------------------
// Single joined mapping table -- one row per declared code
// ---------------------------------------------------------------------------
describe('joined mapping table', () => {
  test('renders one joined table (not two separate Differences/ToUnicode tables) keyed by code', () => {
    const detail: FontDetail13_3 = {
      ...base,
      toUnicodeMappings: [{ code: 0x41, unicode: 'U+0041', glyph: 'A' }],
      mappingRows: [
        { code: 32, codeHex: '0x20', glyphName: '/space', unicode: '', unicodeText: '' },
        { code: 0x41, codeHex: '0x41', glyphName: '/A', unicode: 'U+0041', unicodeText: 'A' },
      ],
    };
    renderFont(detail);
    const table = screen.getByTestId('font-mapping-table');
    expect(table).toBeInTheDocument();
    // The joined row for 0x41 must carry BOTH the glyph name AND the Unicode
    // text in a single row (the JOIN payoff).
    const row = within(table).getByText('U+0041').closest('tr');
    expect(row).not.toBeNull();
    expect(within(row as HTMLElement).getByText('A')).toBeInTheDocument();
  });

  test('shows hex code form per row', () => {
    const detail: FontDetail13_3 = {
      ...base,
      mappingRows: [
        { code: 65, codeHex: '0x41', glyphName: '/A', unicode: 'U+0041', unicodeText: 'A' },
      ],
    };
    renderFont(detail);
    const table = screen.getByTestId('font-mapping-table');
    expect(within(table).getByText('0x41')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// health-signals banner
// ---------------------------------------------------------------------------
describe('health banner', () => {
  test('flags a missing ToUnicode CMap', () => {
    const detail: FontDetail13_3 = {
      ...base,
      health: {
        declaredCodeCount: 2,
        toUnicodeMissing: true,
        identityWithoutToUnicode: false,
        encodingWithoutToUnicodeCodes: [32, 33],
      },
    };
    renderFont(detail);
    const banner = screen.getByTestId('font-health-banner');
    expect(banner).toBeInTheDocument();
    expect(within(banner).getByText(/ToUnicode/i)).toBeInTheDocument();
  });

  test('flags Identity encoding without ToUnicode (copy yields gibberish)', () => {
    const detail: FontDetail13_3 = {
      ...base,
      subtype: 'Type0',
      encodingName: '/Identity-H',
      health: {
        declaredCodeCount: 0,
        toUnicodeMissing: true,
        identityWithoutToUnicode: true,
        encodingWithoutToUnicodeCodes: [],
      },
    };
    renderFont(detail);
    const banner = screen.getByTestId('font-health-banner');
    expect(within(banner).getByText(/Identity/i)).toBeInTheDocument();
  });

  test('reports a count of encoding codes with no ToUnicode entry', () => {
    const detail: FontDetail13_3 = {
      ...base,
      health: {
        declaredCodeCount: 3,
        toUnicodeMissing: true,
        identityWithoutToUnicode: false,
        encodingWithoutToUnicodeCodes: [32, 33, 34],
      },
    };
    renderFont(detail);
    const banner = screen.getByTestId('font-health-banner');
    // The banner surfaces the affected-code count (3) -- loose match.
    expect(within(banner).getByText(/3/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Virtualization -- thousands of rows must NOT all render to the DOM.
// A CID font with 5,000 mapping rows must render far fewer table-body rows
// than the data length (windowed), keeping the panel interactive.
// ---------------------------------------------------------------------------
describe('virtualization', () => {
  test('windows a large joined table instead of rendering every row', () => {
    const rows: FontMappingRowData[] = [];
    for (let i = 0; i < 5000; i++) {
      rows.push({
        code: i,
        codeHex: `0x${i.toString(16)}`,
        glyphName: `/g${i}`,
        unicode: `U+${i.toString(16).toUpperCase().padStart(4, '0')}`,
        unicodeText: 'x',
      });
    }
    const detail: FontDetail13_3 = {
      ...base,
      subtype: 'Type0',
      encodingName: '/Identity-H',
      mappingRows: rows,
      health: {
        declaredCodeCount: 5000,
        toUnicodeMissing: false,
        identityWithoutToUnicode: false,
        encodingWithoutToUnicodeCodes: [],
      },
    };
    renderFont(detail);
    const table = screen.getByTestId('font-mapping-table');
    // Windowed rendering: the number of rendered <tr> elements in the table
    // body must be far below the 5,000 data rows. A non-virtualized .map()
    // would render all 5,000 and fail this guard.
    const bodyRows = within(table).queryAllByRole('row');
    expect(bodyRows.length).toBeLessThan(500);
  });
});

// ---------------------------------------------------------------------------
// Degradation -- FontPreview renders WITHOUT crashing for a still-'detail'
// payload that carries a malformed ToUnicode signal (toUnicodeError set)
// alongside whatever partial mappingRows / health the backend could still
// assemble. This is the frontend render path for the "malformed ToUnicode
// stream ... never a crash" clause; the backend assembly side is covered by
// TestFontMapping_MalformedToUnicode_DegradesNotCrash in internal/pdfcore.
// The DetailPanel-level 'neither' (fallback DictView) and real-'error' (inline
// message) paths are already covered in DetailPanel.fontPreview.test.tsx, so
// they are NOT duplicated here.
// ---------------------------------------------------------------------------
describe('degradation renders without crashing', () => {
  test('malformed ToUnicode (toUnicodeError set) still renders the health banner and any parsed rows', () => {
    const detail: FontDetail13_3 = {
      ...base,
      // Malformed ToUnicode: backend keeps kind:'detail' and sets the error;
      // ToUnicode mappings are empty because the stream did not parse, but the
      // Differences-derived rows still assemble.
      toUnicodeError: 'invalid bfrange at offset 42',
      toUnicodeMappings: [],
      mappingRows: [
        { code: 32, codeHex: '0x20', glyphName: '/space', unicode: '', unicodeText: '' },
        { code: 33, codeHex: '0x21', glyphName: '/exclam', unicode: '', unicodeText: '' },
      ],
      health: {
        declaredCodeCount: 2,
        toUnicodeMissing: true,
        identityWithoutToUnicode: false,
        encodingWithoutToUnicodeCodes: [32, 33],
      },
    };
    // The render itself must not throw -- the crash guard for the degraded
    // malformed-ToUnicode payload.
    expect(() => renderFont(detail)).not.toThrow();

    // The malformed-ToUnicode signal surfaces (existing ToUnicode error panel).
    expect(screen.getByTestId('font-tounicode-error')).toBeInTheDocument();

    // Health signals still render for whatever parsed ("Health signals
    // still render for whatever parsed").
    expect(screen.getByTestId('font-health-banner')).toBeInTheDocument();

    // The partial mapping rows the backend could still assemble render.
    const table = screen.getByTestId('font-mapping-table');
    expect(within(table).getByText('/space')).toBeInTheDocument();
    expect(within(table).getByText('/exclam')).toBeInTheDocument();
  });

  test('degraded payload with no parsed rows renders the empty-state note, not a crash', () => {
    const detail: FontDetail13_3 = {
      ...base,
      differences: [],
      toUnicodeError: 'truncated CMap stream',
      toUnicodeMappings: [],
      mappingRows: [],
      health: {
        declaredCodeCount: 0,
        toUnicodeMissing: true,
        identityWithoutToUnicode: false,
        encodingWithoutToUnicodeCodes: [],
      },
    };
    expect(() => renderFont(detail)).not.toThrow();
    // Empty declared-code set degrades to the compact note, not an empty table.
    expect(screen.getByTestId('font-mapping-empty')).toBeInTheDocument();
    expect(screen.getByTestId('font-tounicode-error')).toBeInTheDocument();
  });

  test('Identity-H CID descendant with no ToUnicode renders the gibberish warning without crashing', () => {
    // Unresolved-CID-descendant flavour of degradation reachable at the
    // component level (no real browser needed): a Type0/Identity-H font whose
    // ToUnicode is absent -- the classic "copy yields gibberish" case. Health
    // still renders; nothing throws.
    const detail: FontDetail13_3 = {
      ...base,
      subtype: 'Type0',
      baseFont: '/CIDFontNoToUnicode',
      encodingName: '/Identity-H',
      differences: [],
      toUnicodeMappings: [],
      toUnicodeError: '',
      mappingRows: [],
      health: {
        declaredCodeCount: 0,
        toUnicodeMissing: true,
        identityWithoutToUnicode: true,
        encodingWithoutToUnicodeCodes: [],
      },
    };
    expect(() => renderFont(detail)).not.toThrow();
    const banner = screen.getByTestId('font-health-banner');
    expect(within(banner).getByText(/Identity/i)).toBeInTheDocument();
    expect(within(banner).getByText(/gibberish/i)).toBeInTheDocument();
  });
});
