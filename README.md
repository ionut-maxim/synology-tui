# synology-tui

A terminal UI for Synology DSM 7, written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

- **Overview:** model, DSM version, uptime, temperature, CPU/RAM/network load, volumes, pools, disks, packages, and a list of warnings (bad sectors, full volumes, degraded pools).
- **Storage:** every disk with temperature, bad sectors, status and S.M.A.R.T. self-test state; pools and volumes. Run a quick or extended S.M.A.R.T. test, or stop one.
- **Packages:** installed packages and their state. Start, stop or restart one.
- **Logs:** DSM's own log, newest first, 100 at a time.

Every action asks for confirmation first. The data refreshes every 30 seconds.

## Install

With [mise](https://mise.jdx.dev):

```sh
mise use -g github:ionut-maxim/synology-tui
```

or with Go: `go install github.com/ionut-maxim/synology-tui@latest`.

## Use

```sh
export SYNOLOGY_HOST=https://nas.local:5001   # or -host
export SYNOLOGY_USER=admin                     # or -user
synology-tui -insecure                         # asks for the password
synology-tui -insecure -status                 # print a summary and exit
```

- The password comes from `SYNOLOGY_PASSWORD` or a prompt, never a flag (it would land in your shell history).
- `-insecure` (or `SYNOLOGY_INSECURE=true`) skips TLS verification. DSM ships a self-signed certificate; drop the flag once yours is trusted.
- The account needs to be in DSM's **administrators** group: the storage and package APIs refuse other users. Accounts with 2-step verification aren't supported yet.

Keys: `tab`/`1`–`4` switch screens, `r` refreshes, `q` quits. Storage: `t` quick test, `T` extended test, `x` stop the test. Packages: `s` start, `S` stop, `R` restart. Logs: `n` older, `p` newer.

## The `dsm` package

`github.com/ionut-maxim/synology-tui/dsm` is a small client for the DSM Web API, usable on its own:

```go
c := dsm.New("https://nas.local:5001", dsm.Options{Insecure: true})
if err := c.Login(ctx, user, password); err != nil { … }
defer c.Logout(ctx)
st, err := c.Storage(ctx) // disks, pools, volumes
```

It renews an expired session by itself. DSM documents few of these APIs; [docs/dsm-api.md](docs/dsm-api.md) lists the ones used here and how they were found.

## Develop

Tools come from `mise.toml` (`mise install`). `go test -race ./...` runs without a NAS (a fake DSM server). Tag `vX.Y.Z` to release (GoReleaser).
