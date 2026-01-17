LDFLAGS := -s -w

.PHONY: all cli server clean

all: cli server

cli:
	go build -ldflags="$(LDFLAGS)" -o cctop ./cli

server:
	go build -ldflags="$(LDFLAGS)" -o cctop-server ./server

clean:
	rm -f cctop cctop-server
