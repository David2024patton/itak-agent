---
name: github-ops
description: GitHub repository operations including cloning, branching, PRs, and release management. Use when working with Git repos, creating branches, pushing changes, or managing releases.
---

# GitHub Operations

Standard patterns for all Git and GitHub interactions across David's projects.

## Repository Setup

### Clone
```powershell
git clone https://github.com/<owner>/<repo>.git
```

### Check Status Before Any Work
```powershell
git status
git log -n 5 --oneline
```

## Branching Strategy

### Feature Work
```powershell
git checkout -b feature/<descriptive-name>
```

### Bug Fixes
```powershell
git checkout -b fix/<issue-description>
```

### Always pull before branching
```powershell
git pull origin main
git checkout -b feature/<name>
```

## Commit Standards

### Format
```
<type>: <short description>

<optional body with details>
```

### Types
- `feat`: New feature
- `fix`: Bug fix
- `refactor`: Code restructuring
- `docs`: Documentation changes
- `style`: CSS/formatting changes
- `test`: Adding or updating tests
- `chore`: Build/tooling changes

### Examples
```
feat: add Models tab to Shield dashboard
fix: resolve 404 on /api/models endpoint
refactor: extract token formatting into helper function
```

## Pull Requests
1. Push branch: `git push origin feature/<name>`
2. Create PR with descriptive title and body
3. Link related issues
4. Request review if collaborators exist

## Release Tagging
```powershell
git tag -a v1.0.0 -m "Release v1.0.0: description"
git push origin v1.0.0
```

## Common Fixes

### Undo last commit (keep changes)
```powershell
git reset --soft HEAD~1
```

### Stash changes
```powershell
git stash
git stash pop
```

### Force pull (discard local)
```powershell
git fetch origin
git reset --hard origin/main
```

## Private Repo Access

David has private repos (e.g., `iTaK`). Access them with the PAT from `creds.md`:

```powershell
$headers = @{ Authorization = "Bearer $PAT"; Accept = "application/vnd.github+json" }
Invoke-RestMethod -Uri "https://api.github.com/repos/David2024patton/<repo>/contents/" -Headers $headers
```

Or check if the repo is cloned locally first (usually under `e:\.agent\`).

## David's Repos
- GitHub profile: https://github.com/David2024patton
- Check active repos before starting work on any project
- Many repos are cloned locally at `e:\.agent\`

## Additional Errors (from iTaK)

| Error | Fix |
|-------|-----|
| Not a git repository | `git init` or clone first |
| Detached HEAD | Checkout a proper branch |
| Push rejected | Pull latest changes first |
| Merge conflicts | Resolve manually, then commit |
