# cctop

Tracks Claude Code token consumption over time, similar to `ccusage` but in a single self-contained binary and with sync.

## Quickstart
```bash
sudo curl -fsSL https://github.com/zhaobenny/cctop/releases/latest/download/cctop-linux-amd64 -o /usr/local/bin/cctop && sudo chmod +x /usr/local/bin/cctop
cctop
cctop --help
```

## Server & Sync

The server stores synced usage in SqLite and hosts a simple web frontend for displaying usage data from multiple Claude Code instances.

Use the provided [Docker Compose](https://raw.githubusercontent.com/zhaobenny/cctop/main/docker-compose.yml) or [`cctop-server` binary](https://github.com/zhaobenny/cctop/releases/latest) to run the server.
Client configuration is provided in the frontend after registering an new account.

## Development
```bash
make clean    # Remove built binaries
make # Builds both CLI and server
```

---

Made with mostly Claude Code.