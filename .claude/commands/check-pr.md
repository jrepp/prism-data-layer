---
description: Check and fix PR quality issues based on submit-pr standards
---

# Check and Fix Pull Request

You are tasked with checking an existing pull request and fixing any quality issues.

## Workflow Steps

### 1. Identify Current PR

```bash
# Get current branch
BRANCH=$(git branch --show-current)

# Find PR for current branch
gh pr view --json number,title,body,url
```

If no PR found for current branch:
- Stop and inform user
- Suggest they create PR first with `/submit-pr`

### 2. Analyze PR Quality

Check PR against standards from `.claude/commands/submit-pr.md`:

**Title Issues**:
- [ ] Over 72 characters
- [ ] Contains emojis
- [ ] Uses wrong verb tense (not imperative)
- [ ] Too vague or generic
- [ ] Contains marketing language
- [ ] Has trailing period

**Body Issues**:
- [ ] Missing or poor structure
- [ ] Contains emojis (except appropriate checklist items)
- [ ] Uses marketing language (amazing, awesome, incredible, etc.)
- [ ] Lacks specific file references
- [ ] No quantified improvements
- [ ] Missing testing checklist
- [ ] No breaking changes section
- [ ] No dependencies section
- [ ] Uses paragraphs instead of bullet points
- [ ] Excessive enthusiasm or fluff

**Content Issues**:
- [ ] Vague descriptions without technical details
- [ ] Missing "User request:" attribution
- [ ] No measurable impacts where relevant
- [ ] Poor categorization of changes

### 3. Extract Technical Information

To rewrite PR properly, analyze:

```bash
# Get all commits in PR
git log main..HEAD --oneline

# Get detailed commit messages
git log main..HEAD --format="%H%n%s%n%b%n---"

# Get file changes
git diff main..HEAD --stat

# Get specific changes per file
git diff main..HEAD --name-only | head -10
```

Extract:
- Technical changes made (what files, what functionality)
- Measurable improvements (time, performance, coverage)
- Breaking changes or new dependencies
- Testing implications

### 4. Generate Fixed Title

Apply rules:
- Choose correct action verb (Add/Fix/Update/Refactor/Optimize/Remove/Document)
- Maximum 72 characters
- Imperative mood
- Technical and specific
- No emojis, no marketing language
- No trailing period

**Decision tree for action verb**:
- New functionality → `Add`
- Bug fix → `Fix`
- Enhance existing → `Update`
- Code restructure → `Refactor`
- Performance → `Optimize`
- Delete code → `Remove`
- Docs only → `Document`

### 5. Generate Fixed Body

Follow exact structure:

```markdown
## Summary

- First technical change with specific impact
- Second technical change with file reference
- Third technical change with measurable outcome

## Changes

### New Features
- Feature 1 in `path/to/file.ext:123`
- Feature 2 with quantified benefit

### Bug Fixes
- Fix 1 resolving specific issue
- Fix 2 in `path/to/file.ext:456`

### Improvements
- Improvement 1 (reduces X by Y%)
- Improvement 2 enabling new capability

### Documentation
- Doc change 1
- Doc change 2

## Technical Details

<Optional: Include if architectural decisions, trade-offs, or implementation notes are relevant>

Example:
- Implements merge queue using GitHub Actions merge_group event
- Mirrors CI pipeline but optimizes by removing coverage uploads
- Uses concurrency groups to prevent duplicate runs

## Testing

- [ ] Unit tests pass locally
- [ ] Integration tests pass locally
- [ ] CI pipeline passes
- [ ] Manual testing completed for affected features
- [ ] <Any pattern-specific testing>

## Breaking Changes

None

OR

- Breaking change 1 with migration path
- Breaking change 2 with workaround

## Dependencies

None

OR

- New dependency: package-name@version (reason)
- Updated dependency: package-name from X to Y (reason)

---
User request: "<original user request if available>"
```

**Rules to enforce**:
- Use bullet points, never paragraphs
- Include file paths with line numbers where relevant
- Quantify all improvements (time, percentage, count)
- Remove ALL emojis from body
- Remove marketing language
- Be concise and factual
- Categorize changes logically
- Include specific examples, not generalizations

### 6. Apply Fixes

If issues found:

```bash
# Update PR title and body
gh pr edit <number> --title "<fixed-title>" --body "$(cat <<'EOF'
<fixed-body>
EOF
)"

# Add explanatory comment
gh pr comment <number> --body "Updated PR description to meet quality standards:
- Fixed title format (imperative mood, under 72 chars)
- Restructured body with bullet points
- Removed marketing language and emojis
- Added quantified improvements
- Included file references and testing checklist"
```

