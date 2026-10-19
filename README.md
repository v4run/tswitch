# tswitch

A quiet tmux session/window switcher: a tree of sessions and their windows, vim keys, inline search, kill/rename prompts and a live pane preview. Built to run in a tmux popup.

```
   hangar                         4 │ hangar › tmux
       1  claude                    │
       2  editor                    │  (live capture of the pane)
 ▎     3  tmux                    ● │
       4  logs                      │
                                    │
   snipauth                       1 │
       1  api                       │

 / search · enter switch · x kill · r rename · ? help
```

## Install

```
go install github.com/v4run/tswitch/cmd/tswitch@latest
```

## tmux

```tmux
bind w display-popup -E -b rounded -w 90% -h 70% "tswitch"
bind s display-popup -E -b rounded -w 70% -h 50% "tswitch --sessions"
```

## Keys

| Key             | Action                                   |
|-----------------|------------------------------------------|
| j / k           | Move (skips the blank lines)             |
| g / G           | Top / bottom                             |
| ] / [           | Next / previous session                  |
| C-d / C-u       | Half page                                |
| h / l           | Fold / unfold the session                |
| /               | Search; esc returns keeping the filter   |
| c               | Clear search                             |
| enter           | Switch                                   |
| x               | Kill (y to confirm)                      |
| r               | Rename inline                            |
| ?               | Help                                     |
| q / esc         | Quit                                     |

Search terms are space separated and fuzzy. A term may match the session or the window, so `snip zsh` finds the `zsh` window of `snipauth`. A query that matches a session name keeps all of its windows.

## Flags

| Flag         | Notes                              |
|--------------|------------------------------------|
| `--sessions` | sessions only, without windows     |
| `--version`  | print version                      |

Colours are ANSI (green accent, blue matches, bright-black muted), so they follow the terminal theme.
