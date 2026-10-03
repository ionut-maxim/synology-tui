// Package ui is the Bubble Tea front end: four screens over one snapshot of
// the NAS, refreshed every 30 seconds, plus confirmed actions.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ionut-maxim/synology-tui/dsm"
)

type tab int

const (
	tabOverview tab = iota
	tabStorage
	tabPackages
	tabLogs
)

var tabNames = []string{"Overview", "Storage", "Packages", "Logs"}

const (
	refreshEvery = 30 * time.Second
	logPage      = 100
)

// Model is the whole app.
type Model struct {
	c             *dsm.Client
	host          string
	width, height int

	tab     tab
	snap    snapshot
	loaded  bool
	loading bool
	err     error
	note    string // result of the last action

	disks, pkgs, logs table.Model
	logItems          []dsm.LogEntry
	logOffset         int
	logTotal          int

	confirm *confirmation
}

// confirmation is an action waiting for y/N.
type confirmation struct {
	prompt string
	run    tea.Cmd
}

type (
	snapshotMsg struct {
		s   snapshot
		err error
	}
	logsMsg struct {
		items         []dsm.LogEntry
		total, offset int
		err           error
	}
	actionMsg struct {
		text string
		err  error
	}
	tickMsg time.Time
)

// New returns the app for a logged-in client.
func New(c *dsm.Client, host string) Model {
	mk := func(cols ...table.Column) table.Model {
		t := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(10))
		st := table.DefaultStyles()
		st.Header = st.Header.Bold(true).Foreground(lipgloss.Color("12"))
		st.Selected = st.Selected.Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12"))
		t.SetStyles(st)
		return t
	}
	return Model{
		c: c, host: host, loading: true,
		disks: mk(
			table.Column{Title: "Drive", Width: 9}, table.Column{Title: "Device", Width: 9},
			table.Column{Title: "Model", Width: 22}, table.Column{Title: "Size", Width: 8},
			table.Column{Title: "Temp", Width: 6}, table.Column{Title: "Bad sectors", Width: 11},
			table.Column{Title: "Status", Width: 9}, table.Column{Title: "S.M.A.R.T.", Width: 12},
		),
		pkgs: mk(
			table.Column{Title: "Package", Width: 28}, table.Column{Title: "Name", Width: 32},
			table.Column{Title: "Version", Width: 18}, table.Column{Title: "Status", Width: 10},
		),
		logs: mk(
			table.Column{Title: "Time", Width: 19}, table.Column{Title: "Level", Width: 7},
			table.Column{Title: "Type", Width: 14}, table.Column{Title: "Who", Width: 18},
			table.Column{Title: "Message", Width: 60},
		),
	}
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.refresh(), tick()) }

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) refresh() tea.Cmd {
	c := m.c
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		s, err := load(ctx, c)
		return snapshotMsg{s, err}
	}
}

func (m Model) loadLogs(offset int) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		items, total, err := c.Logs(ctx, offset, logPage)
		return logsMsg{items, total, offset, err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tickMsg:
		cmds := []tea.Cmd{tick()}
		if !m.loading {
			m.loading = true
			cmds = append(cmds, m.refresh())
		}
		return m, tea.Batch(cmds...)

	case snapshotMsg:
		m.loading, m.err = false, msg.err
		if !msg.s.at.IsZero() {
			m.snap, m.loaded = msg.s, true
			m.fillTables()
		}
		return m, nil

	case logsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.logItems, m.logTotal, m.logOffset = msg.items, msg.total, msg.offset
		rows := make([]table.Row, 0, len(msg.items))
		for _, l := range msg.items {
			rows = append(rows, table.Row{l.Time, l.Level, l.Type, l.Who, l.Text})
		}
		m.logs.SetRows(rows)
		m.logs.SetCursor(0)
		return m, nil

	case actionMsg:
		if msg.err != nil {
			m.note = "✗ " + msg.err.Error()
		} else {
			m.note = "✓ " + msg.text
		}
		m.loading = true
		return m, m.refresh()

	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.confirm != nil {
		c := m.confirm
		m.confirm = nil
		if k == "y" || k == "Y" {
			m.note = "… working"
			return m, c.run
		}
		m.note = "Cancelled."
		return m, nil
	}
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "right":
		return m.switchTab((m.tab + 1) % tab(len(tabNames)))
	case "shift+tab", "left":
		return m.switchTab((m.tab + tab(len(tabNames)) - 1) % tab(len(tabNames)))
	case "1", "2", "3", "4":
		return m.switchTab(tab(k[0] - '1'))
	case "r":
		m.loading, m.note = true, ""
		cmds := []tea.Cmd{m.refresh()}
		if m.tab == tabLogs {
			cmds = append(cmds, m.loadLogs(m.logOffset))
		}
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	switch m.tab {
	case tabStorage:
		if d, ok := m.selectedDisk(); ok {
			switch k {
			case "t":
				return m.ask(fmt.Sprintf("Run a quick S.M.A.R.T. test on %s (%s)? About %s; it only reads.",
					d.Name, d.Device, minutes(m.snap.smart[d.Device].QuickMinutes, 1)), m.smartCmd(d, dsm.SmartQuick))
			case "T":
				return m.ask(fmt.Sprintf("Run an extended S.M.A.R.T. test on %s (%s)? About %s: it reads the whole disk (data untouched) and slows it while it runs.",
					d.Name, d.Device, minutes(m.snap.smart[d.Device].ExtendedMinutes, 0)), m.smartCmd(d, dsm.SmartExtended))
			case "x":
				return m.ask(fmt.Sprintf("Stop the S.M.A.R.T. test on %s (%s)?", d.Name, d.Device), m.smartCmd(d, dsm.SmartStop))
			}
		}
		m.disks, cmd = m.disks.Update(msg)
	case tabPackages:
		if p, ok := m.selectedPackage(); ok {
			switch k {
			case "s":
				return m.ask(fmt.Sprintf("Start package %s?", p.ID), m.packageCmd(p, "start"))
			case "S":
				return m.ask(fmt.Sprintf("Stop package %s? Whatever depends on it stops working.", p.ID), m.packageCmd(p, "stop"))
			case "R":
				return m.ask(fmt.Sprintf("Restart package %s (stop, then start)?", p.ID), m.packageCmd(p, "restart"))
			}
		}
		m.pkgs, cmd = m.pkgs.Update(msg)
	case tabLogs:
		switch k {
		case "n":
			if m.logOffset+logPage < m.logTotal {
				return m, m.loadLogs(m.logOffset + logPage)
			}
			return m, nil
		case "p":
			if m.logOffset > 0 {
				return m, m.loadLogs(max(0, m.logOffset-logPage))
			}
			return m, nil
		}
		m.logs, cmd = m.logs.Update(msg)
	}
	return m, cmd
}

