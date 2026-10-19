package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/v4run/tswitch/internal/tmux"
	"github.com/v4run/tswitch/internal/tui"
)

// fallbackVersion is reported only when build info is unavailable.
// `go install` users get the module version from runtime/debug.
const fallbackVersion = "v0.1.0"

func versionString() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return fallbackVersion
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, errVersion) || errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "tswitch:", err)
		os.Exit(2)
	}
}

var errVersion = errors.New("--version")

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tswitch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sessions := fs.Bool("sessions", false, "list sessions only, without their windows")
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, "tswitch", versionString())
		return errVersion
	}
	if os.Getenv("TMUX") == "" {
		return errors.New("not running inside tmux")
	}

	client := tmux.Exec{}
	m, err := tui.New(client, *sessions)
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	if session, index, ok := final.(tui.Model).Choice(); ok {
		return client.Switch(session, index)
	}
	return nil
}
