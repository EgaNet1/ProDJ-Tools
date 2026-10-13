//go:build !windows

package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"prodjtools/internal/i18n"
)

// OpenWindow opens url in the default browser.
func OpenWindow(url, profile string) error { return OpenURL(url) }

// OpenURL opens url in the default browser.
func OpenURL(url string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, url).Start()
}

// PickFolder is only available on Windows; elsewhere the path is typed in.
func PickFolder(start string) (string, error) {
	return "", i18n.Error("picker.windows_only")
}

// Alert prints the message.
func Alert(title, text string) { fmt.Fprintf(os.Stderr, "%s: %s\n", title, text) }
