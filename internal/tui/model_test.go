package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/v4run/tswitch/internal/tmux"
	"github.com/v4run/tswitch/internal/tree"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// fakeClient is an in-memory tmux server.
type fakeClient struct {
	st      tmux.State
	calls   []string
	capture string
}

func (f *fakeClient) Load() (tmux.State, error) { return f.st, nil }

func (f *fakeClient) Capture(s string, i int) (string, error) {
	return f.capture, nil
}

func (f *fakeClient) Kill(s string, i int) error {
	f.calls = append(f.calls, fmt.Sprintf("kill %s %d", s, i))
	for si := range f.st.Sessions {
		if f.st.Sessions[si].Name != s {
			continue
		}
		if i < 0 {
			f.st.Sessions = append(f.st.Sessions[:si], f.st.Sessions[si+1:]...)
			return nil
		}
		ws := f.st.Sessions[si].Windows
		for wi := range ws {
			if ws[wi].Index == i {
				f.st.Sessions[si].Windows = append(ws[:wi], ws[wi+1:]...)
				return nil
			}
		}
	}
	return nil
}

func (f *fakeClient) Rename(s string, i int, name string) error {
	f.calls = append(f.calls, fmt.Sprintf("rename %s %d %s", s, i, name))
	for si := range f.st.Sessions {
		if f.st.Sessions[si].Name != s {
			continue
		}
		if i < 0 {
			f.st.Sessions[si].Name = name
			return nil
		}
		for wi := range f.st.Sessions[si].Windows {
			if f.st.Sessions[si].Windows[wi].Index == i {
				f.st.Sessions[si].Windows[wi].Name = name
			}
		}
	}
	return nil
}

func (f *fakeClient) Switch(s string, i int) error { return nil }

func newFake() *fakeClient {
	return &fakeClient{st: tmux.State{
		CurrentSession: "hangar",
		CurrentWindow:  2,
		Sessions: []tmux.Session{
			{Name: "hangar", Windows: []tmux.Window{{Index: 1, Name: "claude"}, {Index: 2, Name: "tmux", Active: true}}},
			{Name: "snipauth", Windows: []tmux.Window{{Index: 1, Name: "api", Active: true}, {Index: 2, Name: "zsh"}}},
			{Name: "tether", Windows: []tmux.Window{{Index: 1, Name: "zsh", Active: true}}},
		},
	}}
}

