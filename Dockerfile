# Build context: this directory
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
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

FROM --platform=$BUILDPLATFORM golang:1.25.7-alpine AS gobuilder
WORKDIR /app
ARG TARGETOS
ARG TARGETARCH
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
  CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /usr/local/bin/supabase-studio-go ./cmd/studio

RUN mkdir -p /tmp/rootfs/etc /tmp/rootfs/home/app && \
  cp /etc/ssl/certs/ca-certificates.crt /tmp/rootfs/etc/ca-certificates.crt && \
  printf 'app:x:10001:10001:app:/home/app:/sbin/nologin\n' > /tmp/rootfs/etc/passwd && \
  printf 'app:x:10001:\n' > /tmp/rootfs/etc/group

FROM alpine:3.21
COPY --from=gobuilder /tmp/rootfs/etc/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=gobuilder /tmp/rootfs/etc/passwd /etc/passwd
COPY --from=gobuilder /tmp/rootfs/etc/group /etc/group
COPY --chown=10001:10001 --from=gobuilder /tmp/rootfs/home/app /home/app
COPY --from=gobuilder /usr/local/bin/supabase-studio-go /usr/local/bin/supabase-studio-go
WORKDIR /home/app
USER 10001:10001
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/supabase-studio-go"]
