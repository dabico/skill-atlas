package htmlreport

import (
	"os"
	"os/exec"
	"runtime"
)

// browserCommand returns the program and arguments that open url; browserEnv is $BROWSER.
func browserCommand(goos, browserEnv, url string) (string, []string) {
	switch {
	case browserEnv != "":
		return browserEnv, []string{url}
	case goos == "darwin":
		return "open", []string{url}
	case goos == "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// OpenBrowser starts the browser command for url without waiting for it. No shell is involved.
func OpenBrowser(url string) error {
	name, args := browserCommand(runtime.GOOS, os.Getenv("BROWSER"), url)
	cmd := exec.Command(name, args...) // stdio stays nil (/dev/null) so the child holds no pipes of ours
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
