#!/usr/bin/env node
/**
 * MDX Syntax Validator
 *
 * Uses @mdx-js/mdx to validate MDX syntax in markdown files.
 * Called from Python validate_docs.py script.
 *
 * Usage:
 *   node validate_mdx.mjs <file1.md> <file2.md> ...
 *
 * Exit codes:
 *   0 - All files valid
 *   1 - Validation errors found
 */

import { readFile } from 'fs/promises';
import { compile } from '@mdx-js/mdx';
import { VFile } from 'vfile';

const args = process.argv.slice(2);

if (args.length === 0) {
  console.error('Usage: node validate_mdx.mjs <file1.md> <file2.md> ...');
  process.exit(2);
}

let hasErrors = false;
const results = [];

for (const filePath of args) {
  try {
    const content = await readFile(filePath, 'utf8');
    const file = new VFile({ path: filePath, value: content });

    // Attempt to compile MDX
    await compile(file, {
      remarkPlugins: [],
      rehypePlugins: [],
      development: false,
    });

    results.push({
      file: filePath,
      valid: true,
      error: null
    });

  } catch (error) {
    hasErrors = true;

    // Extract useful error information
    const errorInfo = {
      file: filePath,
      valid: false,
      message: error.message,
      line: error.line || null,
      column: error.column || null,
      reason: error.reason || error.message,
      source: error.source || null
    };

    results.push(errorInfo);
  }
}

// Output results as JSON for Python to parse
console.log(JSON.stringify(results, null, 2));

process.exit(hasErrors ? 1 : 0);
