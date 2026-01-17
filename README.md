# cctop

Tracks Claude Code token consumption over time, similar to `ccusage` but in a single binary and with sync.

## Quickstart
```bash
sudo curl -fsSL https://github.com/zhaobenny/cctop/releases/latest/download/cctop-linux-amd64 -o /usr/local/bin/cctop && sudo chmod +x /usr/local/bin/cctop
cctop
cctop --help
```

## Server & Sync

The server is a simple web frontend for displaying synced usage data from multiple machines.

Use the provided [Docker Compose](https://raw.githubusercontent.com/zhaobenny/cctop/main/docker-compose.yml) or [`cctop-server` binary](https://github.com/zhaobenny/cctop/releases/latest) to run the server.
Client configuration is provided in the frontend after registering an new account.