/**
 * Factory for generating PDF document test data.
 *
 * These factories produce data objects representing the expected shape of
 * PDF documents as surfaced by the Go backend through Wails bindings.
 * They are used to set up test expectations, not to create real PDF files.
 */
import { faker } from '@faker-js/faker';

export type PdfDocumentInfo = {
  id: string;
  fileName: string;
  filePath: string;
  pageCount: number;
  fileSize: number;
  version: string;
};

export const createPdfDocumentInfo = (
  overrides: Partial<PdfDocumentInfo> = {},
): PdfDocumentInfo => ({
  id: faker.string.uuid(),
  fileName: `${faker.system.fileName({ extensionCount: 0 })}.pdf`,
  filePath: faker.system.filePath(),
  pageCount: faker.number.int({ min: 1, max: 200 }),
  fileSize: faker.number.int({ min: 1024, max: 50_000_000 }),
  version: faker.helpers.arrayElement(['1.4', '1.5', '1.6', '1.7', '2.0']),
  ...overrides,
});

export type PdfObjectNode = {
  id: string;
  label: string;
  type: 'dictionary' | 'array' | 'stream' | 'reference' | 'name' | 'string' | 'number' | 'boolean' | 'null';
  hasChildren: boolean;
  childCount: number;
};

export const createPdfObjectNode = (
  overrides: Partial<PdfObjectNode> = {},
): PdfObjectNode => ({
  id: faker.string.uuid(),
  label: faker.helpers.arrayElement([
    '/Type', '/Pages', '/Page', '/Contents', '/Resources',
    '/MediaBox', '/Font', '/ExtGState', '/XObject',
  ]),
  type: faker.helpers.arrayElement(['dictionary', 'array', 'stream', 'reference', 'name', 'string', 'number', 'boolean', 'null']),
  hasChildren: faker.datatype.boolean(),
  childCount: faker.number.int({ min: 0, max: 20 }),
  ...overrides,
});
