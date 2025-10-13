/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import React, {type ReactNode} from 'react';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import type {Props} from '@theme/Footer/Copyright';

function formatBuildTime(isoString: string | undefined): string {
  if (!isoString) return '';

  try {
    const date = new Date(isoString);
    return date.toLocaleString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      timeZoneName: 'short',
    });
  } catch {
    return isoString;
  }
}

export default function FooterCopyright({copyright}: Props): ReactNode {
  const {siteConfig} = useDocusaurusContext();
  const {buildTime, version, commitHash} = siteConfig.customFields as {
    buildTime?: string;
    version?: string;
    commitHash?: string;
  };

  const formattedBuildTime = buildTime ? formatBuildTime(buildTime) : null;
  const shortCommit = commitHash ? String(commitHash).substring(0, 7) : null;

  return (
    <div className="footer__copyright">
      <div
        // Developer provided the HTML, so assume it's safe.
        // eslint-disable-next-line react/no-danger
        dangerouslySetInnerHTML={{__html: copyright}}
      />
      {(formattedBuildTime || version || shortCommit) && (
        <div style={{marginTop: '0.5rem', fontSize: '0.875rem', opacity: 0.8}}>
          {version && <span>Version {version}</span>}
          {version && (formattedBuildTime || shortCommit) && <span> • </span>}
          {formattedBuildTime && <span>Built {formattedBuildTime}</span>}
          {formattedBuildTime && shortCommit && <span> • </span>}
          {shortCommit && <span>Commit {shortCommit}</span>}
        </div>
      )}
    </div>
  );
}
