# iTaK Agent Cloud (SaaS)

**The full business automation platform.** Everything in the open-source agent, plus Agency CRM, 10+ payment gateways, phone system, and multi-user team management.

> This is the **hosted/enterprise** branch. For the free self-hosted version, see the [`main` branch](https://github.com/David2024patton/itak-agent/tree/main).

---

## Features

### Core (included in all tiers)
- **AI Chat Bot** - Multi-persona chat with knowledge base, vision, and file attachments
- **Knowledge Base** - Document ingestion, repo scanning, semantic search
- **System Monitoring** - Real-time analytics, logs, and health checks
- **16+ LLM Backends** - Ollama, LM Studio, iTaK Torch, vLLM, OpenAI, Anthropic, Google, and more

### Pro ($29/mo)
- **Autonomous Agents** - Deploy researcher, coder, browser, and voice agents
- **Task Orchestration** - Kanban boards, priority scheduling, auto-execution
- **Automations** - Cron scheduler, calendar, report generation
- **Database Explorer** - Visual graph explorer with SQL queries
- **Marketplace** - Browse and deploy agent templates

### Agency ($99/mo)
- **Full CRM** - Contacts, deal pipelines, conversation inbox, sub-accounts
- **10+ Payment Gateways** - Stripe, PayPal, Square, Zelle (JPMC), GoCardless, Authorize.net
- **Phone System** - Twilio, Vonage, Plivo (SMS, calls, number management)
- **Sites & Funnels** - Landing page builder
- **Social Planner** - Multi-platform scheduling
- **Reputation Management** - Review monitoring and response
- **Brand Boards** - Visual brand guidelines
- **Memberships** - Course and community management
- **Media Library** - Centralized asset management
- **Workflow Automation** - Multi-step business process flows

---

## Architecture

```
SaaS Platform
├── Landing Page          # Registration, login, pricing (landing.html)
├── Auth System           # JWT + bcrypt, tier management (auth.go)
├── Dashboard             # Tier-gated SPA (index.html + app.js)
├── Connector Framework   # 10 built-in connectors
│   ├── Payments          # Stripe, PayPal, Square, Zelle, GoCardless, Authorize.net
│   ├── Phone             # Twilio, Vonage, Plivo
│   └── CRM               # GoHighLevel
├── Tenant Isolation      # Per-user data namespacing + security sandbox
└── Database              # bbolt (users, tenants, billing) + external backup
```

---

## Deployment

```bash
# Build and run
docker compose up -d --build

# Access
http://localhost:42800          # Landing page (register/login)
http://localhost:42800/dashboard  # Dashboard (requires auth)
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `ITAK_API_PORT` | `42800` | Dashboard + API port |
| `ITAK_DATA_DIR` | `./data` | Data storage directory |
| `ITAK_JWT_SECRET` | *(auto-generated)* | JWT signing key |
| `ITAK_BACKUP_INTERVAL` | `24h` | Auto-backup interval |
| `ITAK_BACKUP_TARGET` | *(none)* | Backup destination (path or scp://...) |

---

## API Endpoints

### Auth
| Endpoint | Method | Description |
|---|---|---|
| `/v1/auth/register` | POST | Create account |
| `/v1/auth/login` | POST | Authenticate |
| `/v1/auth/me` | GET | Get current user |
| `/v1/auth/logout` | POST | Clear session |
| `/v1/auth/upgrade` | POST | Change tier |

### Connectors
| Endpoint | Method | Description |
|---|---|---|
| `/v1/connectors` | GET | List all connectors |
| `/v1/connectors/{id}/setup` | POST | Configure connector credentials |
| `/v1/connectors/{id}/actions/{action}` | POST | Execute connector action |

---

## Tier Comparison

| Feature | Starter (Free) | Pro ($29/mo) | Agency ($99/mo) |
|---|---|---|---|
| AI Chat | Yes | Yes | Yes |
| Knowledge Base | Yes | Yes | Yes |
| System Monitor | Yes | Yes | Yes |
| LLM Backends | All 16+ | All 16+ | All 16+ |
| Autonomous Agents | - | Yes | Yes |
| Task Board | - | Yes | Yes |
| Automations | - | Yes | Yes |
| Database Explorer | - | Yes | Yes |
| Agency CRM | - | - | Yes |
| Payment Gateways | - | - | Yes |
| Phone System | - | - | Yes |
| Sites & Funnels | - | - | Yes |
| Social Planner | - | - | Yes |

---

## Security

- **Password hashing**: bcrypt with default cost
- **Session management**: HMAC-SHA256 JWT (httpOnly cookies)
- **Tenant isolation**: Per-user data namespacing
- **File system restrictions**: Users confined to their tenant directory
- **Code execution**: Disabled for hosted users
- **Storage quotas**: 500MB (Starter), 2GB (Pro), 10GB (Agency)

---

## License

Proprietary. Not for redistribution.
