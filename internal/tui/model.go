// Package tui is the Bubble Tea switcher: a session/window tree with vim
// keys, inline search, kill/rename prompts and a live pane preview.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/v4run/tswitch/internal/tmux"
	"github.com/v4run/tswitch/internal/tree"
)

type mode int

const (
	modeNormal mode = iota
	modeSearch
	modeConfirm
	modeRename
)

// scrolloff is how many rows stay visible above and below the cursor.
const scrolloff = 2

// Model is the switcher state.
type Model struct {
	client       tmux.Client
	sessionsOnly bool

	state  tmux.State
	rows   []tree.Row
	folded map[string]bool
	query  string

	cursor int
	offset int
	width  int
	height int

	mode     mode
	input    string // rename buffer
	showHelp bool
	status   string // last error, cleared on the next key

	preview    string
	previewKey tree.Key

	choice *tree.Row
}

// New loads the server state and puts the cursor on the current window (or
// the current session in sessions-only mode).
func New(client tmux.Client, sessionsOnly bool) (Model, error) {
	st, err := client.Load()
	if err != nil {
		return Model{}, err
	}
	m := Model{
		client:       client,
		sessionsOnly: sessionsOnly,
		state:        st,
		folded:       map[string]bool{},
		width:        80,
		height:       24,
	}
	m.rebuild()
	start := tree.Key{Kind: tree.Window, Session: st.CurrentSession, Index: st.CurrentWindow}
	if sessionsOnly {
		start = tree.Key{Kind: tree.Session, Session: st.CurrentSession, Index: -1}
	}
	m.restore(start, 0)
	if r, ok := m.selected(); ok {
		m.previewKey = r.Key()
	}
	return m, nil
}

// Choice returns the row picked with enter, if any.
func (m Model) Choice() (session string, index int, ok bool) {
	if m.choice == nil {
		return "", 0, false
	}
	return m.choice.Session, m.choice.Index, true
}

type previewMsg struct {
	key     tree.Key
	content string
	err     error
}

// Init loads the preview for the starting row.
func (m Model) Init() tea.Cmd {
	if r, ok := m.selected(); ok {
		return m.capture(r)
	}
	return nil
}

// Update is the message handler.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case previewMsg:
		if msg.key == m.previewKey {
			m.preview = msg.content
			if msg.err != nil {
				m.preview = msg.err.Error()
			}
		}
		return m, nil
	case tea.KeyMsg:
		cmd = m.handleKeys(msg)
	}
	m.scroll()
	return m, tea.Batch(cmd, m.syncPreview())
}

// handleKeys normalises what the terminal delivers before dispatching:
// runes that arrive in one read ("/zsh" when typed fast or pasted) are fed
// one at a time since any of them may change the mode, and an esc that
// arrives glued to the next key comes through as alt+key, so it is split
// back into esc then the key (in normal mode the esc is dropped rather than
// quitting).
func (m *Model) handleKeys(msg tea.KeyMsg) tea.Cmd {
	if msg.Alt {
		msg.Alt = false
		if m.mode != modeNormal {
			if cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc}); cmd != nil {
				return cmd
			}
		}
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) <= 1 {
		return m.handleKey(msg)
	}
	for _, r := range msg.Runes {
		if cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}); cmd != nil {
			return cmd
		}
	}
	return nil
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	m.status = ""
	switch m.mode {
	case modeSearch:
		return m.searchKey(msg)
	case modeConfirm:
		return m.confirmKey(msg)
	case modeRename:
		return m.renameKey(msg)
	}
	return m.normalKey(msg)
}

