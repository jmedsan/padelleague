# PadelLeague

[![CI](https://github.com/jmedsan/padelleague/actions/workflows/ci.yml/badge.svg)](https://github.com/jmedsan/padelleague/actions/workflows/ci.yml)

Padel league management — competitions, matches, rankings and scheduling.

## What it does

A web app for organizing padel leagues. An admin creates competitions, assigns pairs, and generates round-robin fixtures. Players log in, negotiate match schedules through an in-app thread, submit scores, and confirm results. The system computes standings with proper padel rules (3 points per win, set/game diff, head-to-head tiebreakers).

### For players

- **Home dashboard** — next match, pending actions, recent results
- **Match thread** — schedule proposals, chat, score discussion in one place
- **Structured score entry** — set-by-set dropdowns, venue selection
- **Player profile** — win rate, streaks, per-competition stats
- **Notifications** — bell icon with unread count, email notifications

### For admins

- **Competition management** — create leagues/playoffs, assign pairs, generate fixtures
- **Invite-only registration** — admin generates invite links, no open signup
- **Dispute resolution** — review and resolve score disagreements
- **Penalty system** — apply/remove point penalties per pair
- **Payment tracking** — per-pair payment status with batch toggle
- **Venue management** — add/edit venues, used in dropdowns across the app

## Tech stack

- **Backend:** [PocketBase](https://pocketbase.io) v0.39 as a Go framework (custom binary, not vanilla)
- **Frontend:** Go templates + [HTMX](https://htmx.org) for server-rendered interactivity
- **Styling:** [Tailwind CSS](https://tailwindcss.com) v3 + [DaisyUI](https://daisyui.com) v4
- **Database:** SQLite (embedded via PocketBase)
- **Deployment:** Single binary with embedded templates and static assets

## Prerequisites

- Go 1.27+
- Node.js (for Tailwind CSS build only)

## Getting started

```bash
# Install frontend dependencies (first time only)
cd frontend && npm install && cd ..

# Build (compiles CSS + Go binary)
make build

# Run
./padelleague serve --http=0.0.0.0:8090
```

The app is available at `http://localhost:8090`. PocketBase admin UI is at `http://localhost:8090/_/`.

On first run, create a superuser:

```bash
./padelleague superuser create admin@example.com yourpassword
```

## Docker

```bash
docker build -t padelleague .
docker run -p 8090:8090 -v padelleague_data:/app/pb_data padelleague
```

The `-v` flag persists the SQLite database between container restarts.

## Environment variables

| Variable | Description |
|----------|-------------|
| `PB_ADMIN_EMAIL` | PocketBase superuser email |
| `PB_ADMIN_PASSWORD` | PocketBase superuser password |
| `APP_ADMIN_EMAIL` | App-level admin user email |
| `APP_ADMIN_PASSWORD` | App-level admin user password |
| `APP_PLAYER_EMAIL` | Seed player email (dev/test) |
| `APP_PLAYER_PASSWORD` | Seed player password (dev/test) |
| `VAPID_PUBLIC_KEY` | Web push VAPID public key |
| `VAPID_PRIVATE_KEY` | Web push VAPID private key |

## Project structure

```
main.go              # Entry point, wires packages together
config/              # Env-based configuration struct
handlers/            # HTTP handlers (auth, match, admin, thread, push, etc.)
hooks/               # PocketBase event hooks and cron jobs
middleware/          # Cookie auth bridge, admin role check
migrations/          # PocketBase schema migrations
render/              # Template rendering helpers
routes/              # Route registration
seed/                # Dev/test data seeding
views/               # Go HTML templates (layout + pages + partials)
static/              # CSS, JS, images, manifest.json, sw.js (PWA)
frontend/            # Tailwind config + input CSS (build only)
```

## CI

Every push and pull request runs format check, lint, tests with race detector, vulnerability scan, and Docker build via GitHub Actions. Enable branch protection on `main` requiring the `gate` job to pass before merging.

## UI language

All user-facing text is in Spanish. Code (Go, HTML, CSS classes) is in English.
