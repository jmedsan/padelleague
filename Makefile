-include .env
export

LOCAL_URL ?= http://127.0.0.1:8090
OPENER ?= xdg-open

.PHONY: build run migrate css open open-local open-remote stop reset test lint fmt vuln fmt-check ci check e2e e2e-bg e2e-failed e2e-smoke scenario-test scenario-serve scenario-stop mutate

css:
	cd frontend && npx tailwindcss -i ../static/css/input.css -o ../static/css/styles.css --minify

VERSION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

build: css version-file
	go build -ldflags="-X main.Version=$(VERSION)" -o padelleague .

version-file:
	@echo "$(VERSION)" > VERSION

run: stop build
	./padelleague serve

migrate:
	go run . migrate up

open-local:
	$(OPENER) $(LOCAL_URL)

open-remote:
	$(OPENER) $(APP_URL)

open: open-local

test:
	go test -parallel 4 ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	go mod tidy

vuln:
	govulncheck ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:" && gofmt -l . && exit 1)

dead:
	@out=$$(deadcode -test ./... 2>&1); \
	if [ -n "$$out" ]; then \
		echo "$$out"; \
		echo "FAIL: dead code found, or deadcode could not analyse the tree"; \
		exit 1; \
	fi; \
	echo "no dead code"

invariants:
	@fail=0; \
	n=$$(grep -rn 'err\.Error()' handlers/ --include='*.go' | grep -v _test.go | wc -l); \
	if [ "$$n" != "0" ]; then \
		echo "FAIL: $$n use(s) of err.Error() in handlers/ — raw errors must not reach the UI (R-19)"; \
		grep -rn 'err\.Error()' handlers/ --include='*.go' | grep -v _test.go; fail=1; \
	fi; \
	n=$$(grep -rln '^\t"log"$$' --include='*.go' . | grep -v '_test.go' | grep -v '^\./main\.go$$' | wc -l); \
	if [ "$$n" != "0" ]; then \
		echo "FAIL: standard log imported outside main.go — use log/slog (R-19)"; \
		grep -rln '^\t"log"$$' --include='*.go' . | grep -v '_test.go' | grep -v '^\./main\.go$$'; fail=1; \
	fi; \
	if ! grep -q 'slog.Info("startup"' main.go; then \
		echo "FAIL: startup config log line missing from main.go (R-19)"; fail=1; \
	fi; \
	( cd frontend && npx tailwindcss -i ../static/css/input.css -o /tmp/styles-css-check.css --minify ) >/dev/null 2>&1; \
	if ! diff -q /tmp/styles-css-check.css static/css/styles.css >/dev/null 2>&1; then \
		echo "FAIL: static/css/styles.css is stale — run 'make css' and commit it (the Dockerfile embeds the committed CSS without rebuilding)"; fail=1; \
	fi; \
	rm -f /tmp/styles-css-check.css; \
	n=$$(grep -rn 'FactsOnly' --include='*.go' . | wc -l); \
	if [ "$$n" != "0" ]; then \
		echo "FAIL: $$n use(s) of the retired FactsOnly ad-hoc flag — express via Mode"; \
		grep -rn 'FactsOnly' --include='*.go' .; fail=1; \
	fi; \
	n=$$(grep -rn '"Compact"\|"Large"\|"Linked"' --include='*.html' views/ | grep -v '{{/\*' | wc -l); \
	if [ "$$n" != "0" ]; then \
		echo "FAIL: $$n use(s) of a retired ad-hoc dict flag (Compact/Large/Linked) in templates — express via Mode"; \
		grep -rn '"Compact"\|"Large"\|"Linked"' --include='*.html' views/ | grep -v '{{/\*'; fail=1; \
	fi; \
	scripts/check-e2e-selectors.sh || fail=1; \
	scripts/check-e2e-tags.sh || fail=1; \
	if [ "$$fail" != "0" ]; then exit 1; fi; \
	echo "invariants hold"

ci: fmt-check lint dead css invariants test vuln
	@echo "CI gate passed"

# Fast iteration gate: fmt + lint + test scoped to the Go packages changed vs
# HEAD (plus everything that imports one of them), plus a smoke e2e pass.
# `make ci` stays the full release gate; this is for iterating on a change.
check:
	@gofmt -l . | (! grep .) || (echo "gofmt needed, run: make fmt" && exit 1)
	@pkgs=$$(scripts/changed-packages.sh); \
	if [ -z "$$pkgs" ]; then \
		echo "no changed Go packages vs HEAD"; \
	else \
		echo "checking: $$pkgs"; \
		golangci-lint run $$pkgs && \
		go test -parallel 4 $$pkgs; \
	fi
	@areas=$$(scripts/changed-areas.sh); \
	if [ -n "$$areas" ]; then \
		echo "e2e areas: $$areas"; \
		E2E_PORT=$$(node e2e/find-free-port.mjs) && \
		cd e2e && E2E_PORT=$$E2E_PORT npx playwright test --grep "$$areas" --workers 4 --project desktop --project mobile; \
	else \
		echo "no mapped e2e area changed, falling back to @smoke"; \
		$(MAKE) e2e-smoke; \
	fi

