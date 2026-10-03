package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ionut-maxim/synology-tui/dsm"
)

func sample(t *testing.T) Model {
	t.Helper()
	var st dsm.Storage
	if err := json.Unmarshal([]byte(`{
		"disks":[
		 {"id":"sda","device":"/dev/sda","name":"Drive 1","vendor":"Seagate","model":"ST4000VN008","firm":"SC60","diskType":"SATA","size_total":"4000787030016","status":"normal","smart_status":"normal","temp":39,"unc":0,"used_by":"reuse_1"},
		 {"id":"sdb","device":"/dev/sdb","name":"Drive 2","vendor":"Seagate","model":"ST4000VN008","firm":"SC60","diskType":"SATA","size_total":"4000787030016","status":"normal","smart_status":"normal","temp":40,"unc":16,"used_by":"reuse_1"}],
		"storagePools":[{"id":"reuse_1","desc":"SHR","device_type":"shr_with_1_disk_protect","status":"attention","space_status":{"detail":"fs_almost_full"},"disks":["sda","sdb"],"size":{"total":"8000","used":"8000"}}],
		"volumes":[{"id":"volume_1","vol_path":"/volume1","fs_type":"ext4","status":"attention","space_status":{"detail":"fs_almost_full"},"size":{"total":"11892862238720","used":"11446951395328"}}]}`), &st); err != nil {
		t.Fatal(err)
	}
	m := New(nil, "https://nas.local:5001")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	next, _ = next.Update(snapshotMsg{s: snapshot{
		info:    dsm.SystemInfo{Model: "DS420j", DSM: "DSM 7.4.1-90080", RAMMB: 1024, Uptime: "700:37:58", TempC: 40},
		storage: st,
		packages: []dsm.Package{
			{ID: "HyperBackup", Name: "Hyper Backup", Version: "4.2.2", Status: "running"},
			{ID: "LogCenter", Name: "Log Center", Version: "1.3.2", Status: "running"},
		},
		smart: map[string]dsm.SmartStatus{
			"/dev/sdb": {Device: "/dev/sdb", Testing: true, Progress: "10%", ExtendedMinutes: 622},
			"/dev/sda": {Device: "/dev/sda", LastResult: "completed", LastType: dsm.SmartQuick, LastTime: "2022/12/11 00:00:02", ExtendedMinutes: 622},
		},
		at: time.Date(2026, 10, 3, 5, 30, 0, 0, time.UTC),
	}})
	return next.(Model)
}

func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		r := []rune(k)
		msg := tea.KeyPressMsg{Code: r[0], Text: k}
		switch k {
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		}
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

func screen(m Model) string { return m.View().Content }

func TestOverview(t *testing.T) {
	m := sample(t)
	out := screen(m)
	for _, want := range []string{"DS420j", "up 29d 4h", "/volume1", "96%", "Drive 2 (16)",
		"S.M.A.R.T. test running on /dev/sdb (10%)", "Storage pool SHR: attention (fs almost full)", "2 installed, 2 running"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview lacks %q", want)
		}
	}
	if os.Getenv("SHOW") != "" {
		t.Log("\n" + out)
	}
}

func TestStorageActionsNeedConfirmation(t *testing.T) {
	m := sample(t)
	m, _ = press(t, m, "2", "down")
	out := screen(m)
	if !strings.Contains(out, "testing 10%") || !strings.Contains(out, "16 !") {
		t.Errorf("storage screen:\n%s", out)
	}
	if os.Getenv("SHOW") != "" {
		t.Log("\n" + out)
	}
	m, cmd := press(t, m, "T")
	if cmd != nil || m.confirm == nil || !strings.Contains(m.confirm.prompt, "extended S.M.A.R.T. test on Drive 2 (/dev/sdb)? About 10h22m") {
		t.Fatalf("T must ask first: %+v", m.confirm)
	}
	if !strings.Contains(screen(m), "[y/N]") {
		t.Error("the prompt must be on screen")
	}
	m, cmd = press(t, m, "n")
	if cmd != nil || m.confirm != nil || m.note != "Cancelled." {
		t.Fatalf("anything but y cancels: %+v %q", m.confirm, m.note)
	}
	m, _ = press(t, m, "t")
	if _, cmd = press(t, m, "y"); cmd == nil {
		t.Fatal("y must run the action")
	}
}

func TestPackagesAndTabs(t *testing.T) {
	m := sample(t)
	m, _ = press(t, m, "tab", "tab")
	if m.tab != tabPackages || !strings.Contains(screen(m), "Hyper Backup") {
		t.Fatalf("tab %d:\n%s", m.tab, screen(m))
	}
	m, _ = press(t, m, "R")
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "Restart package HyperBackup") {
		t.Fatalf("restart prompt: %+v", m.confirm)
	}
	m, _ = press(t, m, "x") // cancels
	m, cmd := press(t, m, "4")
	if m.tab != tabLogs || cmd == nil {
		t.Fatal("the logs screen loads its first page on entry")
	}
	next, _ := m.Update(logsMsg{items: []dsm.LogEntry{{Time: "2026/10/02 22:23:17", Level: "info", Type: "System", Who: "admin", Text: "SNMP service was started."}}, total: 2814})
	if out := screen(next.(Model)); !strings.Contains(out, "SNMP service was started.") || !strings.Contains(out, "entries 1–1 of 2814") {
		t.Errorf("logs screen:\n%s", out)
	}
	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Error("q quits")
	}
}
