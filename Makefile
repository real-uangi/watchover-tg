.PHONY: frontend test check build
frontend:
	pnpm --dir web build

test: frontend
	go test ./...

check: frontend
	pnpm --dir web test
	go test -race ./...
	go vet ./...
	go build -o bin/watchover-tg .

build: frontend
	go build -o bin/watchover-tg .
