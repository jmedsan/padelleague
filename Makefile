.PHONY: build run migrate css

css:
	cd frontend && npx tailwindcss -i ../static/css/input.css -o ../static/css/styles.css --minify

build: css
	go build -o padelleague .

run:
	go run . serve

migrate:
	go run . migrate up
