# PadelLeague

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

## Project structure

```
main.go              # Routes, middleware, template rendering
handlers/            # Request handlers (auth, matches, admin, etc.)
middleware/          # Cookie auth bridge, admin role check
migrations/          # PocketBase schema migrations
views/               # Go HTML templates (layout + pages)
views/admin/         # Admin-only templates
static/css/          # Tailwind CSS (built from frontend/)
frontend/            # Tailwind config + input CSS
```

## UI language

All user-facing text is in Spanish. Code (Go, HTML, CSS classes) is in English.
