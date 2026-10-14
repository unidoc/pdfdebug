/**
 * Plain Text load refused because the file changed on disk: the view shows a
 * reload hint instead of its generic error, and asks GetDiskStatus once so the
 * tab is marked at once (or not, when the file already went back).
 *
 * Run: cd frontend && npx vitest run src/components/PlainTextView.fileChanged.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { PlainTextView } from './PlainTextView';
import { AppProvider } from '../hooks/useDocumentState';
import { readDiskStatus, resetDiskStatusStore } from '../lib/diskStatusStore';

function ProviderWrapper({ children }: { children: ReactNode }) {
  return <AppProvider>{children}</AppProvider>;
}

const mockGetPlainText = vi.fn();
const mockGetPlainTextSize = vi.fn();
const mockGetDiskStatus = vi.fn();

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  GetPlainText: (...args: unknown[]) => mockGetPlainText(...args),
  GetPlainTextSize: (...args: unknown[]) => mockGetPlainTextSize(...args),
  GetDiskStatus: (...args: unknown[]) => mockGetDiskStatus(...args),
}));

const changedStatus = { state: 'changed', loadedModTime: 1_768_219_200_000, diskModTime: 1_768_219_500_000, diskSize: 4096 };

function envelope(message: string) {
  return new Error(JSON.stringify({ message, cause: {}, kind: 'RuntimeError' }));
}

beforeEach(() => {
  vi.clearAllMocks();
  resetDiskStatusStore();
  mockGetPlainTextSize.mockResolvedValue(17);
});

describe('Plain Text on a file changed on disk', () => {
  test('shows the reload hint and marks the tab from GetDiskStatus', async () => {
    mockGetPlainText.mockRejectedValue(envelope('file changed on disk since it was opened'));
    mockGetDiskStatus.mockResolvedValue(changedStatus);

    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });

    expect(await screen.findByText('File changed on disk. Reload to view.')).toBeInTheDocument();
    await waitFor(() => expect(mockGetDiskStatus).toHaveBeenCalledWith('tab-1'));
    await waitFor(() => expect(readDiskStatus('tab-1')?.state).toBe('changed'));
    expect(mockGetDiskStatus).toHaveBeenCalledTimes(1);
  });

  test('a file that already went back is not marked', async () => {
    mockGetPlainText.mockRejectedValue(envelope('file changed on disk since it was opened: open x: no such file'));
    mockGetDiskStatus.mockResolvedValue({ ...changedStatus, state: 'current' });

    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });

    await waitFor(() => expect(mockGetDiskStatus).toHaveBeenCalledWith('tab-1'));
    expect(readDiskStatus('tab-1')).toBeUndefined();
  });

  test('other load errors keep the generic error and do not check the disk', async () => {
    mockGetPlainText.mockRejectedValue(envelope('malformed PDF: read failed'));

    render(<PlainTextView tabId="tab-1" active={true} />, { wrapper: ProviderWrapper });

    await screen.findByTestId('plain-text-error');
    expect(screen.queryByText('File changed on disk. Reload to view.')).not.toBeInTheDocument();
    expect(mockGetDiskStatus).not.toHaveBeenCalled();
  });
});
