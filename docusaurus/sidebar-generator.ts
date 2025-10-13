/**
 * Custom sidebar generator for ADRs, RFCs, and MEMOs
 *
 * Features:
 * - Numerical sorting (memo-010 before memo-020)
 * - Excludes templates (000-template.md)
 * - Places category index at the top
 * - De-emphasizes document numbers in UI via custom labels
 */

import * as fs from 'fs';
import * as path from 'path';
import matter from 'gray-matter';

export interface DocItem {
  id: string;
  title: string;
  number: number;
  fileName: string;
  isIndex: boolean;
}

/**
 * Extract document number from ID (e.g., "memo-010" → 10)
 */
function extractNumber(id: string): number {
  const match = id.match(/-(0*\d+)/);
  return match ? parseInt(match[1], 10) : 999999;
}

/**
 * Generate sorted sidebar items for a document type
 *
 * @param docsDir - Path to docs directory (e.g., '../docs-cms/memos')
 * @param prefix - Document prefix (e.g., 'memo', 'adr', 'rfc')
 * @returns Sidebar configuration array
 */
export function generateSidebar(docsDir: string, prefix: string) {
  const absolutePath = path.resolve(__dirname, docsDir);

  if (!fs.existsSync(absolutePath)) {
    console.warn(`Directory not found: ${absolutePath}`);
    return [];
  }

  const files = fs.readdirSync(absolutePath);
  const docs: DocItem[] = [];

  for (const fileName of files) {
    // Skip non-markdown files
    if (!fileName.endsWith('.md')) {
      continue;
    }

    // Skip templates
    if (fileName.includes('000-template')) {
      continue;
    }

    // Skip README files
    if (fileName === 'README.md') {
      continue;
    }

    const filePath = path.join(absolutePath, fileName);
    const content = fs.readFileSync(filePath, 'utf8');
    const { data } = matter(content);

    // Determine if this is the index/category page
    const isIndex = fileName === 'index.md' || data.sidebar_position === 1;

    const docId = data.id || fileName.replace('.md', '');
    const docTitle = data.title || fileName;

    docs.push({
      id: docId,
      title: docTitle,
      number: isIndex ? 0 : extractNumber(docId),  // Extract from ID, not filename
      fileName,
      isIndex,
    });
  }

  // Sort: index first, then by number
  docs.sort((a, b) => {
    if (a.isIndex) return -1;
    if (b.isIndex) return 1;
    return a.number - b.number;
  });

  // Generate sidebar items
  return docs.map((doc) => {
    if (doc.isIndex) {
      // Category summary page - use plain title
      return {
        type: 'doc' as const,
        id: doc.id,
        label: doc.title,
      };
    }

    // Regular document - format with de-emphasized number
    // Strip the prefix from title if present (e.g., "MEMO-010: Title" → "Title")
    const titleMatch = doc.title.match(/^[A-Z]+-\d+:\s*(.+)$/);
    const mainTitle = titleMatch ? titleMatch[1].trim() : doc.title;

    // Extract prefix and number from document ID (e.g., "memo-010" → "MEMO", "010")
    const idMatch = doc.id.match(/^([a-z]+)-(\d+)$/);
    if (!idMatch) {
      // Fallback for documents without standard ID format
      return {
        type: 'doc' as const,
        id: doc.id,
        label: doc.title,
      };
    }

    const upperPrefix = idMatch[1].toUpperCase();
    const numberPart = idMatch[2].padStart(3, '0');

    return {
      type: 'doc' as const,
      id: doc.id,
      label: `${mainTitle} • ${upperPrefix}-${numberPart}`,
      customProps: {
        documentNumber: `${upperPrefix}-${numberPart}`,
        mainTitle: mainTitle,
      },
    };
  });
}

/**
 * Generate sidebar for MEMOs
 */
export function generateMemosSidebar() {
  return generateSidebar('../docs-cms/memos', 'memo');
}

/**
 * Generate sidebar for ADRs
 */
export function generateAdrSidebar() {
  return generateSidebar('../docs-cms/adr', 'adr');
}

/**
 * Generate sidebar for RFCs
 */
export function generateRfcSidebar() {
  return generateSidebar('../docs-cms/rfcs', 'rfc');
}
