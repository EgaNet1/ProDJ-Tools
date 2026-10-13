//go:build !windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// OpenWindow opens url in the default browser.
func OpenWindow(url, profile string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, url).Start()
}

// PickFolder is only available on Windows; elsewhere the path is typed in.
func PickFolder(start string) (string, error) {
	return "", errors.New("выбор папки через диалог есть только в Windows — вставьте путь вручную")
}

// Alert prints the message.
func Alert(title, text string) { fmt.Fprintf(os.Stderr, "%s: %s\n", title, text) }
