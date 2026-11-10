// Package browser opens the local admin UI with the user's default browser.
package browser

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const readinessTimeout = 5 * time.Second

type commandSpec struct {
	name string
	args []string
}

// AdminURL returns the loopback admin URL, including an optional base path.
func AdminURL(port int, basePath string) string {
	root := strings.TrimSuffix(basePath, "/")
	return fmt.Sprintf("http://127.0.0.1:%d%s/", port, root)
}

// WaitUntilReady polls healthz and invokes open only after the local server is
// ready. This prevents a desktop launcher from opening an error page during startup.
func WaitUntilReady(baseURL string, open func(url string) error, interval time.Duration) error {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	probeURL := strings.TrimSuffix(baseURL, "/") + "/healthz"
	deadline := time.Now().Add(readinessTimeout)
	for {
		resp, err := client.Get(probeURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return open(baseURL)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("admin UI not ready at %s", probeURL)
		}
		time.Sleep(interval)
	}
}

// Open opens url with the platform default browser.
func Open(url string) error {
	spec := openCommand(runtime.GOOS, url)
	if spec.name == "" {
		return fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}
	cmd := exec.Command(spec.name, spec.args...)
	return cmd.Run()
}

func openCommand(goos, url string) commandSpec {
	switch goos {
	case "windows":
		return commandSpec{name: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", url}}
	case "darwin":
		return commandSpec{name: "open", args: []string{url}}
	case "linux":
		return commandSpec{name: "xdg-open", args: []string{url}}
	default:
		return commandSpec{}
	}
}