func (m *Model) normalKey(msg tea.KeyMsg) tea.Cmd {
	if m.showHelp {
		// Any key closes help; quit keys also quit.
		m.showHelp = false
		switch msg.String() {
		case "q", "ctrl+c":
			return tea.Quit
		}
		return nil
	}
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return tea.Quit
	case "j", "down", "ctrl+n":
		m.move(1)
	case "k", "up", "ctrl+p":
		m.move(-1)
	case "g", "home":
		m.cursor = 0
		m.snap(1)
	case "G", "end":
		m.cursor = len(m.rows) - 1
		m.snap(-1)
	case "ctrl+d":
		m.move(max(1, m.bodyHeight()/2))
	case "ctrl+u":
		m.move(-max(1, m.bodyHeight()/2))
	case "]":
		m.jumpSession(1)
	case "[":
		m.jumpSession(-1)
	case "h", "left":
		m.fold(true)
	case "l", "right":
		m.fold(false)
	case "/":
		m.mode = modeSearch
	case "c":
		m.setQuery("")
	case "?":
		m.showHelp = true
	case "enter":
		if r, ok := m.selected(); ok {
			m.choice = &r
			return tea.Quit
		}
	case "x":
		if _, ok := m.selected(); ok {
			m.mode = modeConfirm
		}
	case "r":
		if r, ok := m.selected(); ok {
			m.mode = modeRename
			m.input = r.Name
		}
	}
	return nil
}

func (m *Model) searchKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.mode = modeNormal
	case tea.KeyEnter:
		if r, ok := m.selected(); ok {
			m.choice = &r
			return tea.Quit
		}
	case tea.KeyUp, tea.KeyCtrlP:
		m.move(-1)
	case tea.KeyDown, tea.KeyCtrlN:
		m.move(1)
	case tea.KeyBackspace:
		if q := []rune(m.query); len(q) > 0 {
			m.setQuery(string(q[:len(q)-1]))
		}
	case tea.KeyCtrlW:
		m.setQuery(dropWord(m.query))
	case tea.KeyCtrlU:
		m.setQuery("")
	case tea.KeySpace:
		m.setQuery(m.query + " ")
	case tea.KeyRunes:
		m.setQuery(m.query + string(msg.Runes))
	}
	return nil
}

func (m *Model) confirmKey(msg tea.KeyMsg) tea.Cmd {
	m.mode = modeNormal
	if msg.String() != "y" && msg.String() != "Y" {
		return nil
	}
	r, ok := m.selected()
	if !ok {
		return nil
	}
	if err := m.client.Kill(r.Session, r.Index); err != nil {
		m.status = err.Error()
		return nil
	}
	if r.Kind == tree.Session {
		delete(m.folded, r.Session)
	}
	return m.reload(r.Key())
}

func (m *Model) renameKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.mode = modeNormal
	case tea.KeyEnter:
		m.mode = modeNormal
		r, ok := m.selected()
		name := strings.TrimSpace(m.input)
		if !ok || name == "" || name == r.Name {
			return nil
		}
		if err := m.client.Rename(r.Session, r.Index, name); err != nil {
			m.status = err.Error()
			return nil
		}
		key := r.Key()
		if r.Kind == tree.Session {
			if m.folded[r.Session] {
				delete(m.folded, r.Session)
				m.folded[name] = true
			}
			key.Session = name
		}
		return m.reload(key)
	case tea.KeyBackspace:
		if in := []rune(m.input); len(in) > 0 {
			m.input = string(in[:len(in)-1])
		}
	case tea.KeyCtrlW:
		m.input = dropWord(m.input)
	case tea.KeyCtrlU:
		m.input = ""
	case tea.KeySpace:
		m.input += " "
	case tea.KeyRunes:
		m.input += string(msg.Runes)
	}
	return nil
}

// reload re-reads the server after a kill or rename and puts the cursor back
// on key, or on the nearest row if key is gone. If the server went away
// with the last session, the switcher quits.
func (m *Model) reload(key tree.Key) tea.Cmd {
	st, err := m.client.Load()
	if err != nil {
		return tea.Quit
	}
	m.state = st
	at := m.cursor
	m.rebuild()
	m.restore(key, at)
	return nil
}

func (m *Model) setQuery(q string) {
	prev, hadPrev := m.selected()
	m.query = q
	m.rebuild()
	if q == "" {
		if hadPrev {
			m.restore(prev.Key(), 0)
		} else {
			m.restore(tree.Key{Kind: tree.Window, Session: m.state.CurrentSession, Index: m.state.CurrentWindow}, 0)
		}
		return
	}
	m.cursor = m.bestMatch()
}

// bestMatch is the first window whose own name matched, else the first
// session header, else 0.
func (m *Model) bestMatch() int {
	for i, r := range m.rows {
		if r.Kind == tree.Window && len(r.Match) > 0 {
			return i
		}
	}
	for i, r := range m.rows {
		if r.Selectable() {
			return i
		}
	}
	return 0
}

