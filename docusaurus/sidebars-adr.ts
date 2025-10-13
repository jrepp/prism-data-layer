import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';
import {generateAdrSidebar} from './sidebar-generator';

/**
 * Sidebar configuration for Architecture Decision Records (ADRs)
 *
 * Features:
 * - Numerical sorting (adr-010 before adr-020)
 * - Excludes templates (000-template.md)
 * - Category summary at the top
 * - De-emphasized ADR numbers in UI
 */
const sidebars: SidebarsConfig = {
  adrSidebar: generateAdrSidebar(),
};

export default sidebars;
