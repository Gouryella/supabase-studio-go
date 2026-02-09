#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

LABEL="${LAUNCHD_LABEL:-com.$(whoami).supabase-studio-go}"
UID_NUM="$(id -u)"
TARGET="gui/${UID_NUM}/${LABEL}"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
PORT="${PORT:-3000}"
LOG_FILE="${LOG_FILE:-/tmp/supabase-studio-go-server.log}"
PID_FILE="${PID_FILE:-/tmp/supabase-studio-go.pid}"
BINARY="./bin/supabase-studio-go"
USE_LAUNCHD=0

if [[ "$(uname -s)" == "Darwin" ]] && command -v launchctl >/dev/null 2>&1; then
  USE_LAUNCHD=1
fi

usage() {
  echo "Usage: $0 <start|stop|restart|status> [--build]"
}

load_env() {
  set -a
  if [[ -f ".env.local" ]]; then
    source ".env.local"
  elif [[ -f ".env" ]]; then
    source ".env"
  elif [[ -f "./frontend/.env.local" ]]; then
    source "./frontend/.env.local"
  elif [[ -f "./frontend/.env" ]]; then
    source "./frontend/.env"
  fi
  set +a
}

build_if_needed() {
  local force_build="$1"
  if [[ "${force_build}" == "1" || ! -x "${BINARY}" ]]; then
    echo "Building supabase-studio-go binary..."
    go build -o "${BINARY}" ./cmd/studio
  fi
}

wait_health() {
  local retries=40
  local i
  for i in $(seq 1 "${retries}"); do
    if curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

ensure_launchd_plist() {
  mkdir -p "$(dirname "${PLIST}")"
  cat > "${PLIST}" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>${LABEL}</string>
    <key>ProgramArguments</key>
    <array>
      <string>/bin/zsh</string>
      <string>-lc</string>
      <string>cd '${SCRIPT_DIR}'; set -a; if [ -f '.env.local' ]; then source '.env.local'; elif [ -f '.env' ]; then source '.env'; elif [ -f './frontend/.env.local' ]; then source './frontend/.env.local'; elif [ -f './frontend/.env' ]; then source './frontend/.env'; fi; set +a; exec '${BINARY}'</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>${SCRIPT_DIR}</string>
    <key>StandardOutPath</key>
    <string>${LOG_FILE}</string>
    <key>StandardErrorPath</key>
    <string>${LOG_FILE}</string>
  </dict>
</plist>
PLIST
}

start_service() {
  local force_build="$1"
  build_if_needed "${force_build}"

  if [[ "${USE_LAUNCHD}" == "1" ]]; then
    ensure_launchd_plist
    launchctl bootstrap "gui/${UID_NUM}" "${PLIST}" >/dev/null 2>&1 || true
    launchctl kickstart -k "${TARGET}"
  else
    load_env
    nohup "${BINARY}" >"${LOG_FILE}" 2>&1 < /dev/null &
    echo "$!" > "${PID_FILE}"
  fi

  if ! wait_health; then
    echo "Start failed: health check not ready on :${PORT}" >&2
    [[ "${USE_LAUNCHD}" == "1" ]] && launchctl print "${TARGET}" | sed -n '1,80p' || true
    tail -n 80 "${LOG_FILE}" || true
    exit 1
  fi

  echo "supabase-studio-go started on :${PORT}"
}

stop_service() {
  if [[ "${USE_LAUNCHD}" == "1" ]]; then
    launchctl bootout "${TARGET}" >/dev/null 2>&1 || true
  fi

  local pids
  pids="$(lsof -ti tcp:${PORT} 2>/dev/null || true)"
  if [[ -n "${pids}" ]]; then
    kill ${pids} >/dev/null 2>&1 || true
  fi
  rm -f "${PID_FILE}"
  echo "supabase-studio-go stopped"
}

status_service() {
  lsof -nP -iTCP:${PORT} -sTCP:LISTEN || true
  if [[ "${USE_LAUNCHD}" == "1" ]]; then
    launchctl print "${TARGET}" >/dev/null 2>&1 && echo "launchd: loaded (${TARGET})" || echo "launchd: not loaded"
  fi
}

ACTION="${1:-}"
FORCE_BUILD=0
if [[ "${2:-}" == "--build" || "${1:-}" == "--build" ]]; then
  FORCE_BUILD=1
fi

case "${ACTION}" in
  start)
    start_service "${FORCE_BUILD}"
    ;;
  stop)
    stop_service
    ;;
  restart)
    stop_service
    start_service "${FORCE_BUILD}"
    ;;
  status)
    status_service
    ;;
  *)
    usage
    exit 1
    ;;
esac

if [[ "${ACTION}" == "start" || "${ACTION}" == "restart" ]]; then
  curl -sS "http://127.0.0.1:${PORT}/healthz"; echo
fi