simulate: ## leveled-league simulation (SEASONS=100)
	go test ./league -run TestSimulation_LeveledLeague -count=1 -v -timeout 0 \
	    -parallel 5 -simulation.seasons=$(or $(SEASONS),100)

# Push notification error paths. Needs system Chrome with the Push API and a
# display, so it cannot run headless in CI alongside `make e2e`. Kept as a
# named target so it is discoverable and runnable in one command rather than
# a script someone has to find.
e2e-push:
	cd e2e/manual && DISPLAY=$${DISPLAY:-:0} node push-error-handling.mjs

e2e:
	@E2E_PORT=$$(node e2e/find-free-port.mjs) && \
	echo "Using port $$E2E_PORT" && \
	cd e2e && E2E_PORT=$$E2E_PORT npx playwright test $(if $(AREA),--grep "@$(AREA)")

# Background milestone runner: snapshots HEAD (git archive) into a temp dir so
# it never races an in-progress edit in the working tree, builds one shared
# binary, runs the full suite at 2 workers, and writes a summary + full log
# to e2e/.bg-runs/<timestamp>/. Fire-and-forget — check the summary file
# whenever convenient, no need to wait on the shell. Not part of `make ci`;
# the owner decides when a milestone run is due.
e2e-bg:
	@ts=$$(date +%Y%m%d-%H%M%S); \
	outdir=e2e/.bg-runs/$$ts; mkdir -p $$outdir; \
	snap=$$(mktemp -d /tmp/e2e-bg-XXXX); \
	git archive HEAD | tar -x -C $$snap; \
	ln -s $(CURDIR)/e2e/node_modules $$snap/e2e/node_modules; \
	echo "snapshot: $$snap -> $$outdir"; \
	( \
	  cd $$snap && \
	  go build -o padelleague . && \
	  E2E_PORT=$$(node e2e/find-free-port.mjs) && \
	  cd e2e && \
	  E2E_PORT=$$E2E_PORT E2E_BINARY=$$snap/padelleague npx playwright test --workers 2 \
	    > $(CURDIR)/$$outdir/full.log 2>&1; \
	  code=$$?; \
	  tail -20 $(CURDIR)/$$outdir/full.log > $(CURDIR)/$$outdir/summary.txt; \
	  cp test-results/.last-run.json $(CURDIR)/$$outdir/last-run.json 2>/dev/null; \
	  echo "exit_code=$$code" >> $(CURDIR)/$$outdir/summary.txt; \
	  rm -rf $$snap; \
	  echo "done: $(CURDIR)/$$outdir/summary.txt (exit $$code)" \
	) </dev/null >$$outdir/runner.log 2>&1 & \
	echo "started in background, pid group left running — see $$outdir/summary.txt when done"