func (m *Model) rebuild() {
	m.rows = tree.Build(m.state, tree.Options{Query: m.query, Folded: m.folded, SessionsOnly: m.sessionsOnly})
}

// restore puts the cursor on key, or on the selectable row nearest to
// fallback.
func (m *Model) restore(key tree.Key, fallback int) {
	for i, r := range m.rows {
		if r.Key() == key {
			m.cursor = i
			return
		}
	}
	m.cursor = min(max(fallback, 0), max(len(m.rows)-1, 0))
	m.snap(1)
}

// snap moves the cursor off a spacer, preferring dir, then the other way.
func (m *Model) snap(dir int) {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	for _, d := range []int{dir, -dir} {
		for i := m.cursor; i >= 0 && i < len(m.rows); i += d {
			if m.rows[i].Selectable() {
				m.cursor = i
				return
			}
		}
	}
}

// move steps n selectable rows (negative goes up), stopping at the ends.
func (m *Model) move(n int) {
	dir := 1
	if n < 0 {
		dir, n = -1, -n
	}
	for i := m.cursor + dir; n > 0 && i >= 0 && i < len(m.rows); i += dir {
		if m.rows[i].Selectable() {
			m.cursor = i
			n--
		}
	}
}

// jumpSession goes to the next/previous session header. Going back from a
// window lands on its own header first, like vim's [[.
func (m *Model) jumpSession(dir int) {
	for i := m.cursor + dir; i >= 0 && i < len(m.rows); i += dir {
		if m.rows[i].Kind == tree.Session {
			m.cursor = i
			return
		}
	}
}

func (m *Model) fold(fold bool) {
	if m.query != "" || m.sessionsOnly {
		return
	}
	r, ok := m.selected()
	if !ok {
		return
	}
	if fold == m.folded[r.Session] {
		if fold && r.Kind == tree.Window {
			m.cursor = r.Header
		}
		return
	}
	if fold {
		m.folded[r.Session] = true
	} else {
		delete(m.folded, r.Session)
	}
	m.rebuild()
	m.restore(tree.Key{Kind: tree.Session, Session: r.Session, Index: -1}, m.cursor)
}

func (m Model) selected() (tree.Row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) || !m.rows[m.cursor].Selectable() {
		return tree.Row{}, false
	}
	return m.rows[m.cursor], true
}

// bodyHeight is the number of list rows: total minus the top padding line,
// the blank line above the footer, and the footer.
func (m Model) bodyHeight() int {
	return max(1, m.height-3)
}

// scroll keeps the cursor within scrolloff rows of the viewport edges.
func (m *Model) scroll() {
	h := m.bodyHeight()
	so := min(scrolloff, (h-1)/2)
	if m.cursor < m.offset+so {
		m.offset = m.cursor - so
	}
	if m.cursor > m.offset+h-1-so {
		m.offset = m.cursor - h + 1 + so
	}
	m.offset = min(max(m.offset, 0), max(len(m.rows)-h, 0))
}

// pinned returns the session header to draw over the first visible line
// when the viewport starts inside that session's windows.
func (m Model) pinned() (tree.Row, bool) {
	if m.offset == 0 || m.offset >= len(m.rows) || m.cursor == m.offset {
		return tree.Row{}, false
	}
	r := m.rows[m.offset]
	if r.Kind != tree.Window || r.Header >= m.offset {
		return tree.Row{}, false
	}
	return m.rows[r.Header], true
}

func (m *Model) syncPreview() tea.Cmd {
	r, ok := m.selected()
	if !ok || r.Key() == m.previewKey {
		return nil
	}
	m.previewKey = r.Key()
	return m.capture(r)
}

func (m Model) capture(r tree.Row) tea.Cmd {
	c, key := m.client, r.Key()
	return func() tea.Msg {
		out, err := c.Capture(r.Session, r.Index)
		return previewMsg{key: key, content: out, err: err}
	}
}

func dropWord(s string) string {
	s = strings.TrimRight(s, " ")
	if i := strings.LastIndex(s, " "); i >= 0 {
		return s[:i+1]
	}
	return ""
}
