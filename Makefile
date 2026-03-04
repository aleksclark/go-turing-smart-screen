.PHONY: build test lint clean

build:
	go build -mod=vendor -o screens ./cmd/screens

test:
	go test -mod=vendor -v ./...

lint:
	golangci-lint run --timeout=5m

clean:
	rm -f screens