func (m Model) switchTab(t tab) (tea.Model, tea.Cmd) {
	m.tab, m.note = t, ""
	if t == tabLogs && m.logItems == nil {
		return m, m.loadLogs(0)
	}
	return m, nil
}

func (m Model) ask(prompt string, run tea.Cmd) (tea.Model, tea.Cmd) {
	m.confirm = &confirmation{prompt: prompt, run: run}
	return m, nil
}

func (m Model) selectedDisk() (dsm.Disk, bool) {
	i := m.disks.Cursor()
	if i < 0 || i >= len(m.snap.storage.Disks) {
		return dsm.Disk{}, false
	}
	return m.snap.storage.Disks[i], true
}

func (m Model) selectedPackage() (dsm.Package, bool) {
	i := m.pkgs.Cursor()
	if i < 0 || i >= len(m.snap.packages) {
		return dsm.Package{}, false
	}
	return m.snap.packages[i], true
}

func (m Model) smartCmd(d dsm.Disk, t dsm.SmartTest) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := c.StartSmartTest(ctx, d.Device, t)
		text := fmt.Sprintf("Started a %s S.M.A.R.T. test on %s.",
			map[dsm.SmartTest]string{dsm.SmartQuick: "quick", dsm.SmartExtended: "extended"}[t], d.Name)
		if t == dsm.SmartStop {
			text = "Stopped the S.M.A.R.T. test on " + d.Name + "."
		}
		return actionMsg{text, err}
	}
}

func (m Model) packageCmd(p dsm.Package, op string) tea.Cmd {
	c := m.c
	return func() tea.Msg {
		// DSM's own UI allows an hour; most packages take seconds.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var err error
		switch op {
		case "start":
			err = c.StartPackage(ctx, p.ID)
		case "stop":
			err = c.StopPackage(ctx, p.ID)
		default:
			err = c.RestartPackage(ctx, p.ID)
		}
		return actionMsg{fmt.Sprintf("%s: %s done.", p.ID, op), err}
	}
}

func (m *Model) fillTables() {
	var rows []table.Row
	for _, d := range m.snap.storage.Disks {
		bad := fmt.Sprint(d.BadSectors)
		if d.BadSectors > 0 {
			bad += " !"
		}
		smart := d.SmartStatus
		if st := m.snap.smart[d.Device]; st.Testing {
			smart = "testing " + st.Progress
		}
		rows = append(rows, table.Row{d.Name, d.Device, d.Model, d.SizeBytes.String(), fmt.Sprintf("%d °C", d.TempC), bad, d.Status, smart})
	}
	m.disks.SetRows(rows)
	rows = nil
	for _, p := range m.snap.packages {
		rows = append(rows, table.Row{p.ID, p.Name, p.Version, p.Status})
	}
	m.pkgs.SetRows(rows)
}

func (m *Model) resize() {
	body := max(m.height-6, 4) // title, tabs, blank, footer (2), margin
	m.disks.SetHeight(max(body-9, 3))
	m.pkgs.SetHeight(body)
	m.logs.SetHeight(body - 1)
	cols := m.logs.Columns()
	used := 0
	for _, c := range cols[:len(cols)-1] {
		used += c.Width + 2
	}
	cols[len(cols)-1].Width = max(m.width-used-2, 20)
	m.logs.SetColumns(cols)
	for _, t := range []*table.Model{&m.disks, &m.pkgs, &m.logs} {
		t.SetWidth(m.width)
	}
}

// minutes renders DSM's estimate ("10h22m"); def when DSM gave none.
func minutes(n, def int) string {
	if n <= 0 {
		n = def
	}
	if n <= 0 {
		return "a while"
	}
	if n < 60 {
		return fmt.Sprintf("%d min", n)
	}
	return fmt.Sprintf("%dh%02dm", n/60, n%60)
}

// helpFor is the key help of a screen.
func helpFor(t tab) string {
	common := "tab/1-4 screens · r refresh · q quit"
	switch t {
	case tabStorage:
		return "↑/↓ disk · t quick test · T extended test · x stop test · " + common
	case tabPackages:
		return "↑/↓ package · s start · S stop · R restart · " + common
	case tabLogs:
		return "↑/↓ scroll · n older · p newer · " + common
	}
	return common
}

func bar(pct float64, width int) string {
	filled := min(max(int(pct/100*float64(width)), 0), width)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
