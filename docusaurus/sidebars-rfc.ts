import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';
import {generateRfcSidebar} from './sidebar-generator';

/**
 * Sidebar configuration for RFCs (Request for Comments)
 *
 * Features:
 * - Numerical sorting (rfc-010 before rfc-020)
 * - Excludes templates (000-template.md)
 * - Category summary at the top
 * - De-emphasized RFC numbers in UI
 */
const sidebars: SidebarsConfig = {
  rfcSidebar: generateRfcSidebar(),
};

export default sidebars;
