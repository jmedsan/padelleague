# PadelLeague

Padel league management — competitions, matches, rankings and scheduling.

## What it does

A web app for organizing padel leagues. An admin creates competitions, assigns pairs, and generates round-robin fixtures. Players log in, negotiate match schedules through an in-app thread, submit scores, and confirm results. The system computes standings with proper padel rules (3 points per win, set/game diff, head-to-head tiebreakers).

### For players

- **Urgent-tasks home** — the single most important next action first: an open dispute, your next confirmed match to play, or a match to organize before its deadline
- **Scheduling deadlines** — each league round shows a recommended "organize by" date; matches surface escalating warnings (próximo → urgente → vencido) with at most one reminder per step
- **Match thread** — schedule proposals, chat, and score discussion in one place
- **Structured score entry** — set-by-set input with valid padel scores, venue selection
- **Score confirmation with reminders** — one player submits a result, the opponent confirms or disputes; if it sits unconfirmed the app reminds the opponent before the quorum timeout resolves it
- **Add to calendar** — one-tap "Añadir al calendario" (`.ics`) on a scheduled match
- **Player profile** — win rate, per-competition stats
- **Notifications** — bell icon with unread count, email, and web push

### For admins

- **Competition management** — create leagues/playoffs, assign pairs, generate fixtures
- **League scheduling** — set start/end dates; the app computes and **stores a recommended arrange-by date per round** (admin-editable on the competition page, with a "regenerate" option), sends escalating reminders, and flags overdue matches
- **End-of-league recovery window** — a per-competition grace period after the end date (default 14 days, editable) during which still-pending matches show an "En recuperación" state and stay organizable; the admin can finalize a league early
- **Admin-approved walkovers** — when a team reports a match unplayed, the admin approves a walkover with a configurable default score (6-0 6-0) and points penalty; nothing is ever penalized automatically
- **Playoff brackets** — admin-set fixed dates enforced in bracket order (quarters → semis → final), with a mobile-friendly bracket view
- **Home dashboard** — a setup checklist before a league starts, and dispute/overdue/walkover alerts once it is active
- **Outstanding matches** — one view of every unresolved match across active competitions with its deadline and urgency, most-urgent first
- **Invite-only registration** — admin generates invite links, no open signup
- **Dispute resolution** — review and resolve score disagreements
- **Penalty system** — apply configurable, reasoned point penalties per pair; every application records its admin and timestamp, and removals retain the audit history
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
| `APP_ADMIN1_EMAIL` | First app-level admin user email |
| `APP_ADMIN1_PASSWORD` | First app-level admin user password |
| `APP_ADMIN1_NAME` | First admin display name (default: `Admin`) |
| `APP_ADMIN2_EMAIL` | Second app-level admin user email (optional) |
| `APP_ADMIN2_PASSWORD` | Second app-level admin user password (optional) |
| `APP_ADMIN2_NAME` | Second admin display name (default: `Admin 2`) |
| `APP_PLAYER_EMAIL` | Seed player email (dev/test) |
| `APP_PLAYER_PASSWORD` | Seed player password (dev/test) |
| `APP_PLAYER_NAME` | Seed player display name (default: `Jugador`) |
| `APP_PLAYER2_EMAIL` | Second seed player email (dev/test, optional) |
| `APP_PLAYER2_PASSWORD` | Second seed player password (dev/test) |
| `APP_PLAYER2_NAME` | Second seed player display name (default: `Jugador 2`) |
| `APP_ENV` | Environment: `dev` (default) or `prod`. `prod` skips the player seed |
| `APP_DEV_TOOLS` | `true` to enable the admin database reset tool (default: `false`) |
| `VAPID_PUBLIC_KEY` | Web push VAPID public key |
| `VAPID_PRIVATE_KEY` | Web push VAPID private key |

## Project structure

```
main.go              # Entry point, wires packages together
config/              # Env-based configuration struct
league/              # Domain logic (scoring, standings, fixtures, awards, quorum)
notify/              # Notification delivery (push, in-app, email)
handlers/            # HTTP handlers (thin: parse request, call domain, render)
hooks/               # PocketBase event hooks and cron jobs
middleware/          # Cookie auth bridge, admin role check
migrations/          # PocketBase schema migrations
render/              # Template rendering helpers
routes/              # Route registration with Deps struct
seed/                # Dev/test data seeding
views/               # Go HTML page templates (layout.html, pages, admin/)
views/partials/      # One shared partial per domain object; explicit modes select player, summary, or admin controls
static/              # CSS, JS, images, manifest.json, sw.js (PWA)
frontend/            # Tailwind config + input CSS (build only)
e2e/                 # Playwright end-to-end tests (includes full-season simulation)
```

## CI

`make ci` runs the full gate — six checks: format check (`fmt-check`), lint, dead-code (`dead`), invariants, tests, and vulnerability scan (`vuln`). Any CI provider (GitHub Actions, GitLab CI, Northflank, etc.) just calls `make ci`.

`make e2e` runs the Playwright end-to-end suite, which includes a full-season simulation: an admin creates a competition, players, and pairs through the UI; a complete double round-robin league is played (12 matches with scheduling, disputes, and penalties); standings are asserted against an independent computation covering all tiebreakers (points, set diff, game diff, head-to-head); and a playoff bracket is seeded, played, and resolved to a champion.

## UI language

All user-facing text is in Spanish. Code (Go, HTML, CSS classes) is in English.

## Security

The app is server-rendered and exposes no data API to clients — every read and write goes through a hand-written handler that enforces authorization (participant checks, opponent-only score confirmation, admin-only mutations). PocketBase still auto-serves a record REST API at `/api/collections/*`, so its write access rules are deliberately locked to superusers only (`create`/`update`/`delete` = `nil` on `matches`, `match_messages`, `users`, and `notifications.create`); server-side saves bypass those rules, so in-app flows are unaffected. **Do not relax these rules** — an empty-string rule (`""`) means *public* in PocketBase, which would let any client rewrite match results or self-escalate their role directly through the API. `handlers/api_security_test.go` guards this.
