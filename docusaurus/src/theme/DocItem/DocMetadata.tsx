/**
 * Custom component to display document frontmatter metadata
 * Shows tags and compact metadata (status, author, dates) at the top of each document
 */

import React from 'react';
import type {DocFrontMatter as BaseDocFrontMatter} from '@docusaurus/plugin-content-docs';
import styles from './DocMetadata.module.css';

// Extend DocFrontMatter to include custom properties
interface ExtendedDocFrontMatter extends BaseDocFrontMatter {
  status?: string | string[];
  author?: string;
  deciders?: string;
  created?: string;
  updated?: string;
  date?: string;
}

interface DocMetadataProps {
  frontMatter: ExtendedDocFrontMatter;
}

function formatDate(dateString: string | undefined): string {
  if (!dateString) return '';
  try {
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  } catch {
    return dateString;
  }
}

function getStatusColor(status: string | string[] | undefined): string {
  if (!status) return 'default';

  // Handle array or non-string status
  if (typeof status !== 'string') return 'default';

  const normalized = status.toLowerCase();
  if (normalized === 'accepted' || normalized === 'implemented') return 'success';
  if (normalized === 'proposed' || normalized === 'draft') return 'info';
  if (normalized === 'deprecated' || normalized === 'superseded' || normalized === 'rejected') return 'warning';
  return 'default';
}

export default function DocMetadata({frontMatter}: DocMetadataProps): React.ReactElement | null {
  // Extract simple values from frontMatter
  const tags = frontMatter.tags || [];
  const status = frontMatter.status;
  const author = frontMatter.author;
  const deciders = frontMatter.deciders;
  const created = frontMatter.created;
  const updated = frontMatter.updated;
  const date = frontMatter.date;

  // Don't render if there's no relevant metadata
  const hasTags = tags.length > 0;
  const hasStatus = !!status && typeof status === 'string';
  const hasAuthor = !!author;
  const hasDeciders = !!deciders;
  const hasDates = !!(created || updated || date);

  if (!hasTags && !hasStatus && !hasAuthor && !hasDeciders && !hasDates) {
    return null;
  }

  const statusColor = getStatusColor(status);
  const statusDisplay = typeof status === 'string' ? status : '';

  // Convert FrontMatterTag to string
  const tagLabels = tags.map((tag) => (typeof tag === 'string' ? tag : tag.label));

  return (
    <div className={styles.docMetadata}>
      {/* Tags at the top */}
      {hasTags && (
        <div className={styles.tagsRow}>
          {tagLabels.map((tag, index) => (
            <span key={`${tag}-${index}`} className={styles.tag}>
              {tag}
            </span>
          ))}
        </div>
      )}

      {/* Compact metadata row */}
      <div className={styles.metadataRow}>
        {hasStatus && (
          <span className={`${styles.metadataItem} ${styles[statusColor]}`}>
            <strong>Status:</strong> {statusDisplay}
          </span>
        )}

        {hasAuthor && (
          <span className={styles.metadataItem}>
            <strong>Author:</strong> {author}
          </span>
        )}

        {hasDeciders && (
          <span className={styles.metadataItem}>
            <strong>Deciders:</strong> {deciders}
          </span>
        )}

        {created && (
          <span className={styles.metadataItem}>
            <strong>Created:</strong> {formatDate(created)}
          </span>
        )}

        {updated && (
          <span className={styles.metadataItem}>
            <strong>Updated:</strong> {formatDate(updated)}
          </span>
        )}

        {date && !created && (
          <span className={styles.metadataItem}>
            <strong>Date:</strong> {formatDate(date)}
          </span>
        )}
      </div>
    </div>
  );
}
