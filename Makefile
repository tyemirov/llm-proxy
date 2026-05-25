.PHONY: fmt lint test ci

fmt:
	gofmt -w cmd internal tests

lint:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...
	go run github.com/gordonklaus/ineffassign@latest ./...

test:
	go test ./...

ci: lint test
