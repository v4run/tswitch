package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/v4run/tswitch/internal/tree"
)

// Colors are ANSI indices so the switcher follows the terminal theme:
// 1 red, 2 green (accent), 4 blue (matches), 8 bright black (muted).
var (
	accent    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	accentB   = accent.Bold(true)
	matchS    = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	muted     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	bold      = lipgloss.NewStyle().Bold(true)
	plain     = lipgloss.NewStyle()
	warn      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	divider   = muted.Render("│")
	minSplitW = 90
)

var helpKeys = [][2]string{
	{"j k", "move"},
	{"g G", "top · bottom"},
	{"] [", "next · previous session"},
	{"C-d C-u", "half page"},
	{"o", "open · close session"},
	{"h l", "to session · into windows"},
	{"/", "search"},
	{"c", "clear search"},
	{"enter", "switch"},
	{"x", "kill"},
	{"r", "rename"},
	{"q", "quit"},
}

// View renders the whole popup.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	h := m.bodyHeight()
	listW, prevW := m.columns()

	list := m.listLines(h, listW)
	var prev []string
	if prevW > 0 {
		prev = m.previewLines(h, prevW)
	}

	var b strings.Builder
	b.WriteString("\n")
	for i := 0; i < h; i++ {
		b.WriteString(fit(list[i], listW))
		if prevW > 0 {
			b.WriteString(divider + " " + prev[i])
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
}

// columns splits the width into list and preview. Narrow popups get the
// list alone.
func (m Model) columns() (list, preview int) {
	if m.width < minSplitW {
		return m.width, 0
	}
	list = min(max(m.width*2/5, 36), 60)
	return list, m.width - list - 2
}

func (m Model) listLines(h, w int) []string {
	lines := make([]string, h)
	if m.showHelp {
		for i, k := range helpKeys {
			if i >= h {
				break
			}
			lines[i] = "   " + accent.Render(fmt.Sprintf("%-9s", k[0])) + muted.Render(k[1])
		}
		return lines
	}
	if len(m.rows) == 0 {
		lines[0] = "   " + muted.Render("no matches")
		return lines
	}
	for i := 0; i < h && m.offset+i < len(m.rows); i++ {
		idx := m.offset + i
		lines[i] = m.renderRow(m.rows[idx], idx == m.cursor, w)
	}
	if r, ok := m.pinned(); ok {
		lines[0] = m.renderRow(r, false, w)
	}
	return lines
}

// renderRow draws one list row exactly w cells wide; "-" marks an
// unfolded session and "+" a folded one (no marker in sessions-only mode):
//
//	▎ - session               4
//	▎    2  window            ●
func (m Model) renderRow(r tree.Row, sel bool, w int) string {
	bar := " "
	if sel {
		bar = accent.Render("▎")
	}

	var left, right string
	switch r.Kind {
	case tree.Session:
		base := bold
		if sel {
			base = accentB
		}
		left = highlight(r.Name, r.Match, base)
		if !m.sessionsOnly {
			marker := "-"
			if r.Folded {
				marker = "+"
			}
			left = muted.Render(marker) + " " + left
		}
		right = muted.Render(fmt.Sprint(r.Count))
		if m.sessionsOnly && r.Current {
			right += " " + accent.Render("●")
		}
	case tree.Window:
		base := plain
		if sel {
			base = accentB
		}
		left = "  " + muted.Render(fmt.Sprintf("%2d", r.Index)) + "  " + highlight(r.Name, r.Match, base)
		if r.Current {
			right = accent.Render("●")
		}
	}

	// " " + bar + " " + left ... right + " "
	avail := w - 3 - lipgloss.Width(right) - 2
	left = ansi.Truncate(left, max(avail, 1), "…")
	line := " " + bar + " " + left
	gap := w - lipgloss.Width(line) - lipgloss.Width(right) - 1
	return line + strings.Repeat(" ", max(gap, 1)) + right + " "
}

// highlight renders s with the runes at idx in the match style and the rest
// in base, grouping runs to keep escape sequences short.
func highlight(s string, idx []int, base lipgloss.Style) string {
	if len(idx) == 0 {
		return base.Render(s)
	}
	hit := map[int]bool{}
	for _, i := range idx {
		hit[i] = true
	}
	var b strings.Builder
	var run []rune
	runHit := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		if runHit {
			b.WriteString(matchS.Render(string(run)))
		} else {
			b.WriteString(base.Render(string(run)))
		}
		run = run[:0]
	}
	for i, c := range []rune(s) {
		if hit[i] != runHit {
			flush()
			runHit = hit[i]
		}
		run = append(run, c)
	}
	flush()
	return b.String()
}

func (m Model) previewLines(h, w int) []string {
	lines := make([]string, h)
	r, ok := m.selected()
	if !ok {
		return lines
	}
	label := bold.Render(r.Session)
	if name := m.windowName(r); name != "" {
		label += muted.Render(" › ") + name
	}
	lines[0] = ansi.Truncate(label, w, "…")
	if h < 3 {
		return lines
	}

	content := strings.Split(strings.TrimRight(m.preview, "\n"), "\n")
	for len(content) > 0 && strings.TrimSpace(ansi.Strip(content[len(content)-1])) == "" {
		content = content[:len(content)-1]
	}
	room := h - 2
	if len(content) > room {
		content = content[len(content)-room:]
	}
	for i, l := range content {
		lines[2+i] = ansi.Truncate(l, w, "") + "\x1b[0m"
	}
	return lines
}

// windowName is the window shown in the preview: the row's own window, or
// the session's active window for header rows.
func (m Model) windowName(r tree.Row) string {
	if r.Kind == tree.Window {
		return r.Name
	}
	for _, s := range m.state.Sessions {
		if s.Name == r.Session {
			return s.ActiveWindow().Name
		}
	}
	return ""
}

func (m Model) footer() string {
	var left string
	switch m.mode {
	case modeSearch:
		left = accent.Render("/ ") + m.query + accent.Render("▏")
	case modeConfirm:
		r, _ := m.selected()
		what := "session " + bold.Render(r.Label())
		if r.Kind == tree.Window {
			what = "window " + bold.Render(r.Label()) + muted.Render(" · ") + r.Name
		}
		left = warn.Render("kill ") + what + warn.Render("?") + muted.Render("  y / n")
	case modeRename:
		left = accent.Render("rename ") + m.input + accent.Render("▏") + muted.Render("  enter save · esc cancel")
	default:
		switch {
		case m.status != "":
			left = warn.Render(m.status)
		case m.showHelp:
			left = muted.Render("any key to close")
		case m.query != "":
			left = matchS.Render("/ "+m.query) + muted.Render("  c clear · ? help")
		default:
			left = muted.Render("/ search · enter switch · x kill · r rename · ? help")
		}
	}

	right := ""
	if total := len(m.rows); m.query != "" || total > m.bodyHeight() {
		pos := 0
		if total > 0 {
			pos = m.cursor + 1
		}
		right = muted.Render(fmt.Sprintf("%d/%d", pos, total))
	}
	left = ansi.Truncate(left, max(m.width-lipgloss.Width(right)-4, 1), "…")
	gap := m.width - 2 - lipgloss.Width(left) - lipgloss.Width(right)
	return " " + left + strings.Repeat(" ", max(gap, 1)) + right + " "
}

// fit pads or truncates s to exactly w cells.
func fit(s string, w int) string {
	if lw := lipgloss.Width(s); lw < w {
		return s + strings.Repeat(" ", w-lw)
	}
	return ansi.Truncate(s, w, "")
}
