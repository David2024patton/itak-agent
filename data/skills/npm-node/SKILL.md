---
name: npm-node
description: Node.js and npm project operations. Use when initializing projects, managing dependencies, running scripts, debugging build issues, or working with package.json.
---

# NPM & Node.js Operations

Standard patterns for Node.js project management across all of David's projects.

## Project Initialization

### New Vite Project
```powershell
npx -y create-vite@latest ./ --template react-ts
npm install
```

### New Next.js Project
```powershell
npx -y create-next-app@latest ./ --typescript --eslint --tailwind --app --src-dir
```

**Rules:**
- Always run `--help` first to see available options
- Use `npx -y` to auto-install
- Initialize in current directory with `./`
- Run in non-interactive mode

## Dependency Management

### Install
```powershell
npm install <package>
npm install -D <dev-package>
```

### Check for vulnerabilities
```powershell
npm audit
npm audit fix
```

### Check outdated
```powershell
npm outdated
```

## Development Server

### Start dev server
```powershell
npm run dev
```

- Always use dev server during development, not production builds
- Only build production bundle if user explicitly requests it
- Use 5-digit random ports for new services to avoid conflicts

## Common Scripts
```json
{
  "dev": "vite",
  "build": "vite build", 
  "preview": "vite preview",
  "lint": "eslint .",
  "test": "vitest"
}
```

## Troubleshooting

### Module not found
```powershell
# Clear cache and reinstall
Remove-Item -Recurse -Force node_modules
Remove-Item package-lock.json
npm install
```

### Port in use
```powershell
# Find process on port
netstat -ano | Select-String ":<port>"
# Kill by PID
Stop-Process -Id <PID> -Force
```

### TypeScript errors
- Run `npx tsc --noEmit` to check types without building
- Check `tsconfig.json` for path aliases and strict mode settings

## Build Verification
After any dependency change or config update:
1. Run `npm run build` to verify production build succeeds
2. Run `npm run lint` to check for lint errors
3. Fix all errors before committing
