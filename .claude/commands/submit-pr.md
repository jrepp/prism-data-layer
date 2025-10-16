---
description: Submit a high-quality pull request with proper staging and formatting
---

# Submit Pull Request

You are tasked with creating or updating a pull request following these strict guidelines:

## Workflow Steps

1. **Verify Branch Structure**
   - Confirm current branch is `{username}/feature-name` format (NOT main/master)
   - If on wrong branch, stop and ask user for branch name
   - Get GitHub username from `gh api user --jq .login`

2. **Stage and Review Changes**
   - Run `git status` to see all changes
   - Stage ALL relevant changes with `git add`
   - Verify no unintended files are included
   - If unstaged changes exist, confirm with user before proceeding

3. **Analyze Commits**
   - Run `git log main..HEAD --oneline` to see all commits
   - Analyze commit messages and changes for PR summary
   - Extract key technical changes, not fluff

4. **Create/Update PR**
   - Check if PR exists: `gh pr view --json number,title,body 2>/dev/null`
   - If PR exists, analyze if it needs updating
   - If creating new PR, push branch first: `git push -u origin $(git branch --show-current)`

## PR Title Requirements

**Format**: `<action>: <subject>`

**Action verbs** (choose most accurate):
- `Add` - New feature/functionality
- `Update` - Enhancement to existing feature
- `Fix` - Bug fix
- `Refactor` - Code restructuring without behavior change
- `Remove` - Delete feature/code
- `Optimize` - Performance improvement
- `Document` - Documentation only

**Title Rules**:
- Maximum 72 characters
- No trailing period
- Imperative mood (Add, not Added/Adding)
- Be specific and technical
- NO emojis, NO marketing language

**Examples**:
- ✅ `Add merge queue support for GitHub Actions`
- ✅ `Fix pattern build dependencies in CI workflow`
- ✅ `Optimize Go module caching in test pipeline`
- ❌ `🚀 Amazing new merge queue feature!`
- ❌ `Updated some workflows to be better`

## PR Body Requirements

Use this exact structure:

```markdown
## Summary

<2-3 bullet points of technical changes, no fluff>

## Changes

### Category 1
- Specific change with file reference
- Another specific change

### Category 2
- Change with measurable impact (e.g., "reduces build time by 30%")

## Technical Details

<Optional: Architecture decisions, trade-offs, implementation notes>

## Testing

- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed
- [ ] <Any specific test scenarios>

## Breaking Changes

<List any breaking changes or "None">

## Dependencies

<List any new dependencies or "None">
```

**Body Rules**:
- Use bullet points, not paragraphs
- Include file paths when relevant (e.g., `.github/workflows/ci.yml:45`)
- Quantify improvements (time, size, performance)
- NO emojis (except in checklist items if appropriate)
- NO marketing language ("amazing", "awesome", "incredible")
- NO excessive enthusiasm or fluff
- Focus on WHAT changed and WHY, not how great it is

## Data-Driven Language

**Use**:
- "Reduces build time from 5m to 3m"
- "Adds 15 new test cases covering edge cases"
- "Fixes race condition in consumer pattern"
- "Implements RFC-042 merge queue specification"

**Avoid**:
- "Much faster builds!"
- "Tons of new tests!"
- "Amazing fix for the pattern!"
- "Awesome implementation!"

## Creating the PR

If creating NEW PR:
```bash
gh pr create --title "<title>" --body "$(cat <<'EOF'
<body content>
EOF
)"
```

If UPDATING existing PR:
```bash
gh pr edit <number> --title "<new-title>" --body "$(cat <<'EOF'
<updated body>
EOF
)"
```

## Validation Checklist

Before submitting, verify:
- [ ] Title is under 72 chars, no emoji, imperative mood
- [ ] Body has clear structure with bullet points
- [ ] No marketing language or excessive enthusiasm
- [ ] Specific file paths or line numbers where relevant
- [ ] Quantified improvements where possible
- [ ] Breaking changes and dependencies documented
- [ ] Testing checklist included
- [ ] Branch name follows {username}/feature pattern
- [ ] All relevant commits are included

## If PR Exists

When updating an existing PR:
1. Fetch current PR: `gh pr view <number> --json title,body`
2. Analyze for quality issues (emoji, fluff, poor structure)
3. Rewrite according to rules above
4. Update with `gh pr edit`
5. Add comment explaining update: `gh pr comment <number> --body "Updated PR description for clarity and technical accuracy"`

## User Prompt Inclusion

At the end of the PR body, add:
```markdown
---
User request: "<original user request>"
```

## Output to User

After creating/updating PR:
1. Show PR URL
2. Summarize key changes (3-5 bullets)
3. Note any validation warnings
4. DO NOT include emoji in your response
5. Keep response concise and factual
