---
name: database-ops
description: Database operations for Supabase, Neo4j, and SQLite. Use when creating schemas, running migrations, managing RLS policies, writing queries, or troubleshooting database connectivity.
---

# Database Operations

Unified guide for all database operations across David's projects.

## Supabase (Primary Cloud DB)

### Connection
- Dashboard: Check `creds.md` for project URLs and keys
- Client: Use `@supabase/supabase-js` or Python `supabase` client
- Always use environment variables for keys, never hardcode

### Schema Changes
1. Write migration SQL in `supabase/migrations/` directory
2. Use `ALTER TABLE` for modifications, never drop and recreate in production
3. Always add `IF NOT EXISTS` guards on `CREATE TABLE`
4. Include `created_at TIMESTAMPTZ DEFAULT NOW()` on every table

### Row Level Security (RLS)
- Enable RLS on every table: `ALTER TABLE <table> ENABLE ROW LEVEL SECURITY`
- Known gotcha: Recursive RLS policies cause infinite loops - use `SECURITY DEFINER` functions to break cycles
- Test policies with `SET ROLE authenticated` before deploying

### Common Patterns
```sql
-- Standard user-scoped read policy
CREATE POLICY "Users read own data" ON <table>
  FOR SELECT USING (auth.uid() = user_id);

-- Service role bypass for admin operations  
CREATE POLICY "Service role full access" ON <table>
  USING (auth.role() = 'service_role');
```

## Neo4j (Graph Database)

### Connection
- Hosted on VPS (Skynet: 192.168.0.217) or Dokploy
- Admin username is ALWAYS `neo4j` (mandatory, cannot be changed)
- Check `creds.md` for current password
- Use obfuscated 5-digit ports

### Critical Gotchas
- **First-Boot Restriction**: Credentials can only be set on first launch. To reset: stop container, delete the volume, restart
- **15-Minute Lockout**: After failed auth attempts, Neo4j locks for 15 minutes. Wait or restart the container
- **APOC Plugin**: Must be mounted as a volume, not installed at runtime

### Cypher Patterns
```cypher
// Create node with properties
CREATE (n:Entity {name: $name, created: datetime()})
RETURN n

// Full-text search
CALL db.index.fulltext.queryNodes('searchIndex', $query)
YIELD node, score
RETURN node, score ORDER BY score DESC
```

### Knowledge Graph Relationships (from iTaK)

Use these standard relationship types for mapping entities:

| Type | Example |
|------|---------|
| `uses` | iTaK uses FastAPI |
| `built_by` | iTaK built_by David |
| `runs_on` | Neo4j runs_on VPS |
| `depends_on` | WebUI depends_on FastAPI |
| `related_to` | Polymarket related_to trading |

```cypher
// Save a project relationship
MERGE (a:Entity {name: $entity})
SET a.type = $entity_type
MERGE (b:Entity {name: $related_to})
MERGE (a)-[r:RELATES {type: $relationship}]->(b)
RETURN a, r, b

// Get all context for an entity
MATCH (n:Entity {name: $entity})-[r]-(connected)
RETURN n, r, connected
```

## SQLite (Local/Embedded)
- Use for local state, caching, and development databases
- Always use WAL mode for concurrent reads: `PRAGMA journal_mode=WAL`
- Back up before schema migrations: copy the `.db` file
