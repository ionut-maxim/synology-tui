package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	bold      = lipgloss.NewStyle().Bold(true)
	dim       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	badStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	okStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	label     = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Width(10)
	activeTab = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Padding(0, 1)
	otherTab  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1)
	askStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("3")).Padding(0, 1)
)

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "synology-tui · " + m.snap.info.Model
	return v
}

func (m Model) render() string {
	var b strings.Builder
	title := bold.Render("synology-tui")
	if m.loaded {
		title += dim.Render(fmt.Sprintf(" · %s · %s · %s", m.snap.info.Model, m.snap.info.DSM, m.host))
	}
	state := "updated " + m.snap.at.Format("15:04:05")
	if m.loading {
		state = "loading…"
	}
	b.WriteString(spread(title, dim.Render(state), m.width) + "\n")

	var tabs []string
	for i, n := range tabNames {
		s := fmt.Sprintf("%d %s", i+1, n)
		if tab(i) == m.tab {
			tabs = append(tabs, activeTab.Render(s))
		} else {
			tabs = append(tabs, otherTab.Render(s))
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tabs...) + "\n\n")

	if !m.loaded {
		if m.err != nil {
			b.WriteString(badStyle.Render("✗ " + m.err.Error()))
		} else {
			b.WriteString(dim.Render("Loading…"))
		}
	} else {
		switch m.tab {
		case tabOverview:
			b.WriteString(m.overview())
		case tabStorage:
			b.WriteString(m.storage())
		case tabPackages:
			b.WriteString(m.pkgs.View())
		case tabLogs:
			b.WriteString(m.logs.View())
			if m.logTotal > 0 {
				b.WriteString("\n" + dim.Render(fmt.Sprintf("entries %d–%d of %d, newest first",
					m.logOffset+1, m.logOffset+len(m.logItems), m.logTotal)))
			}
		}
	}

	body := b.String()
	footer := m.footer()
	if gap := m.height - lipgloss.Height(body) - lipgloss.Height(footer); gap > 0 {
		body += strings.Repeat("\n", gap)
	}
	return body + "\n" + footer
}

func (m Model) footer() string {
	if m.confirm != nil {
		return askStyle.Render(m.confirm.prompt+" [y/N]") + "\n"
	}
	line := ""
	switch {
	case m.note != "" && strings.HasPrefix(m.note, "✗"):
		line = badStyle.Render(m.note)
	case m.note != "":
		line = okStyle.Render(m.note)
	case m.err != nil && m.loaded:
		line = warnStyle.Render("Some data didn't load: " + m.err.Error())
	}
	return line + "\n" + dim.Render(helpFor(m.tab))
}

func (m Model) overview() string {
	s := m.snap
	var b strings.Builder
	row := func(k, v string) { b.WriteString(label.Render(k) + v + "\n") }

	temp := fmt.Sprintf("%d °C", s.info.TempC)
	if s.info.TempWarning {
		temp = badStyle.Render(temp + " (warning)")
	}
	row("System", fmt.Sprintf("%s · %s · up %s · %s · %d MB RAM", s.info.Model, s.info.DSM, uptime(s.info.Uptime), temp, s.info.RAMMB))
	load := fmt.Sprintf("CPU %d%% · RAM %d%% · swap %d%%", s.util.CPUPercent(), s.util.Memory.UsedPercent, s.util.Memory.SwapPercent)
	for _, n := range s.util.Network {
		if n.Device == "total" {
			load += fmt.Sprintf(" · net ↓ %s/s ↑ %s/s", rate(n.RxBps), rate(n.TxBps))
		}
	}
	row("Load", load)
	for _, v := range s.storage.Volumes {
		pct := v.UsedPercent()
		used := fmt.Sprintf("%s %.0f%%", bar(pct, 20), pct)
		switch {
		case pct >= 90:
			used = badStyle.Render(used)
		case pct >= 80:
			used = warnStyle.Render(used)
		}
		row("Volume", fmt.Sprintf("%-9s %s of %s  %s  %s", v.Path, v.Size.Used, v.Size.Total, used, status(v.Status)))
	}
	for _, p := range s.storage.Pools {
		row("Pool", fmt.Sprintf("%s (%s), %d disks, %s", p.Description, strings.ReplaceAll(p.Type, "_", " "), len(p.Disks), status(p.Status)))
	}
	minT, maxT, bad := 1000, 0, []string{}
	for _, d := range s.storage.Disks {
		minT, maxT = min(minT, d.TempC), max(maxT, d.TempC)
		if d.BadSectors > 0 {
			bad = append(bad, fmt.Sprintf("%s (%d)", d.Name, d.BadSectors))
		}
	}
	disks := fmt.Sprintf("%d · %d–%d °C", len(s.storage.Disks), minT, maxT)
	if len(bad) > 0 {
		disks += " · bad sectors: " + badStyle.Render(strings.Join(bad, ", "))
	}
	for dev, st := range s.smart {
		if st.Testing {
			disks += " · " + warnStyle.Render(fmt.Sprintf("S.M.A.R.T. test running on %s (%s)", dev, st.Progress))
		}
	}
	row("Disks", disks)
	running := 0
	for _, p := range s.packages {
		if p.Running() {
			running++
		}
	}
	row("Packages", fmt.Sprintf("%d installed, %d running", len(s.packages), running))

	if ws := s.warnings(); len(ws) > 0 {
		b.WriteString("\n" + bold.Render("Warnings") + "\n")
		for _, w := range ws {
			b.WriteString(warnStyle.Render("  ! "+w) + "\n")
		}
	} else {
		b.WriteString("\n" + okStyle.Render("No warnings.") + "\n")
	}
	return b.String()
}

func (m Model) storage() string {
	var b strings.Builder
	b.WriteString(m.disks.View() + "\n\n")
	if d, ok := m.selectedDisk(); ok {
		b.WriteString(bold.Render(d.Name) + dim.Render(fmt.Sprintf(" · %s %s · firmware %s · %s · in %s", d.Vendor, d.Model, d.Firmware, d.Type, d.UsedBy)) + "\n")
		b.WriteString(label.Render("Self-test") + smartLine(m.snap.smart[d.Device]) + "\n")
	}
	b.WriteString("\n")
	for _, p := range m.snap.storage.Pools {
		b.WriteString(label.Render("Pool") + fmt.Sprintf("%s (%s): %s, disks %s\n", p.Description, strings.ReplaceAll(p.Type, "_", " "), status(p.Status), strings.Join(p.Disks, " ")))
	}
	for _, v := range m.snap.storage.Volumes {
		detail := ""
		if v.SpaceStatus.Detail != "" && v.Status != "normal" {
			detail = " (" + strings.ReplaceAll(v.SpaceStatus.Detail, "_", " ") + ")"
		}
		b.WriteString(label.Render("Volume") + fmt.Sprintf("%s %s: %s%s, %.0f%% used\n", v.Path, v.FS, status(v.Status), detail, v.UsedPercent()))
	}
	return b.String()
}

func status(s string) string {
	switch s {
	case "normal":
		return okStyle.Render(s)
	case "attention", "warning":
		return warnStyle.Render(s)
	default:
		return badStyle.Render(s)
	}
}

func rate(bps int64) string {
	switch {
	case bps >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(bps)/1e6)
	case bps >= 1_000:
		return fmt.Sprintf("%.0f kB", float64(bps)/1e3)
	default:
		return fmt.Sprintf("%d B", bps)
	}
}

// spread puts left and right on one line of the given width.
func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}
