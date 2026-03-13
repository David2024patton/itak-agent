---
name: swarm
description: Multi-project orchestration and parallel workspace management. Use when coordinating changes across multiple codebases, managing related projects, or when the user wants to work on several things simultaneously.
---

# Multi-Project Swarm Orchestration

Coordinate work across multiple projects and workspaces. Inspired by the EY model of "orchestrators directing agents" and Antigravity's Agent Manager capabilities.

## Core Concept

Instead of working on one project at a time, the swarm pattern treats the user as an orchestrator managing a portfolio of interconnected projects. The agent should understand the relationships between projects and coordinate changes that span multiple codebases.

## David's Active Project Map

### Infrastructure Layer
- **iTaK Framework** - Core AI agent framework (Go)
- **iTaK Shield** - Security proxy with embedded web dashboard (Go)
- **VPS/Skynet** - Docker services (Neo4j, SearXNG, Open WebUI)

### Application Layer
- **Oath Bot** - Discord attendance bot with web dashboard (Python/aiohttp)
- **Clarity AI** - Counseling diagnostics platform (Next.js/Supabase)
- **AI Trading** - Polymarket prediction market tools (Python)

### Platform Layer
- **Multi-Tenant SaaS** - Pipeline hosting platform (FastAPI/Next.js)
- **OpenClaw Admin** - Admin dashboard for the ecosystem

## Cross-Project Patterns

### Shared Dependencies
When modifying a foundational component, check impact across:
1. **API changes in iTaK** - affects Shield, OpenClaw, SaaS platform
2. **Supabase schema changes** - affects Clarity AI, SaaS platform
3. **Docker compose changes** - affects all VPS-hosted services
4. **CSS/design system changes** - affects all frontends

### Coordinated Updates
When a change requires updates across projects:
1. **Identify all affected projects** from the map above
2. **Order changes by dependency** - infrastructure first, then applications
3. **Track cross-project changes** in the task.md artifact
4. **Verify each project independently** before moving to the next

### Common Cross-Cutting Tasks
- **Dependency upgrades**: Update a shared library across all projects
- **Design system updates**: Propagate color/typography changes to all frontends
- **API versioning**: Ensure backward compatibility when changing shared APIs
- **Credential rotation**: Update secrets across all services when credentials change

## Agent Manager Integration

Antigravity's Agent Manager enables:
- Toggle between Manager and Editor with `Ctrl+E`
- Oversee multiple agents working across workspaces
- Each workspace maps to one Editor instance
- Manager provides the bird's-eye orchestration view

## Task Delegation (from iTaK AGENTS.md)

### When to Delegate
- Task is complex and can be broken into independent subtasks
- A specialized role (researcher, coder, tester) would do a better job on a piece
- Parallel work would speed up delivery

### When NOT to Delegate
- Simple tasks that take one tool call
- Tasks that need your full conversation context
- Tasks where the user expects YOU to do it personally

### Delegation Rules
- Never delegate the full task to a subordinate with the same profile as you
- Always describe the role when creating a new subordinate
- Subordinates inherit your context but get their own history
- Results bubble up - don't repeat work your subordinate already did
- Limit the sub-agent's tool access to what it actually needs

### Common Role Assignments
| Role | Tools Needed | Use For |
|------|-------------|--------|
| Researcher | web search, memory | Finding documentation, comparing solutions |
| Coder | code execution, file ops | Writing code, tests, refactoring |
| DevOps | docker, ssh, terminal | Deployment, infrastructure changes |
| Tester | code execution, browser | Running tests, visual verification |

## Execution Strategy

### Flow Types (from Agent Zero)

#### Sequential (Pipeline)
Agents work one after another. Each agent receives the previous agent's output.
```
Agent 1 -> Agent 2 -> Agent 3 -> Final Output
```
Best for: Refinement workflows, code review chains, multi-step analysis

#### Parallel (Concurrent)
All agents work on the task simultaneously. Results are collected and combined.
```
Agent 1 >
Agent 2 > -> Combined Output
Agent 3 >
```
Best for: Research, brainstorming, multi-perspective analysis

#### Tips
- Keep agent count at 2-5 (more adds overhead)
- Sequential for depth, parallel for breadth
- Be specific in task descriptions for each agent

### For Small Cross-Project Tasks
1. Make changes in dependency order
2. Verify each change before moving on
3. Document all changes in a single walkthrough

### For Large Cross-Project Tasks
1. Create an implementation plan covering all affected projects
2. Group changes into phases (infra, backend, frontend)
3. Execute one phase at a time
4. Run integration tests between phases
