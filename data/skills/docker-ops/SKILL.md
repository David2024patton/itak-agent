---
name: docker-ops
description: Docker container management, compose operations, health checks, and troubleshooting. Use when working with Docker containers, deploying services, debugging container issues, or managing volumes.
---

# Docker Operations

Ported from iTaK `skills/docker_ops.md` and enriched with David's infrastructure patterns.

## Container Lifecycle

### Start services

```bash
docker compose up -d              # All services
docker compose up -d neo4j        # Single service
docker compose up -d --build      # Rebuild and start
```

### Stop services

```bash
docker compose down               # Stop all
docker compose down -v            # Stop and remove volumes (data loss!)
docker compose stop neo4j         # Stop single service
```

### Check status

```bash
docker compose ps                 # All containers
docker compose logs -f            # Follow all logs
docker compose logs -f itak       # Follow specific service
docker compose logs --tail=50     # Last 50 lines
```

### Debug a container

```bash
docker exec -it <container> bash          # Shell into running container
docker inspect <container>                # Full container details
docker stats                              # Resource usage
docker compose config                     # Validate compose file
```

## Common Operations

### Restart a crashed service

```bash
docker compose restart neo4j
```

### Update a service without downtime

```bash
docker compose pull neo4j                # Pull latest image
docker compose up -d --no-deps neo4j     # Restart just that service
```

### Clean up

```bash
docker system prune -f                   # Remove unused containers/images
docker volume prune -f                   # Remove unused volumes
docker image prune -a -f                 # Remove all unused images
```

### Backup volumes

```powershell
# PowerShell
docker run --rm -v neo4j-data:/data -v "${PWD}:/backup" alpine tar czf /backup/neo4j-backup.tar.gz /data
```

```bash
# Linux/SSH
docker run --rm -v neo4j-data:/data -v $(pwd):/backup alpine tar czf /backup/neo4j-backup.tar.gz /data
```

## iTaK Ecosystem Port Mapping

| Service | Container Port | Host Port |
|---------|---------------|-----------|
| Neo4j HTTP | 7474 | 47474 |
| Neo4j Bolt | 7687 | 47687 |
| Weaviate | 8080 | 48080 |
| SearXNG | 8080 | 48888 |
| WebUI | 8920 | 48920 |

> **Rule**: Always use obfuscated 5-digit ports for new services.

## Health Checks

### Neo4j

```bash
curl -s http://localhost:47474 | head -1
```

### Weaviate

```bash
curl -s http://localhost:48080/v1/.well-known/ready
```

### SearXNG

```bash
curl -s http://localhost:48888/healthz
```

## VPS Operations (Skynet: 192.168.0.217)

```bash
# SSH to VPS
ssh skynet@192.168.0.217

# Check all running containers on VPS
ssh skynet@192.168.0.217 "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'"

# View logs remotely
ssh skynet@192.168.0.217 "docker compose -f /path/to/docker-compose.yml logs --tail=50"
```

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Port already in use | `docker compose down` then `docker compose up -d` |
| Volume permission denied | `chmod -R 777 ./data` or recreate volumes |
| Container keeps restarting | Check logs: `docker compose logs <service>` |
| Out of disk space | `docker system prune -a -f` |
| Network issues between services | Ensure all on same network |
| Neo4j 15-min lockout | Wait 15 minutes or restart the container |
| Neo4j credential reset | Stop container, delete volume, restart (first-boot restriction) |

## PowerShell Equivalents (Windows)

```powershell
# List containers
docker ps -a

# Container logs
docker logs -f CONTAINER

# Compose up/down
docker compose up -d
docker compose down
```
