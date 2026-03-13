---
name: code-quality
description: Debugging methodology, linting workflows, build error triage, and runtime debugging for any language. Use when debugging issues, running linters, fixing build errors, reviewing code quality, triaging runtime errors, or doing pre-commit checks. Triggers on any mention of debugging, linting, build failures, code review, quality checks, error investigation, or "why isn't this working". Also use when the user hits an error and needs help understanding what went wrong.
---

# Code Quality

Structured workflows for debugging, linting, build error triage, and quality verification.

## Linting Workflow

### Step 1: Detect the Linter

Identify the project's linter from config files. Check in this order:

| Language | Config Files | Default Linter |
|----------|-------------|----------------|
| JavaScript/TypeScript | `.eslintrc*`, `eslint.config.*`, `biome.json` | ESLint or Biome |
| Python | `pyproject.toml` (ruff/flake8), `.flake8`, `setup.cfg` | Ruff (preferred), then flake8 |
| Go | N/A (built-in) | `go vet` + `golangci-lint` if available |
| CSS | `.stylelintrc*` | Stylelint |
| Markdown | `.markdownlint*` | markdownlint-cli |
| Rust | N/A (built-in) | `cargo clippy` |

If no config exists, use the language's most common linter with default settings.

### Step 2: Run the Linter

```bash
# Auto-fix what's fixable first, then report remaining issues
npx eslint --fix .          # JS/TS
ruff check --fix .          # Python
go vet ./...                # Go
```

### Step 3: Interpret and Prioritize

Triage lint output by severity:
1. **Errors** - Fix immediately. These break builds or cause runtime failures
2. **Warnings** - Fix now if quick, otherwise note for later
3. **Style/formatting** - Auto-fix with formatters (`prettier`, `black`, `gofmt`)

Don't chase style warnings while debugging a functional issue. Fix the bug first.

### Step 4: Re-lint After Every Fix

Run the linter again after each round of fixes. Never assume a fix didn't introduce new issues.

---

## Debugging Methodology

Follow this process every time. Don't skip steps.

### Step 1: Reproduce

Before investigating, confirm you can trigger the bug reliably.
- Get the exact steps, inputs, or request that causes the failure
- If it's intermittent, note the frequency and conditions
- Capture the full error output (stack trace, logs, browser console)

### Step 2: Isolate

Narrow the scope. The goal is to find the smallest reproducer.
- **Binary search**: Comment out half the code. Does it still fail? Narrow further
- **Input reduction**: Simplify the input that triggers the bug
- **Environment check**: Does it fail in a different browser/OS/Node version?
- **Recent changes**: What changed since it last worked? Check `git log` and `git diff`

### Step 3: Hypothesize

Form a specific theory about what's wrong. Not "something is broken" but "the fetch call is returning stale cached data because the query params aren't included in the cache key."

### Step 4: Test the Hypothesis

Validate with the smallest possible change:
- Add targeted logging at the suspected failure point
- Inspect variable values at runtime (breakpoints, `console.log`, `print()`)
- Check network requests in browser devtools (if applicable)
- Read the relevant source code, not just the error message

### Step 5: Fix

Apply the minimal fix. Don't refactor while debugging - fix the bug, ship it, then clean up.

### Step 6: Verify

- Confirm the original reproduction case now passes
- Run the full test suite to check for regressions
- Run the linter (see Linting Workflow above)

### Common Debugging Tools

| Language | Tools |
|----------|-------|
| JavaScript | Browser DevTools (Console, Network, Sources), `debugger` statement, `console.trace()` |
| Python | `print()` debugging, `pdb`/`ipdb`, `traceback.print_exc()` |
| Go | `fmt.Printf`, `log.Printf`, Delve debugger |
| General | `git bisect` to find the commit that introduced the bug |

### Log-Based Debugging Patterns

When adding debug logging:
- Log **inputs and outputs** at function boundaries, not just "got here"
- Include **timestamps** for timing-related bugs
- Log the **actual values**, not just types: `console.log('user:', JSON.stringify(user))` not `console.log('user exists')`
- **Remove debug logs** before committing. Use `grep -rn "console.log\|TODO\|FIXME\|HACK"` to find stragglers

