# PadelLeague

Padel league management — competitions, matches, rankings and scheduling.

## What it does

A web app for organizing padel leagues. An admin creates competitions, assigns pairs, and generates fixtures (round-robin or leveled). Players log in, negotiate match schedules through an in-app thread, submit scores, and confirm results. The system computes standings with proper padel rules (3 points per win, set/game diff, head-to-head tiebreakers).

### For players

- **Urgent-tasks home** — the single most important next action first: an open dispute, your next confirmed match to play, or a match to organize before its deadline
- **Scheduling deadlines** — each league round shows a recommended "organize by" date; matches surface escalating warnings (próximo → urgente → vencido) with at most one reminder per step
- **Match thread** — schedule proposals, chat, and score discussion in one place; every result-changing action (submit, confirm, dispute, correct, resolve, walkover) is recorded as a timeline entry attributed to the acting pair and player
- **Structured score entry** — set-by-set input with valid padel scores, venue selection
- **Score confirmation with reminders** — one player submits a result, the opponent confirms or disputes; if it sits unconfirmed the app reminds the opponent before the quorum timeout resolves it
- **Arbitration requests** — a participant can ask the admin to review a match (result, scheduling, abandonment, no-show, other); the match shows an "Arbitraje solicitado" badge and the admins are notified
- **Proposed results count as played** — a proposed score already counts in standings and stats, marked with a warning icon ("Incluye resultados sin confirmar"), until it is final; a match in dispute, a still-open set, or two pairs holding different pending proposals counts nowhere until it is resolved (a counter-proposal replaces the original, so the newest proposal then counts)
- **Everyone in the match is notified** — chat, date and result proposals, responses, withdrawals and cancellations notify every player in the match except the one who acted, the actor's partner included
- **Contact links** — WhatsApp and email icons for your rivals on the match page and for players on their profile (when they have set a phone or email), and the league admin's contact in the navbar menu, the drawer and error pages
- **Add to calendar** — one-tap "Añadir al calendario" (`.ics`) on a scheduled match
- **Player profile** — win rate, per-competition stats with links to competitions and partners
- **Pair page** — canonical page per pair showing players, competition positions, and match history
- **Notifications** — bell icon with unread count, email, and web push; all three channels delivered automatically from one pipeline. Per-type toggles (result proposals, disputes, scheduling, chat messages, match assignments) in notification preferences
- **Email verification** — unverified users see a persistent banner with a resend link; email notifications are blocked until verified
- **Global search** — fuzzy, accent-folded search across players, pairs, competitions, matches, threads, documents, and venues; focusing the search box without typing shows a zero-query panel with pending obligations, active competitions, recent searches, and quick-nav links

### For admins

- **Competition management** — create leagues/playoffs, assign pairs, generate fixtures (round-robin for standard leagues, rolling opponent assignment for leveled leagues)
- **Gender rules** — each competition is free, male, female or mixed; the admin sets it (default in Configuración) and the app refuses pairs that do not fit (mixed needs one man and one woman; players need a gender set)
- **Pair withdrawal** — withdraw a pair from a competition: its pre-score matches are finalized as walkovers for the opponents with the default score, and the pair shows a "Retirada" badge in the standings
- **Competition announcements** — post an announcement on the competition page and notify its players (they can mute it in notification preferences); delete it later
- **Leveled leagues** — each pair plays opponents closest to them in skill; opponent assignments are rolling: the admin sets `target_matches` (total matches per pair) and `open_assignments` (matches each pair keeps open at once, default 3); standings use the same 3-points-per-win rules as any league
- **League scheduling** — set start/end dates; the app computes and **stores a recommended arrange-by date per round** (admin-editable on the competition page, with a "regenerate" option), sends escalating reminders, and flags overdue matches
- **End-of-league recovery window** — a per-competition grace period after the end date (default 7 days, editable) during which still-pending matches show an "En recuperación" state and stay organizable; the admin can finalize a league early
- **Admin-approved walkovers** — when a team reports a match unplayed, the admin approves a walkover with a configurable default score (6-0 6-0) and points penalty; no walkover is ever applied without admin approval. The one automatic penalty is the rulebook close: once the recovery window ends, a daily job closes the league and applies −1 point per match a pair is short of its target (and −1 per pending match above the limit during the window), logs it, and notifies the admin, who can void or correct it
- **Playoff brackets** — admin-set fixed dates enforced in bracket order (quarters → semis → final), with a mobile-friendly bracket view
- **Home dashboard** — a setup checklist before a league starts, and dispute/overdue/walkover alerts once it is active
- **Outstanding matches** — one view of every unresolved match across active competitions with its deadline and urgency, most-urgent first
- **Invite-only registration** — admin generates invite links, no open signup
- **Dispute resolution** — review and resolve score disagreements; close an arbitration request once it is handled
- **League contact details** — set the admin phone and email that logged-in users see as contact links (Configuración)
- **Admin notifications** — toggleable alerts for match progress, system errors, and new player registrations; admin-only section in notification preferences
- **Reference documents** — upload files or add links with mandatory/default flags; protected file downloads with per-request tokens
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

