/**
 * mapErrorMessage unit tests -- pure function, lowest viable test layer.
 * Covers the error keyword -> user-friendly message mapping from story 2-4.
 */
import { describe, test, expect, vi } from 'vitest';
import { mapErrorMessage } from './usePDFService';

// Mock Wails bindings
vi.mock(
  '../../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
  })
);

describe('mapErrorMessage', () => {
  test('maps "encrypted" keyword to encrypted message', () => {
    expect(mapErrorMessage('file is encrypted')).toBe(
      'This PDF could not be opened. The file appears to be encrypted.'
    );
  });

  test('maps "password" keyword to encrypted message', () => {
    expect(mapErrorMessage('password required')).toBe(
      'This PDF could not be opened. The file appears to be encrypted.'
    );
  });

  test('maps "malformed" keyword to damaged message', () => {
    expect(mapErrorMessage('malformed PDF structure')).toBe(
      'This PDF could not be opened. The file appears to be damaged or corrupt.'
    );
  });

  test('maps "not found" keyword to missing file message', () => {
    expect(mapErrorMessage('file not found at path')).toBe(
      'The file could not be found. It may have been moved or deleted.'
    );
  });

  test('returns generic message for unknown errors', () => {
    expect(mapErrorMessage('something unexpected')).toBe(
      'This PDF could not be opened. An unexpected error occurred.'
    );
  });

  test('matching is case-insensitive', () => {
    expect(mapErrorMessage('ENCRYPTED')).toBe(
      'This PDF could not be opened. The file appears to be encrypted.'
    );
    expect(mapErrorMessage('Malformed')).toBe(
      'This PDF could not be opened. The file appears to be damaged or corrupt.'
    );
  });
});
