/**
 * Build information utilities for displaying version and build metadata
 */

export interface BuildInfo {
  version: string;
  buildTime: string;
  commitHash: string;
}

/**
 * Get build information from environment or defaults
 * These values are injected at build time via docusaurus.config.ts
 */
export function getBuildInfo(): BuildInfo {
  return {
    version: process.env.DOCUSAURUS_VERSION || '0.1.0',
    buildTime: process.env.DOCUSAURUS_BUILD_TIME || new Date().toISOString(),
    commitHash: process.env.DOCUSAURUS_COMMIT_HASH || 'dev',
  };
}

/**
 * Format build time as a human-readable string
 */
export function formatBuildTime(isoString: string): string {
  const date = new Date(isoString);
  return date.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZoneName: 'short',
  });
}