The app is available at `http://localhost:8090`. On startup the app creates the PocketBase superuser and the admin users from the `PB_ADMIN_*` and `APP_ADMIN*` variables (see below). The PocketBase admin UI at `http://localhost:8090/_/` is blocked unless `APP_DEV_TOOLS=true`.

To create a superuser by hand instead:

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
| `PB_ADMIN_EMAIL` | PocketBase superuser email (created at startup if missing) |
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
| `APP_ENV` | Environment: `prod` (default) or `dev`. `prod` skips the player seed; `/dev-login` exists only when `dev` |
| `APP_DEV_TOOLS` | `true` to unblock the PocketBase admin UI at `/_/` and show the admin Dev Tools page (database reset, test push; the page also needs `APP_ENV` other than `prod`). Default: `false` |
| `SMTP_HOST` | SMTP server host (e.g. `smtp.gmail.com`) |
| `SMTP_PORT` | SMTP port (default: `587`) |
| `SMTP_USERNAME` | SMTP username |
| `SMTP_PASSWORD` | SMTP password (app password for Gmail) |
| `SMTP_TLS` | Enable TLS (default: `false`) |
| `SMTP_SENDER_ADDRESS` | Sender email address |
| `SMTP_SENDER_NAME` | Sender display name (default: empty, which uses the league name from Configuración, or `PadelLeague` if unset) |
| `APP_URL` | Public app URL for email links (e.g. `https://league.example.com`) |
| `VAPID_PUBLIC_KEY` | Web push VAPID public key |
| `VAPID_PRIVATE_KEY` | Web push VAPID private key |
| `BACKUP_EMAIL` | Address that receives an emailed copy of each backup; backups are off when unset |
| `BACKUP_ENCRYPTION_KEY` | Passphrase for the emailed backup (AES-256, `openssl enc -d -aes-256-cbc -pbkdf2`); required, the app never mails an unencrypted backup |
| `BACKUP_INTERVAL_HOURS` | Hours between backups (default: `12`; 24 or more means daily) |

## Project structure

```
main.go              # Entry point, wires packages together
config/              # Env-based configuration struct
league/              # Domain logic (scoring, standings, fixtures, awards, quorum, leveled assignment, hidden rating)
notify/              # Notification delivery (in-app, push, email — all three from one deliver method)
handlers/            # HTTP handlers (thin: parse request, call domain, render)
hooks/               # PocketBase event hooks and cron jobs
middleware/          # Cookie auth bridge, admin role check
migrations/          # PocketBase schema migrations
render/              # Template rendering helpers
routes/              # Route registration with Deps struct
search/              # Fuzzy global search (in-memory index, accent-fold, Levenshtein ranking, role-scoped)
seed/                # Dev/test data seeding
views/               # Go HTML page templates (layout.html, pages, admin/)
views/partials/      # One shared partial per domain object; explicit modes select player, summary, or admin controls
static/              # CSS, JS, images, manifest.json, sw.js (PWA)
frontend/            # Tailwind config + input CSS (build only)
e2e/                 # Playwright end-to-end tests (includes full-season simulation)
```

## CI

`make ci` runs the full gate — six checks: format check (`fmt-check`), lint, dead-code (`dead`), invariants, tests, and vulnerability scan (`vuln`). Any CI provider (GitHub Actions, GitLab CI, Northflank, etc.) just calls `make ci`.

`make e2e` runs the Playwright end-to-end suite, which includes a full-season simulation: an admin creates a competition, players, and pairs through the UI; a complete double round-robin league is played (12 matches with scheduling, disputes, and penalties); standings are asserted against an independent computation covering all tiebreakers (points, set diff, game diff, head-to-head); and a playoff bracket is seeded, played, and resolved to a champion. Leveled leagues use rolling assignments instead of Berger round-robin — the admin sets `target_matches` and `open_assignments`; the system assigns opponents by skill proximity.

## Migrations

Migrations in `migrations/` run automatically each time the app starts. To apply them without leaving a server running (the binary has no `migrate` command):

```bash
make migrate                 # data dir pb_data
make migrate DIR=/some/dir   # any data dir
make scenario-migrate        # the kept scenario server (PORT=<port> if several)
```

Each target boots the binary on a free port against the data dir, waits until `/healthz` answers, then stops it. Running one again changes nothing.

## UI language

All user-facing text is in Spanish. Code (Go, HTML, CSS classes) is in English.

## Security

The app is server-rendered and exposes no data API to clients — every read and write goes through a hand-written handler that enforces authorization (participant checks, opponent-only score confirmation, admin-only mutations). PocketBase still auto-serves a record REST API at `/api/collections/*`, so its write access rules are deliberately locked to superusers only (`create`/`update`/`delete` = `nil` on `matches`, `match_messages`, `users`, and `notifications.create`); server-side saves bypass those rules, so in-app flows are unaffected. **Do not relax these rules** — an empty-string rule (`""`) means *public* in PocketBase, which would let any client rewrite match results or self-escalate their role directly through the API. `handlers/api_security_test.go` guards this.

Match data (scores, dates, pair relations) is intentionally readable by all authenticated users via the auto-API, since league results are public — the server-rendered pages display all matches to any participant.
