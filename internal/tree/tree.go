// Package tree flattens a tmux.State into the rows the switcher draws:
// session headers followed by their windows, filtered by an optional search query.
package tree

import (
	"strconv"
	"strings"

	"github.com/v4run/tswitch/internal/tmux"
)

// Kind is what a row represents.
type Kind int

const (
	Session Kind = iota
	Window
)

// Row is one line of the list.
type Row struct {
	Kind    Kind
	Session string
	Index   int    // window index; -1 for session rows
	Name    string // session name for headers, window name for windows
	Count   int    // number of windows, session rows only
	Current bool   // the client's current window (or session, for headers)
	Folded  bool   // session rows only: windows hidden
	Match   []int  // rune indices of Name matched by the query
	Header  int    // row index of the owning session header
}

// Key identifies a row across rebuilds.
type Key struct {
	Kind    Kind
	Session string
	Index   int
}

// Key returns the row's identity.
func (r Row) Key() Key { return Key{r.Kind, r.Session, r.Index} }

// Label is the human target, e.g. "hangar" or "hangar:2".
func (r Row) Label() string {
	if r.Kind == Window {
		return r.Session + ":" + strconv.Itoa(r.Index)
	}
	return r.Session
}

// Options control what Build includes.
type Options struct {
	Query        string
	Folded       map[string]bool
	SessionsOnly bool
}

// Build returns the rows for st. With a query, a session whose name matches
// every term keeps all its windows; otherwise a window is kept when each
// term matches either its own name or its session's name, and its session
// header is kept with it. Folding is ignored while a query is set so
// matches inside folded sessions still show.
func Build(st tmux.State, opts Options) []Row {
	terms := strings.Fields(opts.Query)
	var rows []Row
	for _, s := range st.Sessions {
		sOK, sIdx := matchEach(terms, s.Name)
		sessHit := all(sOK)

		var wins []Row
		if !opts.SessionsOnly {
			for _, w := range s.Windows {
				r := Row{
					Kind:    Window,
					Session: s.Name,
					Index:   w.Index,
					Name:    w.Name,
					Current: s.Name == st.CurrentSession && w.Index == st.CurrentWindow,
				}
				if !sessHit {
					wOK, wIdx := matchEach(terms, w.Name)
					if !covered(wOK, sOK) {
						continue
					}
					r.Match = wIdx
				}
				wins = append(wins, r)
			}
		}
		if !sessHit && len(wins) == 0 {
			continue
		}

		header := len(rows)
		folded := len(terms) == 0 && !opts.SessionsOnly && opts.Folded[s.Name]
		rows = append(rows, Row{
			Kind:    Session,
			Session: s.Name,
			Index:   -1,
			Name:    s.Name,
			Count:   len(s.Windows),
			Current: s.Name == st.CurrentSession,
			Folded:  folded,
			Match:   sIdx,
			Header:  header,
		})
		if folded {
			continue
		}
		for _, w := range wins {
			w.Header = header
			rows = append(rows, w)
		}
	}
	return rows
}

// matchEach matches every term against s, returning per-term success and the
// union of matched rune indices.
func matchEach(terms []string, s string) ([]bool, []int) {
	ok := make([]bool, len(terms))
	var idx []int
	seen := map[int]bool{}
	for i, t := range terms {
		m, hit := Match(t, s)
		ok[i] = hit
		for _, j := range m {
			if !seen[j] {
				seen[j] = true
				idx = append(idx, j)
			}
		}
	}
	return ok, idx
}

func all(ok []bool) bool {
	for _, b := range ok {
		if !b {
			return false
		}
	}
	return true
}

// covered reports whether every term matched in a or b.
func covered(a, b []bool) bool {
	for i := range a {
		if !a[i] && !b[i] {
			return false
		}
	}
	return true
}

// Match reports whether pattern matches s case-insensitively and which rune
// indices of s it matched. A contiguous substring wins; otherwise the
// pattern's runes must appear in order (fuzzy subsequence).
func Match(pattern, s string) ([]int, bool) {
	p := []rune(strings.ToLower(pattern))
	r := []rune(strings.ToLower(s))
	if len(p) == 0 {
		return nil, true
	}
	if i := indexRunes(r, p); i >= 0 {
		idx := make([]int, len(p))
		for j := range p {
			idx[j] = i + j
		}
		return idx, true
	}
	var idx []int
	j := 0
	for i, c := range r {
		if j < len(p) && c == p[j] {
			idx = append(idx, i)
			j++
		}
	}
	if j < len(p) {
		return nil, false
	}
	return idx, true
}

func indexRunes(s, sub []rune) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := range sub {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
