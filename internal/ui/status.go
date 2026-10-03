package ui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ionut-maxim/synology-tui/dsm"
)

// snapshot is everything the screens show except logs.
type snapshot struct {
	info     dsm.SystemInfo
	util     dsm.Utilization
	storage  dsm.Storage
	packages []dsm.Package
	smart    map[string]dsm.SmartStatus // device → test state
	at       time.Time
}

// load reads a snapshot. Per-part errors are joined; what loaded is kept.
func load(ctx context.Context, c *dsm.Client) (snapshot, error) {
	s := snapshot{smart: map[string]dsm.SmartStatus{}, at: time.Now()}
	var errs []string
	note := func(err error) {
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	var err error
	s.info, err = c.SystemInfo(ctx)
	note(err)
	s.util, err = c.Utilization(ctx)
	note(err)
	s.storage, err = c.Storage(ctx)
	note(err)
	s.packages, err = c.Packages(ctx)
	note(err)
	for _, d := range s.storage.Disks {
		st, err := c.SmartTestStatus(ctx, d.Device)
		note(err)
		s.smart[d.Device] = st
	}
	if len(errs) > 0 {
		return s, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return s, nil
}

// warnings are the things worth a look, worst first.
func (s snapshot) warnings() []string {
	var w []string
	for _, d := range s.storage.Disks {
		if d.Status != "normal" || d.SmartStatus != "normal" {
			w = append(w, fmt.Sprintf("%s: status %s, S.M.A.R.T. %s", d.Name, d.Status, d.SmartStatus))
		}
		if d.BadSectors > 0 {
			w = append(w, fmt.Sprintf("%s has %d bad sectors", d.Name, d.BadSectors))
		}
	}
	for _, p := range s.storage.Pools {
		if p.Status != "normal" {
			why := p.Status
			if p.SpaceStatus.Detail != "" {
				why += " (" + strings.ReplaceAll(p.SpaceStatus.Detail, "_", " ") + ")"
			}
			w = append(w, fmt.Sprintf("Storage pool %s: %s", p.Description, why))
		}
	}
	for _, v := range s.storage.Volumes {
		if v.Status != "normal" {
			why := v.Status
			if v.SpaceStatus.Detail != "" {
				why += " (" + strings.ReplaceAll(v.SpaceStatus.Detail, "_", " ") + ")"
			}
			w = append(w, fmt.Sprintf("%s: %s, %.0f%% used", v.Path, why, v.UsedPercent()))
		}
	}
	if s.info.TempWarning {
		w = append(w, fmt.Sprintf("System temperature warning: %d °C", s.info.TempC))
	}
	for _, p := range s.packages {
		if p.Status != "running" && p.Status != "stop" {
			w = append(w, fmt.Sprintf("Package %s: %s", p.ID, p.Status))
		}
	}
	return w
}

// PrintStatus prints a plain-text summary (-status).
func PrintStatus(ctx context.Context, w io.Writer, c *dsm.Client) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	s, err := load(ctx, c)
	i := s.info
	fmt.Fprintf(w, "%s · %s · up %s · %d °C · CPU %d%% · RAM %d%% of %d MB\n",
		i.Model, i.DSM, uptime(i.Uptime), i.TempC, s.util.CPUPercent(), s.util.Memory.UsedPercent, i.RAMMB)
	fmt.Fprintln(w)
	for _, v := range s.storage.Volumes {
		fmt.Fprintf(w, "%-10s %-6s %s of %s used (%.0f%%), %s\n", v.Path, v.FS, v.Size.Used, v.Size.Total, v.UsedPercent(), v.Status)
	}
	for _, p := range s.storage.Pools {
		fmt.Fprintf(w, "pool %-5s %s, %d disks, %s\n", p.Description, p.Type, len(p.Disks), p.Status)
	}
	fmt.Fprintln(w)
	for _, d := range s.storage.Disks {
		fmt.Fprintf(w, "%-8s %-9s %-20s %7s %3d °C  bad sectors %-3d %-7s S.M.A.R.T. %-7s %s\n",
			d.Name, d.Device, d.Model, d.SizeBytes, d.TempC, d.BadSectors, d.Status, d.SmartStatus, smartLine(s.smart[d.Device]))
	}
	running := 0
	for _, p := range s.packages {
		if p.Running() {
			running++
		}
	}
	fmt.Fprintf(w, "\n%d packages, %d running\n", len(s.packages), running)
	if ws := s.warnings(); len(ws) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, x := range ws {
			fmt.Fprintln(w, "  !", x)
		}
	}
	logs, total, lerr := c.Logs(ctx, 0, 5)
	if lerr == nil {
		fmt.Fprintf(w, "\nLatest log entries (%d in total):\n", total)
		for _, l := range logs {
			fmt.Fprintf(w, "  %s %-7s %-12s %s\n", l.Time, l.Level, l.Type, l.Text)
		}
	}
	if err == nil {
		err = lerr
	}
	return err
}

// uptime turns DSM's "700:37:58" into "29d 4h".
func uptime(s string) string {
	var h, m, sec int
	if _, err := fmt.Sscanf(s, "%d:%d:%d", &h, &m, &sec); err != nil {
		return s
	}
	if h >= 24 {
		return fmt.Sprintf("%dd %dh", h/24, h%24)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// smartLine is a disk's self-test state in a few words.
func smartLine(st dsm.SmartStatus) string {
	switch {
	case st.Testing:
		return "testing (" + st.Progress + ")"
	case st.LastResult != "":
		t := string(st.LastType)
		if t == "extend" {
			t = "extended"
		}
		return fmt.Sprintf("last %s test %s %s", t, st.LastResult, st.LastTime)
	default:
		return "no test yet"
	}
}
