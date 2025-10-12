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
    to: '/docs/changelog',
    label: 'What\'s New',
    position: 'left' as const,
  },
  {
    type: 'dropdown' as const,
    label: 'Documentation',
    position: 'left' as const,
    items: [
      {
        to: '/adr',
        label: 'ADRs (Architecture Decision Records)',
      },
      {
        to: '/rfc',
        label: 'RFCs (Request for Comments)',
      },
      {
        to: '/memos',
        label: 'Memos (Technical Analysis)',
      },
      {
        to: '/prds',
        label: 'PRDs (Product Requirements)',
      },
    ],
  },
  {
    type: 'custom-buildInfo' as any,
    position: 'right' as const,
  },
  {
    type: 'html' as any,
    position: 'right' as const,
    value: '<a href="https://github.com/jrepp/prism-data-layer/actions/workflows/docs.yml" target="_blank" rel="noopener noreferrer" title="GitHub Pages" style="display: flex; align-items: center; padding: 0 0.5rem;"><img src="https://github.com/jrepp/prism-data-layer/actions/workflows/docs.yml/badge.svg" alt="Build" style="margin: 0;" /></a>',
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
        label: 'What\'s New',
        to: '/docs/changelog',
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
      {
        label: 'PRDs',
        to: '/prds',
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
