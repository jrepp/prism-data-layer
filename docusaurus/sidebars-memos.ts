import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';
import {generateMemosSidebar} from './sidebar-generator';

/**
 * Sidebar configuration for Technical Memos
 *
 * Features:
 * - Numerical sorting (memo-010 before memo-020)
 * - Excludes templates (000-template.md)
 * - Category summary ("Technical Memos") at the top
 * - De-emphasized memo numbers in UI
 */
const sidebars: SidebarsConfig = {
  memosSidebar: generateMemosSidebar(),
};

export default sidebars;
