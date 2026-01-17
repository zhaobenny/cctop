# cctop

Tracks Claude Code token consumption over time, similar to `ccusage` but in a single binary and with sync.

## Quickstart
```bash

cctop 
cctop --help
```

## Server, sync
The server is in a different binary, `cctop-server`.


Client:
```bash
cctop --sync http://localhost:8080
```