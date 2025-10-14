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

  // Set the production url of your site here
  url: 'https://jrepp.github.io',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/prism-data-layer/',

  // Mobile-friendly viewport configuration
  headTags: [
    {
      tagName: 'meta',
      attributes: {
        name: 'viewport',
        content: 'width=device-width, initial-scale=1.0, maximum-scale=5.0, user-scalable=yes',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        name: 'theme-color',
        content: '#6366f1',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        name: 'mobile-web-app-capable',
        content: 'yes',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        name: 'apple-mobile-web-app-capable',
        content: 'yes',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        name: 'apple-mobile-web-app-status-bar-style',
        content: 'black-translucent',
      },
    },
  ],

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

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'throw',

  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

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
          sidebarPath: './sidebars.ts',
          // Uses ./docs directory (default)
          routeBasePath: 'docs',
          editUrl: 'https://github.com/jrepp/prism-data-layer/tree/main/docusaurus/',
          // Mobile-friendly sidebar configuration
          sidebarCollapsible: true,
          sidebarCollapsed: true,
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
        exclude: ['**/README.md', '**/000-template.md'],
        sidebarCollapsible: true,
        sidebarCollapsed: true,
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
        exclude: ['**/README.md', '**/000-template.md'],
        sidebarCollapsible: true,
        sidebarCollapsed: true,
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
        exclude: ['**/README.md', '**/000-template.md'],
        sidebarCollapsible: true,
        sidebarCollapsed: true,
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
        sidebarCollapsible: true,
        sidebarCollapsed: true,
      },
    ],
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'prds',
        path: '../docs-cms/prds',
        routeBasePath: 'prds',
        sidebarPath: './sidebars-prds.ts',
        editUrl: 'https://github.com/jrepp/prism-data-layer/tree/main/docs-cms/',
        sidebarCollapsible: true,
        sidebarCollapsed: true,
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

  // Enable Mermaid and configure markdown
  markdown: {
    mermaid: true,
    format: 'detect', // Auto-detect .md vs .mdx files
    mdx1Compat: {
      comments: true,
      admonitions: true,
      headingIds: true,
    },
  },

  themeConfig: {
    // Replace with your project's social card
    image: 'img/docusaurus-social-card.jpg',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    docs: {
      sidebar: {
        hideable: true,
        autoCollapseCategories: true,
      },
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
      hideOnScroll: false,
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
