import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';
import {navItems, footerLinks} from './navigation.config';
import {execSync} from 'child_process';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

// Get build metadata
const getCommitHash = (): string => {
  try {
    return execSync('git rev-parse HEAD').toString().trim();
  } catch {
    return 'dev';
  }
};

const getVersion = (): string => {
  try {
    return execSync('git describe --tags --always').toString().trim();
  } catch {
    return '0.1.0';
  }
};

const getBuildTime = (): string => {
  return new Date().toISOString();
};

const config: Config = {
  title: 'Prism',
  tagline: 'High-Performance Data Access Gateway',
  favicon: 'img/favicon.ico',

  // Future flags, see https://docusaurus.io/docs/api/docusaurus-config#future
  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

  // Set the production url of your site here
  url: 'https://jrepp.github.io',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/prism-data-layer/',

  // Set custom build output directory and metadata
  customFields: {
    buildOutputDir: '../docs',
    version: getVersion(),
    buildTime: getBuildTime(),
    commitHash: getCommitHash(),
  },

  // GitHub pages deployment config.
  organizationName: 'jrepp',
  projectName: 'prism-data-layer',

  onBrokenLinks: 'warn',

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          path: '../docs-cms',
          sidebarPath: './sidebars.ts',
          // Use docs-cms as centralized documentation source
          // Exclude subdirectories that have their own plugins
          exclude: ['**/adr/**', '**/rfcs/**', '**/memos/**', '**/netflix/**'],
          routeBasePath: 'docs',
          editUrl: 'https://github.com/jrepp/prism-data-layer/tree/main/docs-cms/',
        },
        blog: false, // Disable blog
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'adr',
        path: '../docs-cms/adr',
        routeBasePath: 'adr',
        sidebarPath: './sidebars-adr.ts',
        editUrl: 'https://github.com/jrepp/prism-data-layer/tree/main/docs-cms/',
      },
    ],
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'rfc',
        path: '../docs-cms/rfcs',
        routeBasePath: 'rfc',
        sidebarPath: './sidebars-rfc.ts',
        editUrl: 'https://github.com/jrepp/prism-data-layer/tree/main/docs-cms/',
      },
    ],
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'memos',
        path: '../docs-cms/memos',
        routeBasePath: 'memos',
        sidebarPath: './sidebars-memos.ts',
        editUrl: 'https://github.com/jrepp/prism-data-layer/tree/main/docs-cms/',
      },
    ],
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'netflix',
        path: '../docs-cms/netflix',
        routeBasePath: 'netflix',
        sidebarPath: './sidebars-netflix.ts',
        editUrl: 'https://github.com/jrepp/prism-data-layer/tree/main/docs-cms/',
      },
    ],
  ],

  themes: [
    '@docusaurus/theme-mermaid',
    [
      require.resolve("@easyops-cn/docusaurus-search-local"),
      {
        // Index all markdown content
        hashed: true,
        language: ["en"],
        highlightSearchTermsOnTargetPage: true,
        explicitSearchResultPath: true,

        // Index optimization
        indexDocs: true,
        indexBlog: false,
        indexPages: true,

        // Search behavior
        searchResultLimits: 8,
        searchResultContextMaxLength: 50,

        // UI customization
        searchBarShortcut: true,
        searchBarShortcutHint: true,
        searchBarPosition: "right" as const,
      },
    ],
  ],

  // Enable Mermaid
  markdown: {
    mermaid: true,
  },

  themeConfig: {
    // Replace with your project's social card
    image: 'img/docusaurus-social-card.jpg',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'Prism',
      logo: {
        alt: 'Prism Data Gateway Logo',
        src: 'img/prism-logo-transparent-background.png',
        width: 32,
        height: 32,
      },
      items: navItems,
    },
    footer: {
      style: 'dark',
      links: footerLinks,
      copyright: `Copyright © ${new Date().getFullYear()} Prism. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'rust', 'python', 'protobuf', 'yaml', 'toml', 'sql', 'json'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
