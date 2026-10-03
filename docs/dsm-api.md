# DSM Web API notes

What `dsm/` calls and how it was found. Verified on DSM 7.4.1 (DS420j), October 2026.

## Basics

- Everything goes to `POST /webapi/entry.cgi`, form-encoded: `api`, `version`, `method`, the method's parameters, and `_sid` (the session ID; in the body, not the URL, so it stays out of logs). Lists and objects are JSON-encoded strings (`additional=["status"]`).
- Responses: `{"success": true, "data": …}` or `{"success": false, "error": {"code": N}}`. Codes 106, 107 and 119 mean the session is gone: log in again (the client does).
- Login: `SYNO.API.Auth` v7 `login` with `account`, `passwd`, `session`, `format=sid` → `data.sid`. `logout` ends it.
- `GET /webapi/query.cgi?api=SYNO.API.Info&version=1&method=query&query=all` lists every API (about 715) with its path and version range, without logging in. It doesn't list methods.

## How to find a method

DSM documents few APIs, and the per-API definitions (`/webapi/*.lib`) return 403. The web UI is plain JavaScript that calls these APIs, and its files are served without a login:

1. Each app has a `config` file naming its scripts: `/webman/modules/<App>/config` (e.g. `DSMNotify`, `PkgManApp`, `ResourceMonitor`, `AdminCenter`, `SystemInfoApp`) or `/webman/3rdparty/<App>/config` (`StorageManager`, `LogCenter`).
2. Download the scripts it lists, plus the desktop bundles from the login page (`/synoSDSjslib/dist/sds.bundle.js`, `/webman/*/dist/*.bundle.js`).
3. Search for `api:"SYNO.…"` … `method:"…"` pairs, then read the surrounding code for `params:{…}` and the values the UI puts in them.

Then verify with a read method on a real NAS before relying on it. Don't call write methods to explore.

## APIs used

| What | API (version) | Method | Parameters |
|---|---|---|---|
| System | `SYNO.Core.System` (3) | `info` | — |
| Load | `SYNO.Core.System.Utilization` (1) | `get` | — |
| Disks, pools, volumes | `SYNO.Storage.CGI.Storage` (1) | `load_info` | — |
| Start/stop a S.M.A.R.T. test | `SYNO.Core.Storage.Disk` (1) | `do_smart_test` | `device` (`/dev/sdb`), `type` (`quick`, `extend`, `stop`) |
| Self-test state | `SYNO.Core.Storage.Disk` (1) | `get_smart_test_log` | `device` |
| Packages | `SYNO.Core.Package` (2) | `list` | `additional=["status"]` |
| Start a package | `SYNO.Core.Package.Control` (1) | `start` | `id`, `dsm_apps=[]` |
| Stop a package | `SYNO.Core.Package.Control` (1) | `stop` | `id` |
| DSM's log | `SYNO.Core.SyslogClient.Log` (1) | `list` | `target=LOCAL`, `offset`, `limit` |

Notes:
- `load_info` sizes are strings of bytes; `unc` is the disk's uncorrectable (bad) sector count; a pool or volume in `attention` explains itself in `space_status.detail` (e.g. `fs_almost_full`).
- `get_smart_test_log`: `testInfo[0].testing`, `remain` (progress, e.g. `10%`), `quickTime`/`extendTime` (DSM's estimates in minutes, space-padded), `latest_test_result`, `latest_test_type` (1 quick, 2 extended), and `latest_test_time`.
- The package UI runs `SYNO.Core.Package` `feasibility_check` before start/stop (dependency checks); the client doesn't.

## Looked at, not used

- **Clearing DSM's log:** `SYNO.Core.SyslogClient.Log` has no clear method. `SYNO.LogCenter.Log` `clear {logtype, path}` clears logs *received from other devices* only.
- **Notifications:** `SYNO.Core.DSMNotify` `notify` with `action`: `load` (list), `apply` + `clean: "all"` (clear), `delete_by_ids` + `notifyIds`. They're per user: clearing them from one account doesn't clear another's.
- **S.M.A.R.T. schedule:** `SYNO.Storage.CGI.Smart.Scheduler` (`list`, `get`, `set`, `run`, `delete`).
- **Disk health history:** `SYNO.Storage.CGI.Smart` `get_health_info {device}`.
