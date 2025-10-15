# Git Commit Format

**CRITICAL**: All commits must include the original user prompt.

## Required Structure

```
<Action> <concise subject>

User request: "<exact user prompt>"

[Optional: 1-2 sentence explanation]

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

## Actions

add, implement, update, fix, refactor, remove, document

## Rules

- ALWAYS include "User request:" line (most important!)
- Keep subject under 50 chars when possible
- Capitalize first word
- No period at end
- Body wrapped at 72 chars
- Focus on what/why, not how

## Example

```
Add Rust proxy skeleton with gRPC server

User request: "Create the initial Rust proxy with basic gRPC setup"

Initializes Rust workspace with tokio and tonic dependencies.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```
