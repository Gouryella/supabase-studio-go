# Build context: this directory
FROM node:22-alpine AS frontend
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"

RUN apk add --no-cache git python3 ca-certificates build-base && \
  update-ca-certificates

RUN npm install -g pnpm@10.16.1

WORKDIR /app
COPY frontend ./frontend

RUN pnpm -C /app/frontend install --frozen-lockfile
RUN pnpm -C /app/frontend exec next build

RUN rm -rf /app/static && \
  mkdir -p /app/static && \
  cp -R /app/frontend/out/* /app/static/

FROM golang:1.25.7-alpine AS gobuilder
WORKDIR /app
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
RUN apk add --no-cache git ca-certificates && \
  update-ca-certificates

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  sh -ec 'for attempt in 1 2 3; do go mod download && exit 0; echo "go mod download failed (attempt ${attempt}/3), retrying..." >&2; sleep $((attempt * 5)); done; exit 1'

COPY . .
RUN rm -rf internal/server/static
COPY --from=frontend /app/static /app/internal/server/static

RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=0 go build -o /usr/local/bin/supabase-studio-go ./cmd/studio

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && \
  addgroup -S app && \
  adduser -S -G app app && \
  mkdir -p /home/app && \
  chown app:app /home/app
COPY --from=gobuilder /usr/local/bin/supabase-studio-go /usr/local/bin/supabase-studio-go
WORKDIR /home/app
USER app
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/supabase-studio-go"]