# Re-run only the tests that failed in the most recent run (local `make e2e`
# or the newest `make e2e-bg` milestone, whichever is newer; LAST=<file> picks one). Fix every failure
# first, then verify them all in this one run. Two phases, because the
# `destructive` project depends on desktop + mobile (it changes shared admin
# settings, so it must run after them): phase 1 re-runs desktop + mobile
# failures, phase 2 re-runs destructive failures with --no-deps. A single
# `--last-failed` over all projects reports "No tests found": Playwright
# filters only the top-level project (`destructive`), empties it, and drops
# its dependencies with it. A phase with no recorded failure is skipped; the
# target fails if either phase fails.
e2e-failed:
	@last=$${LAST:-$$(ls -t e2e/test-results/.last-run.json e2e/.bg-runs/*/last-run.json 2>/dev/null | head -1)}; \
	[ -n "$$last" ] || { echo "no previous e2e run recorded"; exit 1; }; \
	last=$$(realpath $$last); \
	n=$$(jq '.failedTests | length' $$last); \
	[ "$$n" -gt 0 ] || { echo "no failures in $$last"; exit 0; }; \
	echo "re-running $$n failures from $$last"; \
	err=$$(mktemp); \
	list() { (cd e2e && npx playwright test --list --reporter=json "$$@" 2>>$$err) | jq -r '[.. | objects | select(has("id") and has("title")) | .id] | unique[]'; }; \
	main_ids=$$(list --project desktop --project mobile); \
	destr_ids=$$(list --project destructive --no-deps); \
	[ -n "$$main_ids" ] && [ -n "$$destr_ids" ] || { echo "could not list current tests:"; cat $$err; rm -f $$err; exit 1; }; \
	rm -f $$err; \
	failed=$$(jq -r '.failedTests[]' $$last); \
	stale=$$(echo "$$failed" | grep -vxF -e "$$main_ids" -e "$$destr_ids" || true); \
	[ -z "$$stale" ] || { echo "$$(echo "$$stale" | wc -l) of $$n recorded failures match no current test: Playwright IDs hash the title, so a spec renamed or edited since that run drops out. Re-run those by area (make e2e AREA=...):"; echo "$$stale" | sed 's/^/  /'; }; \
	[ "$$(echo "$$stale" | grep -c .)" -lt "$$n" ] || exit 1; \
	lf="--last-failed --last-failed-file $$last"; rc=0; \
	if echo "$$failed" | grep -qxF "$$main_ids"; then \
	  echo "phase 1: desktop + mobile"; \
	  (cd e2e && E2E_PORT=$$(node find-free-port.mjs) npx playwright test $$lf --project desktop --project mobile) || rc=1; \
	fi; \
	if echo "$$failed" | grep -qxF "$$destr_ids"; then \
	  echo "phase 2: destructive"; \
	  (cd e2e && E2E_PORT=$$(node find-free-port.mjs) npx playwright test $$lf --project destructive --no-deps) || rc=1; \
	fi; \
	exit $$rc

# ~15 tests tagged @smoke, one representative per area (auth, match, thread,
# competition, admin, search, responsive, season sim, PWA, notifications,
# presentation guards, walkover, profile, recovery window, leveled league),
# desktop + mobile, run in parallel — a fast sanity pass for `make check`.
# `make e2e` (untagged, workers=1) stays the full release gate.
e2e-smoke:
	@E2E_PORT=$$(node e2e/find-free-port.mjs) && \
	echo "Using port $$E2E_PORT" && \
	cd e2e && E2E_PORT=$$E2E_PORT npx playwright test --grep @smoke --workers 4 --project desktop --project mobile

stop:
	@pid=$$(lsof -ti :8090 2>/dev/null) && kill $$pid 2>/dev/null && echo "stopped (pid $$pid)" || echo "not running"

reset: stop
	rm -rf pb_data
	$(MAKE) run

# A mutant can allocate without bound (one reached 3.9 GB and took the machine
# down on 2026-09-27), so the whole run lives in a cgroup: past MUTATE_MEM the
# kernel kills it instead of the desktop.
MUTATE_MEM ?= 2500M
mutate: ## mutation test one package under a memory cap: make mutate PKG=./league [ARGS="-E 'awards\.go'"]
	@if [ -z "$(PKG)" ]; then echo "usage: make mutate PKG=./league [ARGS=...]"; exit 1; fi
	systemd-run --user --scope -q -p MemoryMax=$(MUTATE_MEM) -p MemorySwapMax=0 \
	  gremlins unleash --workers 1 --timeout-coefficient 100 --coverpkg=$(PKG) $(ARGS) $(PKG)

scenario-test: ## run a scenario: make scenario-test SCENARIO=<name>
	@if [ -z "$(SCENARIO)" ]; then cd e2e && npx tsx list-scenarios.ts; exit 1; fi
	@E2E_PORT=$$(node e2e/find-free-port.mjs) && \
	echo "scenario '$(SCENARIO)' on port $$E2E_PORT" && \
	cd e2e && SCENARIO=$(SCENARIO) E2E_PORT=$$E2E_PORT npx playwright test --config playwright.scenario.config.ts

scenario-serve: ## boot a scenario and keep server alive: make scenario-serve SCENARIO=<name>
	@if [ -z "$(SCENARIO)" ]; then cd e2e && npx tsx list-scenarios.ts; exit 1; fi
	@E2E_PORT=$$(node e2e/find-free-port.mjs) && \
	FIRST_PROJECT=$$(cd e2e && npx tsx first-scenario-project.ts $(SCENARIO)) && \
	cd e2e && E2E_KEEP=1 SCENARIO=$(SCENARIO) E2E_PORT=$$E2E_PORT npx playwright test \
	  --config playwright.scenario.config.ts --project="$$FIRST_PROJECT" --grep "00 "

scenario-stop: ## stop a kept scenario server and delete its data: make scenario-stop PORT=<port>
	@port="$(PORT)"; \
	if [ -z "$$port" ]; then \
		runs=$$(ls -d e2e/.test-data/*/scenario.pid 2>/dev/null | sed 's|e2e/.test-data/\([0-9]*\)/scenario.pid|\1|'); \
		n=$$(echo "$$runs" | grep -c . || true); \
		if [ "$$n" -eq 1 ]; then port=$$runs; \
		elif [ "$$n" -eq 0 ]; then echo "no scenario server running"; exit 0; \
		else echo "several scenario servers running, pick one: make scenario-stop PORT=<port>"; echo "$$runs"; exit 1; fi; \
	fi; \
	d=e2e/.test-data/$$port; \
	if [ -f $$d/scenario.pid ]; then \
		pid=$$(cat $$d/scenario.pid); \
		kill $$pid 2>/dev/null || true; \
		timeout=50; while kill -0 $$pid 2>/dev/null && [ $$timeout -gt 0 ]; do \
			sleep 0.2; timeout=$$((timeout - 1)); done; \
		rm -f $$d/scenario.pid; \
		if [ -f $$d/scenario.dir ]; then \
			dd=$$(cat $$d/scenario.dir); \
			case "$$dd" in /tmp/padelleague-test-*) \
				[ -d "$$dd" ] && rm -rf -- "$$dd" && echo "deleted $$dd";; \
			*) echo "refusing to delete: $$dd";; esac; \
			rm -f $$d/scenario.dir; \
		fi; \
		echo "scenario server on port $$port stopped"; \
	else echo "no scenario server running on port $$port"; fi
