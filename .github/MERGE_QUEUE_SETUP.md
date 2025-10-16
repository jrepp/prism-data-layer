# GitHub Merge Queue Setup Guide

This guide walks you through configuring GitHub's merge queue feature for the Prism repository.

## Overview

The merge queue helps prevent "broken main" scenarios by testing PRs together in a queue before merging them. This ensures that the combination of multiple PRs doesn't introduce conflicts or break CI.

## Prerequisites

- Repository must be on GitHub Team, GitHub Enterprise Cloud, or be a public repository
- Branch protection rules must be configured on `main` branch
- Merge queue workflow (`.github/workflows/merge-queue.yml`) must be present

## Step-by-Step GitHub UI Configuration

### Step 1: Navigate to Branch Protection Settings

1. Go to your repository on GitHub: `https://github.com/YOUR_ORG/data-access-2`
2. Click **Settings** (tab at the top)
3. In the left sidebar, click **Branches**
4. Find the `main` branch protection rule or click **Add branch protection rule** if one doesn't exist

### Step 2: Configure Branch Protection Rule

Configure the following settings for the `main` branch:

#### Basic Protection

- **Branch name pattern**: `main`
- ✅ **Require a pull request before merging**
  - ✅ **Require approvals**: 1 (adjust based on your team size)
  - ✅ **Dismiss stale pull request approvals when new commits are pushed**
  - ✅ **Require approval of the most recent reviewable push**

#### Status Checks (Critical for Merge Queue)

- ✅ **Require status checks to pass before merging**
  - ✅ **Require branches to be up to date before merging**

  **Add the following status checks** (these must match job names in workflows):

  For regular PR checks (from `ci.yml`):
  - `CI Status Check` (the final status check job)
  - `Lint Rust`
  - `Lint Python`
  - `Lint Go (critical)`
  - `Lint Go (security)`
  - `Lint Go (style)`
  - `Lint Go (quality)`
  - `Test Rust Proxy`
  - `Validate Documentation`
  - `Build All Components`

  For pattern tests (from `pattern-acceptance-tests.yml`):
  - `Pattern Acceptance Status` (the final status check job)

#### Additional Protection Rules

- ✅ **Require conversation resolution before merging** (recommended)
- ✅ **Require linear history** (optional, but recommended for clean history)
- ✅ **Do not allow bypassing the above settings** (recommended)
- ✅ **Allow force pushes**: ❌ (disabled)
- ✅ **Allow deletions**: ❌ (disabled)

### Step 3: Enable Merge Queue

Scroll down to the **Merge queue** section:

1. ✅ **Enable merge queue**

2. **Merge method**: Select your preferred method
   - **Merge commit** (recommended for maintaining full history)
   - **Squash commits** (cleaner history, but loses granular commit info)
   - **Rebase commits** (linear history without merge commits)

3. **Build concurrency**:
   - Default: `5` (tests up to 5 PRs together in queue)
   - Adjust based on your CI capacity and typical PR volume

4. **Merge queue size limits**:
   - **Maximum pull requests to build**: `5` (recommended)
   - **Minimum pull requests to merge**: `1` (can increase to batch merges)
   - **Maximum wait time**: `5 minutes` (time to wait for additional PRs to group)

5. **Status checks for merge queue**:
   - ✅ **Only merge when checks pass**
   - Select the merge queue workflow: `Merge Queue Status`

6. **Merge queue grouping strategy**:
   - **Standard**: Groups PRs by arrival time (recommended for most teams)
   - **Stacked**: For teams using stacked PRs

### Step 4: Save and Verify

1. Scroll to the bottom and click **Save changes**
2. Verify the rule appears in the branch protection list

## Using the Merge Queue

### For Pull Request Authors

Once your PR is approved and passes CI:

1. **Instead of clicking "Merge"**, click **"Merge when ready"** (or **"Add to merge queue"**)
2. Your PR enters the queue and GitHub creates a temporary merge commit
3. The merge queue workflow runs (`.github/workflows/merge-queue.yml`)
4. If checks pass, your PR is automatically merged
5. If checks fail, your PR is removed from the queue and you're notified

### Queue Status

Monitor the queue:
- View position: Check the PR page for queue position
- View queue: Settings → Branches → View merge queue
- Notifications: You'll receive updates on queue progress

## Workflow Details

### Regular CI (`ci.yml`)
- Runs on: `push` to `main`, `pull_request` to `main`
- Purpose: Pre-merge validation
- Status check: `CI Status Check`

### Merge Queue (`merge-queue.yml`)
- Runs on: `merge_group` (triggered by queue)
- Purpose: Final validation before merge
- Status check: `Merge Queue Status`
- Difference: Skips coverage uploads and notifications for speed

### Pattern Acceptance (`pattern-acceptance-tests.yml`)
- Runs on: `push` to `main`, `pull_request` to `main`
- Purpose: Validate pattern compliance
- Status check: `Pattern Acceptance Status`

## Benefits

1. **Prevents Broken Main**: Tests PRs together before merging
2. **Automatic Merging**: No manual merge needed after approval
3. **Fair Queuing**: First-approved, first-merged (FIFO)
4. **Conflict Detection**: Catches integration issues early
5. **CI Optimization**: Batches multiple PRs to reduce CI load

## Troubleshooting

### PR Removed from Queue

**Reason**: Merge queue checks failed
**Action**:
1. Check the workflow run in the Actions tab
2. Fix issues in your PR
3. Push new commits
4. Re-add to merge queue after new checks pass

### Queue Is Stuck

**Reason**: A PR in front is failing repeatedly
**Action**:
1. Admin can manually remove failing PRs from queue
2. Consider lowering `build_concurrency` temporarily

### Status Check Not Found

**Reason**: Workflow job name doesn't match branch protection
**Action**:
1. Check job names in `.github/workflows/*.yml`
2. Update branch protection to match exact job names
3. Job names are case-sensitive

## Rollback Plan

If merge queue causes issues:

1. Go to Settings → Branches → main branch rule
2. Uncheck **Enable merge queue**
3. Save changes
4. PRs can now be merged normally with "Merge pull request" button

The merge queue workflow will remain but won't run without the queue enabled.

## Configuration Files

- **Merge Queue Workflow**: `.github/workflows/merge-queue.yml`
- **Regular CI**: `.github/workflows/ci.yml`
- **Pattern Tests**: `.github/workflows/pattern-acceptance-tests.yml`
- **This Guide**: `.github/MERGE_QUEUE_SETUP.md`

## Additional Resources

- [GitHub Docs: Managing a Merge Queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue)
- [GitHub Docs: About Merge Queues](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/about-merge-queues)

---

**Questions?** Consult your team lead or check GitHub's official documentation.
