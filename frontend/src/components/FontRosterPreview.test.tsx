/**
 * FontRosterPreview tests -- per-row rendering, embedded chip, unresolved
 * pill, click-to-navigate.
 */
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi } from 'vitest';
import { FontRosterPreview, type FontResourceMapData } from './FontRosterPreview';

const baseRoster: FontResourceMapData = {
  nodeId: 'dict:dict:obj:0:3:Resources:Font',
  entries: [
    {
      name: 'F1',
      nodeId: 'obj:0:4',
      objectRef: '4 0 R',
      baseFont: '/Helvetica',
      subtype: 'Type1',
      encodingSummary: '/WinAnsiEncoding',
      embedded: false,
      unresolved: false,
    },
    {
      name: 'F2',
      nodeId: 'obj:0:7',
      objectRef: '7 0 R',
      baseFont: '/MyTTFont',
      subtype: 'TrueType',
      encodingSummary: 'Built-in',
      embedded: true,
      unresolved: false,
    },
  ],
};

describe('FontRosterPreview', () => {
  test('renders header with entry count', () => {
    render(<FontRosterPreview roster={baseRoster} onReferenceClick={vi.fn()} />);
    expect(screen.getByText(/Fonts used on this page/)).toBeInTheDocument();
    expect(screen.getByText(/\(2\)/)).toBeInTheDocument();
  });

  test('renders one row per resolved entry with BaseFont and Subtype', () => {
    render(<FontRosterPreview roster={baseRoster} onReferenceClick={vi.fn()} />);
    expect(screen.getByText('/Helvetica')).toBeInTheDocument();
    expect(screen.getByText('/MyTTFont')).toBeInTheDocument();
    expect(screen.getByText('Type1')).toBeInTheDocument();
    expect(screen.getByText('TrueType')).toBeInTheDocument();
  });

  test('embedded column shows Embedded chip for embedded fonts', () => {
    render(<FontRosterPreview roster={baseRoster} onReferenceClick={vi.fn()} />);
    expect(screen.getByTestId('font-roster-embedded-yes')).toBeInTheDocument();
    expect(screen.getByTestId('font-roster-embedded-no')).toBeInTheDocument();
  });

  test('clicking a row calls onReferenceClick with target nodeId', () => {
    const handler = vi.fn();
    render(<FontRosterPreview roster={baseRoster} onReferenceClick={handler} />);
    const rows = screen.getAllByTestId('font-roster-row');
    fireEvent.click(rows[0]);
    expect(handler).toHaveBeenCalledWith('obj:0:4');
  });

  test('unresolved entry renders a red pill and is not clickable', () => {
    const handler = vi.fn();
    const roster: FontResourceMapData = {
      nodeId: 'dict:foo',
      entries: [
        ...baseRoster.entries,
        {
          name: 'F3',
          nodeId: '',
          objectRef: '',
          baseFont: '',
          subtype: '',
          encodingSummary: '',
          embedded: false,
          unresolved: true,
        },
      ],
    };
    render(<FontRosterPreview roster={roster} onReferenceClick={handler} />);
    expect(screen.getByTestId('font-roster-unresolved-pill')).toBeInTheDocument();

    const rows = screen.getAllByTestId('font-roster-row');
    const unresolvedRow = rows.find((r) => r.getAttribute('data-unresolved') === 'true');
    expect(unresolvedRow).toBeDefined();
    if (unresolvedRow) fireEvent.click(unresolvedRow);
    // Unresolved row has no click handler -- onReferenceClick must not fire
    // for it (the two clickable rows haven't been clicked either).
    expect(handler).not.toHaveBeenCalled();
  });

  test('empty entries renders the empty-state message', () => {
    render(
      <FontRosterPreview
        roster={{ nodeId: 'dict:empty', entries: [] }}
        onReferenceClick={vi.fn()}
      />
    );
    expect(screen.getByTestId('font-roster-empty')).toBeInTheDocument();
  });
});
