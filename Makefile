VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all cli server clean

all: cli server

cli:
	go build -ldflags="$(LDFLAGS)" -o cctop ./cli

server:
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o cctop-server ./server

clean:
	rm -f cctop cctop-server
