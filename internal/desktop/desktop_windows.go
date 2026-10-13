// Package desktop connects the program to the Windows desktop: it opens the
// interface window, shows the folder picker and reports fatal errors.
package desktop

import (
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// OpenWindow shows url in a plain browser window without tabs or address
// bar ("app mode") using Edge or Chrome, or in the default browser if neither
// is installed. profile is a folder for the window's own browser profile,
// which keeps it apart from the user's everyday browsing.
func OpenWindow(url, profile string) error {
	for _, exe := range appBrowsers() {
		if _, err := os.Stat(exe); err != nil {
			continue
		}
		cmd := exec.Command(exe,
			"--app="+url,
			"--user-data-dir="+profile,
			"--no-first-run",
			"--no-default-browser-check",
			"--window-size=1440,900",
		)
		if err := cmd.Start(); err == nil {
			return cmd.Process.Release()
		}
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func appBrowsers() []string {
	var list []string
	for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LocalAppData")} {
		if root != "" {
			list = append(list,
				filepath.Join(root, `Microsoft\Edge\Application\msedge.exe`),
				filepath.Join(root, `Google\Chrome\Application\chrome.exe`))
		}
	}
	return list
}

var (
	//go:embed picker.ps1
	pickerScript string
	//go:embed picker.cs
	pickerSource string
)

// PickFolder shows the standard Windows folder dialog, starting in start.
// It returns "" if the user cancels.
func PickFolder(start string) (string, error) {
	// -EncodedCommand (base64 of UTF-16LE) spares the script from
	// command-line quoting rules.
	script := utf16.Encode([]rune(pickerScript))
	encoded := make([]byte, 2*len(script))
	for i, u := range script {
		binary.LittleEndian.PutUint16(encoded[2*i:], u)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-STA",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	cmd.Env = append(os.Environ(),
		"PRODJ_PICKER_SOURCE="+pickerSource,
		"PRODJ_PICKER_TITLE=Папка с треками для сета",
		"PRODJ_PICKER_START="+start,
	)
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Alert shows an error message box. It is used before the window exists.
func Alert(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	const iconError = 0x10
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), iconError)
}
