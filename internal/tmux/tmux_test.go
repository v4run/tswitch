package tmux

import (
	"reflect"
	"testing"
)

func TestParse_CurrentSessionFirstRestInOrder(t *testing.T) {
	st, err := Parse(
		"hangar\t3\n",
		"0\nhangar\nsnipauth\n",
		"0\t1\t0\tzsh\n0\t2\t1\tzsh\nhangar\t1\t0\tclaude\nhangar\t3\t1\ttmux\nsnipauth\t1\t1\tapi server\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := State{
		CurrentSession: "hangar",
		CurrentWindow:  3,
		Sessions: []Session{
			{Name: "hangar", Windows: []Window{{1, "claude", false}, {3, "tmux", true}}},
			{Name: "0", Windows: []Window{{1, "zsh", false}, {2, "zsh", true}}},
			{Name: "snipauth", Windows: []Window{{1, "api server", true}}},
		},
	}
	if !reflect.DeepEqual(st, want) {
		t.Errorf("got  %+v\nwant %+v", st, want)
	}
}

func TestParse_WindowNameWithTabsKept(t *testing.T) {
	st, err := Parse("a\t1", "a", "a\t1\t1\tx\ty")
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Sessions[0].Windows[0].Name; got != "x\ty" {
		t.Errorf("name = %q", got)
	}
}

func TestParse_BadCurrent(t *testing.T) {
	if _, err := Parse("nonsense", "", ""); err == nil {
		t.Error("expected error")
	}
}

func TestPaneTarget(t *testing.T) {
	if got := paneTarget("0", -1); got != "=0:" {
		t.Errorf("session target = %q", got)
	}
	if got := paneTarget("0", 2); got != "=0:2" {
		t.Errorf("window target = %q", got)
	}
}

func TestActiveWindow(t *testing.T) {
	s := Session{Windows: []Window{{1, "a", false}, {2, "b", true}}}
	if got := s.ActiveWindow().Name; got != "b" {
		t.Errorf("got %q", got)
	}
}
