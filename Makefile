-include .env
export

LOCAL_URL ?= http://127.0.0.1:8090
OPENER ?= xdg-open

.PHONY: build run migrate css open open-local open-remote stop reset test lint fmt vuln fmt-check ci e2e

css:
	cd frontend && npx tailwindcss -i ../static/css/input.css -o ../static/css/styles.css --minify

build: css
	go build -o padelleague .

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
	go test ./...

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
	@deadcode ./... | tee /dev/stderr | (! grep -q .)

ci: fmt-check lint dead test vuln
	@echo "CI gate passed"

e2e:
	cd e2e && npx playwright test

stop:
	@pid=$$(lsof -ti :8090 2>/dev/null) && kill $$pid 2>/dev/null && echo "stopped (pid $$pid)" || echo "not running"

reset: stop
	rm -rf pb_data
	$(MAKE) run
