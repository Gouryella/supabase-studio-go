# Supabase Studio Go

Supabase Studio Go is a lightweight Go runtime for the Supabase Studio web app.  
It serves the built frontend assets and implements the API routes needed by Studio in a single binary.

> This fork is optimized for lower resource usage and is tuned to use **10% less memory** than the official runtime in our environment.

## Why this project

- Reduce runtime overhead for self-hosted Supabase Studio deployments.
- Keep operational workflow simple with one Go process.
- Preserve compatibility with Studio frontend behavior and API expectations.
- Support both local development and containerized deployment.

## Tech stack

- **Backend runtime:** Go + Chi router
- **Frontend:** Next.js static export (embedded into the Go binary)
- **Container:** Multi-stage Docker build (Node + Go + Alpine runtime)

## Quick start (local)

### Prerequisites

- Go `1.25+`
- Node.js `22+`
- `pnpm` `10+`

### Build and run

```bash
# Build frontend and embed static files
make assets

# Build Go server
make build

# Run on :3000 (default)
make run
```

The server listens on `:3000` by default. You can override it with:

- `SUPABASE_STUDIO_GO_LISTEN` (preferred)
- `STUDIO_GO_LISTEN` (legacy compatibility)

## Runtime management

```bash
make start    # daemon / launchd mode
make status
make stop
make restart
```

## Docker

```bash
# Build image
docker build -f Dockerfile -t supabase-studio-go .

# Run container (example)
docker run --rm -p 3000:3000 --env-file .env supabase-studio-go
```

## Health check

- `GET /healthz` returns `{"status":"ok"}` when the service is ready.

## Testing

```bash
make test
```

## Repository

- GitHub: `https://github.com/Gouryella/supabase-studio-go`
