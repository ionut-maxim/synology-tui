package dsm

import (
	"context"
	"net/url"
	"strconv"
)

// SystemInfo is SYNO.Core.System info: model, DSM version, uptime, temperature.
type SystemInfo struct {
	Model       string `json:"model"`
	DSM         string `json:"firmware_ver"` // "DSM 7.4.1-90080"
	RAMMB       int    `json:"ram_size"`
	Uptime      string `json:"up_time"` // "700:37:58" (hours:minutes:seconds)
	TempC       int    `json:"sys_temp"`
	TempWarning bool   `json:"temperature_warning"`
}

func (c *Client) SystemInfo(ctx context.Context) (SystemInfo, error) {
	var s SystemInfo
	err := c.Call(ctx, "SYNO.Core.System", 3, "info", nil, &s)
	return s, err
}

// Utilization is the live load (SYNO.Core.System.Utilization get).
type Utilization struct {
	CPU struct {
		User   int `json:"user_load"`
		System int `json:"system_load"`
		Other  int `json:"other_load"`
	} `json:"cpu"`
	Memory struct {
		UsedPercent int `json:"real_usage"`
		TotalKB     int `json:"total_real"`
		SwapPercent int `json:"swap_usage"`
	} `json:"memory"`
	Network []struct {
		Device string `json:"device"` // "total", "eth0", …
		RxBps  int64  `json:"rx"`
		TxBps  int64  `json:"tx"`
	} `json:"network"`
}

// CPUPercent is the total CPU load.
func (u Utilization) CPUPercent() int { return u.CPU.User + u.CPU.System + u.CPU.Other }

func (c *Client) Utilization(ctx context.Context) (Utilization, error) {
	var u Utilization
	err := c.Call(ctx, "SYNO.Core.System.Utilization", 1, "get", nil, &u)
	return u, err
}

// LogEntry is one line of DSM's own log (Log Center → Logs, local).
type LogEntry struct {
	Time  string `json:"time"` // "2026/10/02 22:23:17", NAS local time
	Level string `json:"level"`
	Type  string `json:"logtype"` // System, Connection, File Transfer …
	Who   string `json:"who"`
	Text  string `json:"descr"`
}

// Logs returns DSM's own log, newest first, and the total count
// (SYNO.Core.SyslogClient.Log list). DSM's API offers no way to clear it.
func (c *Client) Logs(ctx context.Context, offset, limit int) ([]LogEntry, int, error) {
	var out struct {
		Items []LogEntry `json:"items"`
		Total int        `json:"total"`
	}
	err := c.Call(ctx, "SYNO.Core.SyslogClient.Log", 1, "list", url.Values{
		"target": {"LOCAL"}, "offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(limit)},
	}, &out)
	return out.Items, out.Total, err
}
