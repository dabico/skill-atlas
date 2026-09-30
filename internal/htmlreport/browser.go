package htmlreport

import (
	"os"
	"os/exec"
	"runtime"
	"time"
)

// launchWait is how long OpenBrowser watches the command before treating it as launched.
const launchWait = 3 * time.Second

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

// OpenBrowser starts the browser command for url. No shell is involved.
func OpenBrowser(url string) error {
	name, args := browserCommand(runtime.GOOS, os.Getenv("BROWSER"), url)
	return launch(name, args, launchWait)
}

// launch starts the command and returns nil if it exits 0 or is still running after wait.
func launch(name string, args []string, wait time.Duration) error {
	cmd := exec.Command(name, args...) // stdio stays nil (/dev/null) so the child holds no pipes of ours
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return nil
	}
}
