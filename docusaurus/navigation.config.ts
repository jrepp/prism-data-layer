/**
 * Central navigation configuration for Prism documentation site.
 * This file serves as the single source of truth for all navigation items.
 */

/**
 * Main navigation items for the navbar
 */
export const navItems = [
  {
    to: '/docs/intro',
    label: 'Overview',
    position: 'left' as const,
  },
  {
    to: '/adr',
    label: 'ADRs',
    position: 'left' as const,
  },
  {
    to: '/rfc',
    label: 'RFCs',
    position: 'left' as const,
  },
  {
    to: '/memos',
    label: 'Memos',
    position: 'left' as const,
  },
  {
    href: 'https://github.com/jrepp/prism-data-layer',
    label: 'GitHub',
    position: 'right' as const,
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