---

## Build Error Triage

### Dependency Issues

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| `MODULE_NOT_FOUND` / `Cannot find module` | Missing dependency | `npm install` / `pip install -r requirements.txt` |
| Version conflicts | Incompatible peer deps | Check `npm ls`, `pip check`, `go mod tidy` |
| Lock file mismatch | Lock file out of sync | Delete lock file + `node_modules`/`.venv`, reinstall |
| `EACCES` / permission errors | Global install issues | Use `npx` instead of global installs |

### TypeScript / Type Errors

- **Read the full error**: TS errors are verbose but precise. The answer is usually in the message
- `Type 'X' is not assignable to type 'Y'` - Check if you're passing the wrong shape or missing a field
- `Property 'X' does not exist on type 'Y'` - Interface mismatch or missing type assertion
- `Cannot find name 'X'` - Missing import or incorrect scope

### Import / Module Issues

- **Circular imports**: Look for A imports B imports A chains. Restructure or use lazy imports
- **Path resolution**: Check `tsconfig.json` paths, `package.json` exports, or module resolver config
- **ESM vs CJS**: `require()` in ESM context or `import` in CJS. Check `"type": "module"` in package.json

---

## Runtime Debugging

### Browser Console Patterns

| Error Pattern | What It Means |
|---------------|---------------|
| `Uncaught TypeError: Cannot read properties of undefined` | Accessing a property on `null`/`undefined`. Check the variable before the `.` |
| `CORS policy: No 'Access-Control-Allow-Origin'` | Backend needs CORS headers. Add them server-side, don't disable browser security |
| `404 Not Found` on API calls | Wrong URL, missing route, or server not running |
| `401 Unauthorized` / `403 Forbidden` | Auth token expired, missing, or insufficient permissions |
| `Failed to fetch` / `net::ERR_CONNECTION_REFUSED` | Server is down or wrong port |
| `Hydration mismatch` (React/Next.js) | Server HTML differs from client render. Check for browser-only APIs in SSR code |

### Network Request Debugging

1. Open browser DevTools > Network tab
2. Filter by XHR/Fetch to see API calls
3. Check: correct URL? correct method? correct headers? correct request body?
4. Read the response body for error details (not just the status code)

### Memory & Performance

Signs of memory leaks:
- Page gets slower over time
- Browser tab memory grows continuously in Task Manager
- `detached HTMLElement` entries in Heap Snapshot

Common causes:
- Event listeners not cleaned up (`addEventListener` without corresponding `removeEventListener`)
- Intervals/timeouts not cleared
- Closures holding references to large objects
- Growing arrays/maps that never shrink

---

## Quality Gates

Run these checks before considering any change complete.

### Gate 1: No AI Slop
- No generic/formulaic layouts (hero text + gradient + standard button)
- No filler-heavy language ("in today's fast-paced world", "seamless experience")
- No em dashes anywhere in text content
- No placeholder images - generate real assets with `generate_image`
- Code handles edge cases and production scenarios, not just the happy path
- UI feels premium, not like a minimum viable product

### Gate 2: Lint & Build
- Run the project's linter after every file change
- Fix all lint errors before proceeding
- Verify the project builds without errors (`go build`, `npm run build`, etc.)
- Run existing tests and confirm they pass

### Gate 3: Browser Verification (for UI projects)
- Open the running app in the browser after every UI change
- Take a screenshot to verify visual correctness
- Check for layout breaks, missing content, or visible error states
- Verify responsive behavior at mobile and desktop widths
- Check the browser console for JavaScript errors

### Gate 4: Code Hygiene
- All interactive elements have unique, descriptive IDs
- Semantic HTML5 elements used properly
- No hardcoded secrets or credentials in code
- Use 5-digit random ports for new services

### PowerShell-Specific
- Never use `&&` in PowerShell commands (use `;` instead)
- Use `-y` flag with `npx` for auto-install
- Always check `--help` before using new CLI tools
