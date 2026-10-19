BINARY  := lgtv
PREFIX  ?= /usr/local
GOFLAGS ?=

.PHONY: all build test vet fmt lint install uninstall clean

all: build

build:
	go build $(GOFLAGS) -o $(BINARY) ./cmd/lgtv

test:
	go test $(GOFLAGS) ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Fails if anything is unformatted, for CI.
lint:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; exit 1; \
	fi

install: build
	install -Dm755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)
