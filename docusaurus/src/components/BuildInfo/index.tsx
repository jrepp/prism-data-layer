import React from 'react';
import type { ReactElement } from 'react';
import styles from './styles.module.css';

interface BuildInfoProps {
  version?: string;
  buildTime?: string;
  commitHash?: string;
}

export default function BuildInfo({ version, buildTime, commitHash }: BuildInfoProps): ReactElement {
  const formatDate = (isoString: string | undefined) => {
    if (!isoString) return 'Unknown';
    const date = new Date(isoString);
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
    });
  };

  // Extract short commit hash from version if it looks like a git describe output
  const displayVersion = version?.startsWith('v') ? version : (commitHash?.substring(0, 7) || version);

  return (
    <div className={styles.buildInfo}>
      <span className={styles.version} title={`Build: ${displayVersion}`}>
        {displayVersion}
      </span>
      <span className={styles.separator}>•</span>
      <span className={styles.buildTime} title={`Last updated: ${formatDate(buildTime)}`}>
        {formatDate(buildTime)}
      </span>
    </div>
  );
}
