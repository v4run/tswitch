// Package tmux reads sessions and windows from the tmux server and runs the
// few commands the switcher needs (capture, kill, rename, switch).
package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Window is one window of a session.
type Window struct {
	Index  int
	Name   string
	Active bool // the session's active window
}

// Session is a tmux session and its windows in index order.
type Session struct {
	Name    string
	Windows []Window
}

// ActiveWindow returns the session's active window, or the zero Window if
// the session has none.
func (s Session) ActiveWindow() Window {
	for _, w := range s.Windows {
		if w.Active {
			return w
		}
	}
	return Window{}
}

// State is a snapshot of the server. Sessions keep tmux's order.
type State struct {
	Sessions       []Session
	CurrentSession string
	CurrentWindow  int
}

// Client is the subset of tmux the switcher talks to. Every method takes a
// session name plus a window index; an index < 0 means the session itself.
type Client interface {
	Load() (State, error)
	Capture(session string, index int) (string, error)
	Kill(session string, index int) error
	Rename(session string, index int, name string) error
	Switch(session string, index int) error
}

// Exec is the Client backed by the tmux binary.
type Exec struct{}

// Load queries the current client position, sessions and windows.
func (e Exec) Load() (State, error) {
	cur, err := e.run("display-message", "-p", "#{session_name}\t#{window_index}")
	if err != nil {
		return State{}, err
	}
	sessions, err := e.run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return State{}, err
	}
	windows, err := e.run("list-windows", "-a", "-F", "#{session_name}\t#{window_index}\t#{window_active}\t#{window_name}")
	if err != nil {
		return State{}, err
	}
	return Parse(cur, sessions, windows)
}

// Capture returns the active pane of the target with ANSI escapes kept.
func (e Exec) Capture(session string, index int) (string, error) {
	return e.run("capture-pane", "-e", "-p", "-t", paneTarget(session, index))
}

// Kill kills a window, or the whole session when index < 0.
func (e Exec) Kill(session string, index int) error {
	if index < 0 {
		_, err := e.run("kill-session", "-t", "="+session)
		return err
	}
	_, err := e.run("kill-window", "-t", paneTarget(session, index))
	return err
}

// Rename renames a window, or the session when index < 0.
func (e Exec) Rename(session string, index int, name string) error {
	if index < 0 {
		_, err := e.run("rename-session", "-t", "="+session, "--", name)
		return err
	}
	_, err := e.run("rename-window", "-t", paneTarget(session, index), "--", name)
	return err
}

// Switch moves the calling client to the target.
func (e Exec) Switch(session string, index int) error {
	_, err := e.run("switch-client", "-t", paneTarget(session, index))
	return err
}

// paneTarget builds an exact-match target: "=s:" (the session's current
// window) or "=s:idx". The "=" stops tmux from prefix-matching names, so a
// session called "0" is never mistaken for a window or pane index.
func paneTarget(session string, index int) string {
	if index < 0 {
		return "=" + session + ":"
	}
	return "=" + session + ":" + strconv.Itoa(index)
}

func (Exec) run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return string(out), nil
}

// Parse builds a State from the raw output of the three queries in Load:
// "session\twindow" for the current position, one session name per line,
// and "session\tindex\tactive\tname" per window.
func Parse(current, sessions, windows string) (State, error) {
	var st State
	curSession, curWindow, ok := strings.Cut(strings.TrimSpace(current), "\t")
	if !ok {
		return st, fmt.Errorf("unexpected current position %q", current)
	}
	idx, err := strconv.Atoi(curWindow)
	if err != nil {
		return st, fmt.Errorf("unexpected window index %q", curWindow)
	}
	st.CurrentSession, st.CurrentWindow = curSession, idx

	pos := map[string]int{}
	for _, name := range lines(sessions) {
		pos[name] = len(st.Sessions)
		st.Sessions = append(st.Sessions, Session{Name: name})
	}
	for _, line := range lines(windows) {
		f := strings.SplitN(line, "\t", 4)
		if len(f) != 4 {
			return st, fmt.Errorf("unexpected window line %q", line)
		}
		i, ok := pos[f[0]]
		if !ok {
			continue // session appeared between the two queries
		}
		n, err := strconv.Atoi(f[1])
		if err != nil {
			return st, fmt.Errorf("unexpected window index %q", f[1])
		}
		w := Window{Index: n, Active: f[2] == "1", Name: f[3]}
		st.Sessions[i].Windows = append(st.Sessions[i].Windows, w)
	}
	return st, nil
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
