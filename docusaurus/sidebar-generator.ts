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
 * Generate sorted sidebar items for a document type with collapsible categories
 *
 * @param docsDir - Path to docs directory (e.g., '../docs-cms/memos')
 * @param prefix - Document prefix (e.g., 'memo', 'adr', 'rfc')
 * @param groupSize - Number of items per category (default: 10)
 * @returns Sidebar configuration array
 */
export function generateSidebar(docsDir: string, prefix: string, groupSize: number = 10) {
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

  const upperPrefix = prefix.toUpperCase();
  const sidebar: any[] = [];

  // Add index page at the top (if exists)
  const indexDoc = docs.find(doc => doc.isIndex);
  if (indexDoc) {
    sidebar.push({
      type: 'doc' as const,
      id: indexDoc.id,
      label: indexDoc.title,
    });
  }

  // Group regular documents into collapsible categories
  const regularDocs = docs.filter(doc => !doc.isIndex);

  if (regularDocs.length === 0) {
    return sidebar;
  }

  // Group documents by range (e.g., 001-010, 011-020)
  const groups = new Map<number, DocItem[]>();

  for (const doc of regularDocs) {
    const groupNumber = Math.floor((doc.number - 1) / groupSize) * groupSize + 1;
    if (!groups.has(groupNumber)) {
      groups.set(groupNumber, []);
    }
    groups.get(groupNumber)!.push(doc);
  }

  // Convert groups to sidebar categories
  for (const [groupStart, groupDocs] of Array.from(groups.entries()).sort((a, b) => a[0] - b[0])) {
    const groupEnd = groupStart + groupSize - 1;
    const startStr = String(groupStart).padStart(3, '0');
    const endStr = String(groupEnd).padStart(3, '0');

    const categoryItems = groupDocs.map((doc) => {
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

    sidebar.push({
      type: 'category' as const,
      label: `${upperPrefix}-${startStr} to ${endStr}`,
      collapsed: true,
      collapsible: true,
      items: categoryItems,
    });
  }

  return sidebar;
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
