.PHONY: build test lint clean install

build:
	go build -mod=vendor -ldflags="-s -w" -o turing-screens ./cmd/screens

test:
	go test -mod=vendor -v ./...

lint:
	golangci-lint run --timeout=5m

install: build
	sudo systemctl stop turing-screens || true
	sudo cp turing-screens /usr/bin/turing-screens
	sudo cp install/turing-screens.service /usr/lib/systemd/system/turing-screens.service
	sudo systemctl daemon-reload
	sudo systemctl start turing-screens

clean:
	rm -f turing-screens
