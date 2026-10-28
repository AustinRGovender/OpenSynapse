// Package browser launches the headless Chromium used by the Rod crawler
// and PDF export. It only ever uses a browser already installed on the
// machine: Rod's default behaviour of downloading Chromium when none is found
// would be an unrequested network call (see CLAUDE.md: no phone-home).
package browser

import (
	"errors"
	"os"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// ErrNotFound means no local Chrome/Chromium could be found.
var ErrNotFound = errors.New("no Chrome or Chromium found: install one or set ROD_BROWSER_BIN to its path")

// Path returns the browser binary to use.
func Path() (string, error) {
	if p := os.Getenv("ROD_BROWSER_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, ok := launcher.LookPath(); ok {
		return p, nil
	}
	return "", ErrNotFound
}

// Launch starts a headless browser and returns it with a cleanup function.
func Launch() (*rod.Browser, func(), error) {
	bin, err := Path()
	if err != nil {
		return nil, nil, err
	}
	l := launcher.New().Bin(bin).Headless(true).Leakless(false)
	if os.Getuid() == 0 {
		// Chromium refuses to run as root with its sandbox (Docker images).
		l = l.NoSandbox(true)
	}
	u, err := l.Launch()
	if err != nil {
		return nil, nil, err
	}
	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		l.Kill()
		return nil, nil, err
	}
	return b, func() {
		b.Close()
		l.Kill()
		l.Cleanup()
	}, nil
}
