---
name: self-improving-agent
description: Captures learnings, errors, and corrections to enable continuous improvement. Use when a command fails unexpectedly, user corrects the agent, knowledge was outdated, or a better approach is found.
---

# Self-Improving Agent

Adapted from ClawHub (pskoett/self-improving-agent v1.0.11). Log learnings and errors to structured files so patterns accumulate and get promoted into permanent project memory.

## Quick Reference

| Situation | Action |
|-----------|--------|
| Command/operation fails | Log to `.learnings/ERRORS.md` |
| User corrects you | Log to `.learnings/LEARNINGS.md` with category `correction` |
| API/external tool fails | Log to `.learnings/ERRORS.md` with integration details |
| Knowledge was outdated | Log to `.learnings/LEARNINGS.md` with category `knowledge_gap` |
| Found better approach | Log to `.learnings/LEARNINGS.md` with category `best_practice` |
| Recurring pattern simplified | Log to `.learnings/LEARNINGS.md` with category `pattern` |
| Broadly applicable learning | Promote to user's global rules or Knowledge Items |

## Logging Format

### LEARNINGS.md
```markdown
## [Date] - [Short Title]
- **Category**: correction | knowledge_gap | best_practice | pattern
- **Context**: What was happening
- **Learning**: What was discovered
- **Action Taken**: How it was resolved
- **See Also**: Links to related learnings
```

### ERRORS.md
```markdown
## [Date] - [Error Summary]
- **Command/Operation**: What failed
- **Error Message**: Exact error text
- **Root Cause**: Why it failed
- **Fix Applied**: What resolved it
- **Prevention**: How to avoid this in the future
```

## Promotion Rules

When a learning is broadly applicable beyond a single project:
1. **Project-level**: Add to the project's `.agent/` config or README
2. **Global-level**: Add to user's global rules in Antigravity settings
3. **Knowledge Items**: Create or update a KI for persistent cross-conversation knowledge

## When to Log

- After any error that took more than one attempt to fix
- When the user explicitly corrects behavior
- When you discover a tool, API, or framework behaves differently than expected
- When a workaround is needed for a known platform limitation (e.g., PowerShell `&&` restriction)
