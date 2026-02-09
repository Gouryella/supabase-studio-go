.PHONY: frontend assets build run start stop restart status test clean docker

FRONTEND_DIR := ./frontend
STUDIO_OUT := $(FRONTEND_DIR)/out
STATIC_DIR := internal/server/static
ENV_FILE_LOCAL := .env.local
ENV_FILE := .env
ENV_FILE_FALLBACK_LOCAL := $(FRONTEND_DIR)/.env.local
ENV_FILE_FALLBACK := $(FRONTEND_DIR)/.env
PORT ?= 3000
PROJECT_DIR := $(CURDIR)
LOG_FILE ?= /tmp/supabase-studio-go-server.log
PID_FILE ?= /tmp/supabase-studio-go.pid
LAUNCHD_LABEL ?= com.$(shell whoami).supabase-studio-go
LAUNCHD_TARGET := gui/$(shell id -u)/$(LAUNCHD_LABEL)
LAUNCHD_PLIST := $(HOME)/Library/LaunchAgents/$(LAUNCHD_LABEL).plist

frontend:
	@if [ ! -d "$(FRONTEND_DIR)/node_modules" ]; then pnpm -C $(FRONTEND_DIR) install --frozen-lockfile; fi
	pnpm -C $(FRONTEND_DIR) exec next build

assets: frontend
	rm -rf $(STATIC_DIR)
	mkdir -p $(STATIC_DIR)
	cp -R $(STUDIO_OUT)/* $(STATIC_DIR)/

build:
	go build -o bin/supabase-studio-go ./cmd/studio

run: build
	@set -a; \
	if [ -f $(ENV_FILE_LOCAL) ]; then . $(ENV_FILE_LOCAL); \
	elif [ -f $(ENV_FILE) ]; then . $(ENV_FILE); \
	elif [ -f $(ENV_FILE_FALLBACK_LOCAL) ]; then . $(ENV_FILE_FALLBACK_LOCAL); \
	elif [ -f $(ENV_FILE_FALLBACK) ]; then . $(ENV_FILE_FALLBACK); fi; \
	set +a; \
	./bin/supabase-studio-go

start: build
	@set -eu; \
	if [ "$$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then \
		mkdir -p "$$(dirname '$(LAUNCHD_PLIST)')"; \
		if [ ! -f "$(LAUNCHD_PLIST)" ]; then \
			printf '%s\n' \
'<?xml version="1.0" encoding="UTF-8"?>' \
'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
'<plist version="1.0">' \
'  <dict>' \
'    <key>Label</key>' \
'    <string>$(LAUNCHD_LABEL)</string>' \
'    <key>ProgramArguments</key>' \
'    <array>' \
'      <string>/bin/zsh</string>' \
'      <string>-lc</string>' \
'      <string>cd $(PROJECT_DIR); set -a; if [ -f $(ENV_FILE_LOCAL) ]; then source $(ENV_FILE_LOCAL); elif [ -f $(ENV_FILE) ]; then source $(ENV_FILE); elif [ -f $(ENV_FILE_FALLBACK_LOCAL) ]; then source $(ENV_FILE_FALLBACK_LOCAL); elif [ -f $(ENV_FILE_FALLBACK) ]; then source $(ENV_FILE_FALLBACK); fi; set +a; exec ./bin/supabase-studio-go</string>' \
'    </array>' \
'    <key>RunAtLoad</key>' \
'    <true/>' \
'    <key>KeepAlive</key>' \
'    <true/>' \
'    <key>WorkingDirectory</key>' \
'    <string>$(PROJECT_DIR)</string>' \
'    <key>StandardOutPath</key>' \
'    <string>$(LOG_FILE)</string>' \
'    <key>StandardErrorPath</key>' \
'    <string>$(LOG_FILE)</string>' \
'  </dict>' \
'</plist>' > "$(LAUNCHD_PLIST)"; \
		fi; \
		launchctl bootstrap "gui/$$(id -u)" "$(LAUNCHD_PLIST)" >/dev/null 2>&1 || true; \
		launchctl kickstart -k "$(LAUNCHD_TARGET)"; \
		echo "Started launchd service: $(LAUNCHD_TARGET)"; \
	else \
		set -a; \
		if [ -f $(ENV_FILE_LOCAL) ]; then . $(ENV_FILE_LOCAL); \
		elif [ -f $(ENV_FILE) ]; then . $(ENV_FILE); \
		elif [ -f $(ENV_FILE_FALLBACK_LOCAL) ]; then . $(ENV_FILE_FALLBACK_LOCAL); \
		elif [ -f $(ENV_FILE_FALLBACK) ]; then . $(ENV_FILE_FALLBACK); fi; \
		set +a; \
		nohup ./bin/supabase-studio-go >"$(LOG_FILE)" 2>&1 < /dev/null & \
		echo $$! >"$(PID_FILE)"; \
		echo "Started pid $$!"; \
	fi; \
	i=0; \
	until curl -fsS "http://127.0.0.1:$(PORT)/healthz" >/dev/null 2>&1; do \
		i=$$((i + 1)); \
		if [ $$i -ge 40 ]; then \
			echo "Startup health check failed on :$(PORT)"; \
			tail -n 80 "$(LOG_FILE)" 2>/dev/null || true; \
			exit 1; \
		fi; \
		sleep 0.25; \
	done; \
	echo "supabase-studio-go is healthy on :$(PORT)"

status:
	@lsof -nP -iTCP:$(PORT) -sTCP:LISTEN || true; \
	if [ "$$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then \
		launchctl print "$(LAUNCHD_TARGET)" >/dev/null 2>&1 && echo "launchd: loaded ($(LAUNCHD_TARGET))" || echo "launchd: not loaded"; \
	fi

stop:
	@set -eu; \
	if [ "$$(uname -s)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then \
		if launchctl print "$(LAUNCHD_TARGET)" >/dev/null 2>&1; then \
			launchctl bootout "$(LAUNCHD_TARGET)" >/dev/null 2>&1 || true; \
			echo "Stopped launchd service: $(LAUNCHD_TARGET)"; \
		fi; \
	fi; \
	PIDS=$$(lsof -ti tcp:$(PORT) 2>/dev/null || true); \
	if [ -n "$$PIDS" ]; then \
		echo "Stopping supabase-studio-go on :$(PORT) (pid: $$PIDS)"; \
		kill $$PIDS || true; \
	else \
		echo "No process listening on :$(PORT)"; \
	fi; \
	rm -f "$(PID_FILE)"

restart: stop start

test:
	go test ./...

clean:
	rm -rf $(STATIC_DIR) bin

docker:
	docker build . -f Dockerfile -t supabase-studio-go
