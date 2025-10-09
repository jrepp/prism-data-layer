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
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  return (
    <div className={styles.buildInfo}>
      <span className={styles.version} title="Documentation version">
        v{version || '0.1.0'}
      </span>
      <span className={styles.separator}>•</span>
      <span className={styles.buildTime} title="Last build time">
        {formatDate(buildTime)}
      </span>
      {commitHash && commitHash !== 'dev' && (
        <>
          <span className={styles.separator}>•</span>
          <span className={styles.commit} title={`Commit: ${commitHash}`}>
            {commitHash.substring(0, 7)}
          </span>
        </>
      )}
    </div>
  );
}