func newModel(t *testing.T, c *fakeClient, sessionsOnly bool) Model {
	t.Helper()
	m, err := New(c, sessionsOnly)
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 120, 20
	return m
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "bspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace}
		case "ctrl+d":
			msg = tea.KeyMsg{Type: tea.KeyCtrlD}
		case "ctrl+u":
			msg = tea.KeyMsg{Type: tea.KeyCtrlU}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func sel(m Model) string {
	r, ok := m.selected()
	if !ok {
		return "<none>"
	}
	return r.Label()
}

func TestNew_CursorOnCurrentWindow(t *testing.T) {
	m := newModel(t, newFake(), false)
	if got := sel(m); got != "hangar:2" {
		t.Errorf("got %s", got)
	}
}

func TestNew_SessionsOnlyCursorOnCurrentSession(t *testing.T) {
	c := newFake()
	c.st.Sessions[0], c.st.Sessions[1] = c.st.Sessions[1], c.st.Sessions[0]
	m := newModel(t, c, true)
	if got := sel(m); got != "hangar" {
		t.Errorf("got %s", got)
	}
}

func TestMove_CrossesIntoNextSession(t *testing.T) {
	m := press(newModel(t, newFake(), false), "j")
	if got := sel(m); got != "snipauth" {
		t.Errorf("j from hangar:2 = %s, want snipauth", got)
	}
	m = press(m, "k")
	if got := sel(m); got != "hangar:2" {
		t.Errorf("k back = %s", got)
	}
}

func TestMove_StopsAtEnds(t *testing.T) {
	m := press(newModel(t, newFake(), false), "G", "j", "j")
	if got := sel(m); got != "tether:1" {
		t.Errorf("got %s", got)
	}
	m = press(m, "g", "k")
	if got := sel(m); got != "hangar" {
		t.Errorf("got %s", got)
	}
}

func TestJumpSession(t *testing.T) {
	m := newModel(t, newFake(), false)
	m = press(m, "]")
	if got := sel(m); got != "snipauth" {
		t.Errorf("] = %s", got)
	}
	m = press(m, "]", "[")
	if got := sel(m); got != "snipauth" {
		t.Errorf("] [ = %s", got)
	}
	m = press(m, "j", "[")
	if got := sel(m); got != "snipauth" {
		t.Errorf("[ from window should land on own header, got %s", got)
	}
}

func TestFold_MinusPlusKeysAndMarkers(t *testing.T) {
	m := press(newModel(t, newFake(), false), "]", "-")
	if !m.folded["snipauth"] {
		t.Fatal("- did not fold snipauth")
	}
	view := m.View()
	if !strings.Contains(view, "+ snipauth") || !strings.Contains(view, "- hangar") {
		t.Errorf("markers missing:\n%s", view)
	}
	m = press(m, "+")
	if m.folded["snipauth"] || !strings.Contains(m.View(), "- snipauth") {
		t.Error("+ did not unfold snipauth")
	}
}

func TestView_SessionsOnlyHasNoMarkers(t *testing.T) {
	m := newModel(t, newFake(), true)
	if v := m.View(); strings.Contains(v, "- hangar") || strings.Contains(v, "+ hangar") {
		t.Errorf("sessions-only view shows fold markers:\n%s", v)
	}
}

func TestFold_FromWindowFoldsSessionAndMovesToHeader(t *testing.T) {
	m := press(newModel(t, newFake(), false), "h")
	if got := sel(m); got != "hangar" {
		t.Errorf("got %s", got)
	}
	for _, r := range m.rows {
		if r.Kind == tree.Window && r.Session == "hangar" {
			t.Fatal("hangar windows still listed")
		}
	}
	m = press(m, "l", "j")
	if got := sel(m); got != "hangar:1" {
		t.Errorf("after unfold j = %s", got)
	}
}

func TestSearch_FiltersAndEscKeepsFilter(t *testing.T) {
	m := press(newModel(t, newFake(), false), "/", "z", "s", "h")
	if got := sel(m); got != "snipauth:2" {
		t.Errorf("first match = %s", got)
	}
	m = press(m, "esc")
	if m.mode != modeNormal || m.query != "zsh" {
		t.Errorf("mode %v query %q", m.mode, m.query)
	}
	m = press(m, "j", "j")
	if got := sel(m); got != "tether:1" {
		t.Errorf("j in filtered list = %s", got)
	}
	m = press(m, "c")
	if m.query != "" || sel(m) != "tether:1" {
		t.Errorf("clear: query %q sel %s", m.query, sel(m))
	}
}

func TestKeys_RunesInOneReadAreSplit(t *testing.T) {
	m := press(newModel(t, newFake(), false), "/zsh")
	if m.mode != modeSearch || m.query != "zsh" {
		t.Errorf("mode %v query %q", m.mode, m.query)
	}
}

func TestKeys_AltKeyInSearchIsEscThenKey(t *testing.T) {
	m := press(newModel(t, newFake(), false), "/zsh")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j"), Alt: true})
	m = next.(Model)
	if m.mode != modeNormal || m.query != "zsh" || sel(m) != "tether" {
		t.Errorf("mode %v query %q sel %s", m.mode, m.query, sel(m))
	}
}

func TestSearch_LettersDoNotTriggerCommands(t *testing.T) {
	m := press(newModel(t, newFake(), false), "/", "x", "q")
	if m.mode != modeSearch || m.query != "xq" {
		t.Errorf("mode %v query %q", m.mode, m.query)
	}
}

func TestSearch_ClearKeepsSelection(t *testing.T) {
	m := press(newModel(t, newFake(), false), "/", "a", "p", "i")
	if got := sel(m); got != "snipauth:1" {
		t.Fatalf("match = %s", got)
	}
	m = press(m, "esc", "c")
	if got := sel(m); got != "snipauth:1" || len(m.rows) != 8 {
		t.Errorf("after clear sel %s rows %d", got, len(m.rows))
	}
}

func TestEnter_SetsChoice(t *testing.T) {
	m := press(newModel(t, newFake(), false), "j", "j")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	s, i, ok := next.(Model).Choice()
	if !ok || s != "snipauth" || i != 1 {
		t.Errorf("choice = %s %d %v", s, i, ok)
	}
}

func TestKill_ConfirmY(t *testing.T) {
	c := newFake()
	m := press(newModel(t, c, false), "k", "x")
	if m.mode != modeConfirm {
		t.Fatal("not confirming")
	}
	m = press(m, "y")
	if len(c.calls) != 1 || c.calls[0] != "kill hangar 1" {
		t.Errorf("calls %v", c.calls)
	}
	if got := sel(m); got != "hangar:2" {
		t.Errorf("after kill sel = %s", got)
	}
}

func TestKill_AnyOtherKeyCancels(t *testing.T) {
	c := newFake()
	m := press(newModel(t, c, false), "x", "n")
	if len(c.calls) != 0 || m.mode != modeNormal {
		t.Errorf("calls %v mode %v", c.calls, m.mode)
	}
}

func TestRename_Session(t *testing.T) {
	c := newFake()
	m := press(newModel(t, c, false), "]", "r", "ctrl+u", "a", "u", "t", "h", "enter")
	if len(c.calls) != 1 || c.calls[0] != "rename snipauth -1 auth" {
		t.Errorf("calls %v", c.calls)
	}
	if got := sel(m); got != "auth" {
		t.Errorf("sel = %s", got)
	}
}

func TestRename_EscCancels(t *testing.T) {
	c := newFake()
	m := press(newModel(t, c, false), "r", "z", "esc")
	if len(c.calls) != 0 || m.mode != modeNormal {
		t.Errorf("calls %v mode %v", c.calls, m.mode)
	}
}

func bigFake() *fakeClient {
	c := &fakeClient{st: tmux.State{CurrentSession: "s0", CurrentWindow: 1}}
	for s := 0; s < 4; s++ {
		sess := tmux.Session{Name: fmt.Sprintf("s%d", s)}
		for w := 1; w <= 8; w++ {
			sess.Windows = append(sess.Windows, tmux.Window{Index: w, Name: fmt.Sprintf("w%d", w), Active: w == 1})
		}
		c.st.Sessions = append(c.st.Sessions, sess)
	}
	return c
}

func TestScroll_KeepsCursorVisibleWithScrolloff(t *testing.T) {
	m := newModel(t, bigFake(), false)
	m.height = 13 // body 10
	for i := 0; i < 20; i++ {
		m = press(m, "j")
		h := m.bodyHeight()
		if m.cursor < m.offset || m.cursor >= m.offset+h {
			t.Fatalf("cursor %d outside [%d,%d)", m.cursor, m.offset, m.offset+h)
		}
		if m.offset > 0 && m.cursor-m.offset < scrolloff {
			t.Fatalf("scrolloff violated: cursor %d offset %d", m.cursor, m.offset)
		}
	}
}

func TestView_PinnedHeaderWhenScrolledIntoWindows(t *testing.T) {
	m := newModel(t, bigFake(), false)
	m.height = 13
	m = press(m, "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j")
	if _, ok := m.pinned(); !ok {
		t.Fatalf("expected pinned header at offset %d cursor %d", m.offset, m.cursor)
	}
	lines := strings.Split(m.View(), "\n")
	first := lines[1] // line 0 is top padding
	r, _ := m.pinned()
	if !strings.Contains(first, r.Name) {
		t.Errorf("first list line %q does not show pinned %q", first, r.Name)
	}
}

func TestView_LayoutAndFooter(t *testing.T) {
	c := newFake()
	c.capture = "$ ls\nfoo bar\n\n\n"
	m := newModel(t, c, false)
	next, _ := m.Update(previewMsg{key: m.previewKey, content: c.capture})
	m = next.(Model)
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Errorf("view has %d lines, want %d", len(lines), m.height)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > m.width {
			t.Errorf("line %d is %d wide (> %d): %q", i, w, m.width, l)
		}
	}
	for _, want := range []string{"hangar", "▎", "●", "hangar › tmux", "foo bar", "/ search"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

func TestView_NarrowHidesPreview(t *testing.T) {
	m := newModel(t, newFake(), false)
	m.width = 60
	if strings.Contains(m.View(), "│") {
		t.Error("preview divider shown in narrow view")
	}
}

func TestView_HelpOverlay(t *testing.T) {
	m := press(newModel(t, newFake(), false), "?")
	if !strings.Contains(m.View(), "fold · unfold") {
		t.Error("help not shown")
	}
	m = press(m, "j")
	if m.showHelp || sel(m) != "hangar:2" {
		t.Errorf("key should only close help: help %v sel %s", m.showHelp, sel(m))
	}
}

func TestPreview_StaleResultIgnored(t *testing.T) {
	m := newModel(t, newFake(), false)
	next, _ := m.Update(previewMsg{key: tree.Key{Kind: tree.Window, Session: "other", Index: 9}, content: "stale"})
	if next.(Model).preview == "stale" {
		t.Error("stale preview applied")
	}
}