### 7. Check for Local Changes Needed

If PR quality issues suggest code problems:
- Typos in commit messages → Offer to amend (if safe)
- Missing files → Ask user if should be included
- Unclear changes → Request clarification before updating

**DO NOT**:
- Rewrite git history without permission
- Force push to remote
- Amend commits from other authors
- Make code changes without user confirmation

### 8. Validation

After updating, verify:

```bash
# Re-fetch PR to confirm changes
gh pr view <number> --json title,body

# Validate title length
echo "<title>" | wc -c

# Check for emoji in title/body
echo "<title>" | grep -E "[🚀✨🎉💡🔧🐛]" && echo "❌ Emojis found" || echo "✅ No emojis"
```

### 9. Output to User

Report findings in this format:

```
PR #<number>: <url>

Issues Found:
- Title: <issue description>
- Body: <issue description>
- Content: <issue description>

Fixes Applied:
- Updated title from "<old>" to "<new>"
- Restructured body with proper sections
- Added <X> file references
- Quantified <Y> improvements
- Removed <Z> instances of marketing language

New Title: <fixed-title>

Validation:
- Title length: <X>/72 characters
- Emojis removed: Yes
- Structure: Complete
- File references: <count>
- Testing checklist: Included
```

## Special Cases

### If PR is Already High Quality

```
PR #<number>: <url>

Status: ✓ Meets quality standards

Title: "<title>" (<X>/72 characters)
- Proper action verb
- Imperative mood
- Technical and specific

Body:
- Well-structured with sections
- Uses bullet points
- Includes file references
- Quantifies improvements
- No marketing language
- Complete testing checklist

No changes needed.
```

### If PR Has Minor Issues

Only update if improvements are substantial. For minor issues:
- Note them in output
- Ask user if they want to update
- Apply fixes only if user confirms

### If Unable to Extract Technical Details

```
Unable to determine technical details for PR rewrite.

Please provide:
- What problem does this PR solve?
- What are the key technical changes?
- Are there measurable improvements?
- Are there breaking changes?

Or run `/submit-pr` to create fresh PR with proper analysis.
```

## Examples

### Bad Title → Fixed
- ❌ "Update workflows and add merge queue stuff 🚀"
- ✅ "Add merge queue support for GitHub Actions"

- ❌ "Fixed the broken patterns that weren't working"
- ✅ "Fix pattern build dependencies in CI workflow"

- ❌ "Amazing optimization making everything super fast!!!"
- ✅ "Optimize Go module caching in test pipeline"

### Bad Body → Fixed

❌ **Before**:
```
This PR adds some really cool merge queue features! 🎉

We updated the workflows to support merge queues which is awesome because it will help prevent broken builds on main. Also fixed some other stuff.

Everything is working great now! ✨
```

✅ **After**:
```
## Summary

- Add merge queue workflow triggering on merge_group events
- Update CI workflows (ci.yml, pattern-acceptance-tests.yml, lint-workflows.yml)
- Fix pattern build by adding go mod download step

## Changes

### New Workflows
- Add `.github/workflows/merge-queue.yml` with comprehensive CI checks
- Add `.github/MERGE_QUEUE_SETUP.md` with configuration guide

### CI Updates
- Update `.github/workflows/ci.yml:22` to trigger on merge_group
- Update `.github/workflows/pattern-acceptance-tests.yml:26` to trigger on merge_group
- Update `.github/workflows/lint-workflows.yml:12` to trigger on merge_group
- Modify concurrency groups to handle merge_group.head_sha

### Bug Fixes
- Add dependency download step in `.github/workflows/pattern-acceptance-tests.yml:120`
- Fixes missing go.sum entries causing build failures

## Technical Details

Implements GitHub merge queue support per GitHub Actions documentation. When PRs enter the merge queue, a temporary merge commit is created and all CI workflows run. This prevents integration issues between concurrent PRs.

## Testing

- [x] CI pipeline passes on PR branch
- [x] Merge queue workflow validates successfully
- [ ] Test with actual merge queue (requires admin configuration)

## Breaking Changes

None

## Dependencies

None
```

## Error Handling

If any command fails:
- Report exact error to user
- Suggest resolution
- Do not proceed with updates if validation fails
- Ask user for manual intervention if needed

## Commit Strategy

If making changes beyond PR description:
- Create descriptive commit messages
- Follow repository commit format (see `.claude/commands/commit-format.md`)
- Stage only intended files
- Push to same branch

Do not create commits for PR description updates only.
