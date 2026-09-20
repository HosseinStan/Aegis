.PHONY: build test vet fmt check run

build:
	cd apps/aegis-api && go build ./...

test:
	cd apps/aegis-api && go test ./...

vet:
	cd apps/aegis-api && go vet ./...

fmt:
	gofmt -w apps/aegis-api

check: test vet build

run:
	cd apps/aegis-api && go run ./cmd/server
