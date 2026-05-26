# Standard targets for the kvstore server. On Windows without make,
# run the go commands directly, they are one liners.

.PHONY: run test race bench web

run:
	go run ./cmd/kvstore

web:
	go run ./cmd/kvstore-web

test:
	go test ./...

race:
	go test -race ./...

bench:
	go test -bench=. -benchmem ./store
