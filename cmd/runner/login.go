//go:build darwin || linux

package main

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func terminalHyperlinks(out io.Writer) bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	return err == nil
}

// The enrollment client verifies the URL before invoking this formatter.
func printSignInURL(out io.Writer, url string, hyperlink bool) error {
	if hyperlink {
		_, err := fmt.Fprintf(out, "Open GitHub sign-in (Cmd+click):\n\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\\n", url, url)
		return err
	}
	_, err := fmt.Fprintf(out, "Open GitHub sign-in:\n%s\n", url)
	return err
}
