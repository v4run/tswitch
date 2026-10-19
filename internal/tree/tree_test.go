package tree

import (
	"reflect"
	"strings"
	"testing"

	"github.com/v4run/tswitch/internal/tmux"
)

func testState() tmux.State {
	return tmux.State{
		CurrentSession: "hangar",
		CurrentWindow:  2,
		Sessions: []tmux.Session{
			{Name: "hangar", Windows: []tmux.Window{{Index: 1, Name: "claude"}, {Index: 2, Name: "tmux", Active: true}}},
			{Name: "snipauth", Windows: []tmux.Window{{Index: 1, Name: "api", Active: true}, {Index: 2, Name: "zsh"}}},
			{Name: "tether", Windows: []tmux.Window{{Index: 1, Name: "zsh", Active: true}}},
		},
	}
}

// shape renders rows compactly: "S name", "  W s:i name", "-" for spacers,
// with "*" for current and "+" for folded.
func shape(rows []Row) string {
	var b strings.Builder
	for _, r := range rows {
		switch r.Kind {
		case Session:
			b.WriteString("S " + r.Name)
			if r.Folded {
				b.WriteString("+")
			}
		case Window:
			b.WriteString("  W " + r.Label() + " " + r.Name)
		case Spacer:
			b.WriteString("-")
		}
		if r.Current {
			b.WriteString("*")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestBuild_TreeWithSpacers(t *testing.T) {
	got := shape(Build(testState(), Options{}))
	want := `S hangar*
  W hangar:1 claude
  W hangar:2 tmux*
-
S snipauth
  W snipauth:1 api
  W snipauth:2 zsh
-
S tether
  W tether:1 zsh
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuild_HeaderIndexPointsAtSession(t *testing.T) {
	rows := Build(testState(), Options{})
	for i, r := range rows {
		if r.Kind == Window && rows[r.Header].Kind != Session {
			t.Errorf("row %d header %d is not a session", i, r.Header)
		}
		if r.Kind == Window && rows[r.Header].Session != r.Session {
			t.Errorf("row %d header belongs to %q", i, rows[r.Header].Session)
		}
	}
}

func TestBuild_Folded(t *testing.T) {
	got := shape(Build(testState(), Options{Folded: map[string]bool{"snipauth": true}}))
	want := `S hangar*
  W hangar:1 claude
  W hangar:2 tmux*
-
S snipauth+
-
S tether
  W tether:1 zsh
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuild_QueryWindowNameKeepsOnlyMatches(t *testing.T) {
	got := shape(Build(testState(), Options{Query: "zsh"}))
	want := `S snipauth
  W snipauth:2 zsh
-
S tether
  W tether:1 zsh
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuild_QuerySessionNameKeepsAllWindows(t *testing.T) {
	got := shape(Build(testState(), Options{Query: "snip"}))
	want := `S snipauth
  W snipauth:1 api
  W snipauth:2 zsh
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuild_MultiTermSpansSessionAndWindow(t *testing.T) {
	got := shape(Build(testState(), Options{Query: "snip zsh"}))
	want := `S snipauth
  W snipauth:2 zsh
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuild_QueryIgnoresFolding(t *testing.T) {
	got := shape(Build(testState(), Options{Query: "api", Folded: map[string]bool{"snipauth": true}}))
	want := `S snipauth
  W snipauth:1 api
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuild_SessionsOnly(t *testing.T) {
	got := shape(Build(testState(), Options{SessionsOnly: true}))
	want := "S hangar*\nS snipauth\nS tether\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuild_NoMatches(t *testing.T) {
	if rows := Build(testState(), Options{Query: "nope"}); len(rows) != 0 {
		t.Errorf("got %d rows", len(rows))
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       []int
		ok         bool
	}{
		{"mux", "tmux", []int{1, 2, 3}, true},
		{"TM", "tmux", []int{0, 1}, true},
		{"tx", "tmux", []int{0, 3}, true},
		{"xt", "tmux", nil, false},
		{"", "tmux", nil, true},
	}
	for _, c := range cases {
		got, ok := Match(c.pattern, c.s)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("Match(%q, %q) = %v, %v; want %v, %v", c.pattern, c.s, got, ok, c.want, c.ok)
		}
	}
}
