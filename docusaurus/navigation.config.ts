/**
 * Central navigation configuration for Prism documentation site.
 * This file serves as the single source of truth for all navigation items.
 */

export interface NavItem {
  to?: string;
  href?: string;
  label: string;
  position: 'left' | 'right';
  type?: string;
  sidebarId?: string;
}

/**
 * Main navigation items for the navbar
 */
export const navItems: NavItem[] = [
  {
    to: '/docs/intro',
    label: 'Overview',
    position: 'left',
  },
  {
    to: '/adr',
    label: 'ADRs',
    position: 'left',
  },
  {
    to: '/rfc',
    label: 'RFCs',
    position: 'left',
  },
  {
    to: '/memos',
    label: 'Memos',
    position: 'left',
  },
  {
    href: 'https://github.com/jrepp/prism-data-layer',
    label: 'GitHub',
    position: 'right',
  },
];

/**
 * Footer navigation organized by sections
 */
export const footerLinks = [
  {
    title: 'Documentation',
    items: [
      {
        label: 'Overview',
        to: '/docs/intro',
      },
      {
        label: 'ADRs',
        to: '/adr',
      },
      {
        label: 'RFCs',
        to: '/rfc',
      },
      {
        label: 'Memos',
        to: '/memos',
      },
    ],
  },
  {
    title: 'Project',
    items: [
      {
        label: 'GitHub',
        href: 'https://github.com/jrepp/prism-data-layer',
      },
    ],
  },
];
