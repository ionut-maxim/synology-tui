# synology-tui — agent rules

Terminal UI for Synology DSM 7 (Go, Bubble Tea v2) plus the reusable DSM client `dsm/`. Public repo: no internal hostnames, IPs, account names or serial numbers in code, tests, fixtures or docs.

## Layout

```
main.go          flags/env, login, -status or the TUI
dsm/             DSM Web API client (also imported by github.com/ionut-maxim/homelab's alert-hub)
internal/ui/     Bubble Tea model (ui.go), rendering (view.go), snapshot + -status (status.go)
docs/dsm-api.md  the DSM APIs used, with parameters and how they were found
```

## Rules

- **Tools from mise** (`mise.toml`); no other installs.
- **`dsm` is a library with outside users** (alert-hub). Keep its API small and stable; validate every argument that reaches DSM (device paths, package IDs, test types) before sending it.
- **New DSM calls:** find the method and parameters in DSM's own web UI code (see docs/dsm-api.md → How to find a method), add a fake-server test in `dsm/client_test.go`, and document it in docs/dsm-api.md.
- **Never call a write method to explore.** Verify against a real NAS with read methods (`-status`), and use write methods only when the user asks for that action.
- **Every action in the TUI asks y/N first** (`Model.ask`).
- Charm v2 modules are `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2` (`View()` returns `tea.View`; keys are `tea.KeyPressMsg`).
- Before pushing: `gofmt -l .`, `go vet ./...`, `go test -race ./...`. Releases: tag `vX.Y.Z` (GoReleaser).
