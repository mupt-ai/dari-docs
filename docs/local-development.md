# Local Development

Run Postgres, the managed-service backend, and the Vite frontend together with Docker Compose:

```bash
cp .env.example .env
# Fill in SUPABASE_URL and SUPABASE_PUBLISHABLE_KEY.
docker compose up
```

Docker chooses open localhost ports by default. Find them with:

```bash
docker compose port frontend 5173
docker compose port backend 8080
docker compose port postgres 5432
```

The compose file supplies local placeholder service secrets so the backend can boot and run migrations. To exercise real Dari-managed runs, add real values to `.env` before starting compose:

```bash
DARI_API_KEY=...
MANAGED_TESTER_AGENT_ID=...
MANAGED_EDITOR_AGENT_ID=...
```
