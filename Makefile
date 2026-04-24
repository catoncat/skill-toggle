.PHONY: build test lint vet clean run

BINARY := skill-toggle
GO := go
GOFMT := gofmt

build:
	$(GO) build -o $(BINARY) ./cmd/skill-toggle

run: build
	./$(BINARY)

test:
	$(GO) test ./...

test-verbose:
	$(GO) test -v ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

fmt:
	$(GOFMT) -w .

clean:
	rm -f $(BINARY)

install: build
	install -m 755 $(BINARY) ~/.local/bin/$(BINARY)
