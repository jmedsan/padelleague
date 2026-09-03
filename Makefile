-include .env
export

LOCAL_URL ?= http://127.0.0.1:8090
OPENER ?= xdg-open

.PHONY: build run migrate css open open-local open-remote stop reset test lint fmt vuln fmt-check ci e2e

css:
	cd frontend && npx tailwindcss -i ../static/css/input.css -o ../static/css/styles.css --minify

VERSION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

build: css
	go build -ldflags="-X main.Version=$(VERSION)" -o padelleague .

run:
	go run . serve

migrate:
	go run . migrate up

open-local:
	$(OPENER) $(LOCAL_URL)

open-remote:
	$(OPENER) $(REMOTE_URL)

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
	@out=$$(deadcode ./... 2>&1); \
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
		echo "FAIL: $$n use(s) of the retired FactsOnly ad-hoc flag — express via Mode (see .claude/steering/component-modes.md)"; \
		grep -rn 'FactsOnly' --include='*.go' .; fail=1; \
	fi; \
	n=$$(grep -rn '"Compact"\|"Large"\|"Linked"' --include='*.html' views/ | grep -v '{{/\*' | wc -l); \
	if [ "$$n" != "0" ]; then \
		echo "FAIL: $$n use(s) of a retired ad-hoc dict flag (Compact/Large/Linked) in templates — express via Mode (see .claude/steering/component-modes.md)"; \
		grep -rn '"Compact"\|"Large"\|"Linked"' --include='*.html' views/ | grep -v '{{/\*'; fail=1; \
	fi; \
	if [ "$$fail" != "0" ]; then exit 1; fi; \
	echo "invariants hold"

ci: fmt-check lint dead invariants test vuln
	@echo "CI gate passed"

# Push notification error paths. Needs system Chrome with the Push API and a
# display, so it cannot run headless in CI alongside `make e2e`. Kept as a
# named target so it is discoverable and runnable in one command rather than
# a script someone has to find.
e2e-push:
	cd e2e/manual && DISPLAY=$${DISPLAY:-:0} node push-error-handling.mjs

e2e:
	cd e2e && npx playwright test

stop:
	@pid=$$(lsof -ti :8090 2>/dev/null) && kill $$pid 2>/dev/null && echo "stopped (pid $$pid)" || echo "not running"

reset: stop
	rm -rf pb_data
	$(MAKE) run
