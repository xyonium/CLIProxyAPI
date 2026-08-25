# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

## CI and contribution model (not in AGENTS.md)

- **PRs target `dev`, not `main`.** The `auto-retarget-main-pr-to-dev` workflow retargets any PR opened against `main` to `dev`. Branch from `dev` and open PRs against `dev` to avoid churn. (This overrides the harness's default suggestion of `main`.)
- **CI only builds — it does not run `go test`.** `pr-test-build.yml` runs `go build ./cmd/server` and nothing else. Run `go test ./...` locally; it is the only test signal before merge.
- **`AGENTS.md` is frozen by CI.** `agents-md-guard.yml` auto-closes any PR that modifies `AGENTS.md` (or any `*/AGENTS.md`). Do not edit it; add Claude-Code-specific guidance here instead.
- **`internal/translator/` is frozen by CI.** `pr-path-guard.yml` fails any PR touching `internal/translator/**`. This is the CI-backed enforcement of the AGENTS.md convention — even with repo write access, translator changes cannot land via PR and must go through a maintainer issue.
- **CI refreshes the models catalog before building.** `pr-test-build.yml` fetches `models.json` from the separate `router-for-me/models` repo into `internal/registry/models/models.json`. A stale local `models.json` can cause build or lookup drift; re-fetch it if you see model-catalog mismatches.

## Additional CLI flags (OAuth login flows)

These one-shot login flags on `cmd/server` are not listed in AGENTS.md but are commonly needed when setting up auth material under `auths/`:

- `--codex-login` / `--codex-device-login` — OAuth or device-code login for OpenAI Codex
- `--claude-login` — OAuth login for Claude Code
- `--antigravity-login` — OAuth login for Antigravity
- `--kimi-login` — OAuth login for Kimi
- `--xai-login` — OAuth login for xAI/Grok
- `--vertex-import <key.json>` (with optional `--vertex-import-prefix`) — import a Vertex AI service account key

Use `--no-browser` for headless OAuth flows, and `--oauth-callback-port <port>` to override the callback port.

## Standalone utilities

- `cmd/fetch_antigravity_models` and `cmd/fetch_codex_models` — small `main.go` utilities (not part of the server) that fetch provider model lists. Build/run independently when refreshing those catalogs.

## Local Docker build (this machine)

- **Network:** `proxy.golang.org` gets connection-reset from this host (github.com TLS is intermittently flaky too — just retry). `goproxy.cn` works. The tracked `Dockerfile` declares no `ARG GOPROXY`, so inject it at build time via a stdin Dockerfile (leaves the repo file untouched):

  ```bash
  awk '{if ($0 ~ /^RUN go mod download$/) print "ENV GOPROXY=https://goproxy.cn,direct"; print}' Dockerfile | \
  docker build --no-cache -f - -t cliproxy-api:local \
    --build-arg VERSION=dev --build-arg COMMIT=$(git rev-parse --short HEAD) \
    --build-arg BUILD_DATE=$(date -u +%Y-%m-%d) .
  ```

  Permanent alternative: add `ARG GOPROXY` + `ENV GOPROXY=${GOPROXY}` before `RUN go mod download` (harmless if merged upstream) and pass `--build-arg GOPROXY=https://goproxy.cn,direct`.
- **Image CI trigger:** `docker-image.yml` runs ONLY on `v*` tag pushes, publishing to DockerHub `eceasy/cli-proxy-api` (`:latest-amd64`, `:<tag>-amd64`, plus arm64 and a manifest). Pushing `main` builds nothing. Requires `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` secrets with push rights to that DockerHub repo.
- **Smoke test the baked-in version:** `docker run --rm cliproxy-api:local ./CLIProxyAPI --help` — the first line prints `Version/Commit/BuiltAt`. There is no `--version` flag, and CMD is `./CLIProxyAPI`, so the binary path must be given explicitly.

## Fork layout (xyonium/CLIProxyAPI)

- Fork `main` carries **zero unique commits** vs upstream — it mirrors upstream `main` plus occasional local patch commits. To re-sync: `git fetch https://github.com/router-for-me/CLIProxyAPI.git main`, then reset/rebase the local patches on top; as long as this discipline holds, pushing is a fast-forward. The GitHub remote `ER-EPR/CLIProxyAPI` redirects to `xyonium/CLIProxyAPI`.
- An abandoned pre-2026-08-25 WIP (unfinished upstream merge, conflict markers intact) is parked on local branch `backup/wip-merge-20260825`; safe to delete once confirmed obsolete.
- **Fork patch:** `sdk/cliproxy/service_auth.go:ensureWebsocketGateway` wires the previously-unused `wsrelay.Options.ProviderFactory` to the `?provider_name=` query param, so websocket clients register under a stable provider name instead of a random `aistudio-XXXX` on every reconnect. The `aistudio-` prefix is enforced there because `wsOnConnected` silently skips channels without it. Client side lives in the AIStudioBuildWS repo: `browser/instance.py` injects `window.__PROVIDER_NAME__`, and the AI Studio app (`websocket-proxy-logger/`) appends it to the WS URL.
