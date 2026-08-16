// Package tunnel starts public HTTPS tunnels (cloudflared quick tunnel by
// default) in front of the local HTTP endpoint.
package tunnel

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Start launches the requested provider pointed at the local target
// (host:port) and blocks until ctx is canceled.
func Start(ctx context.Context, provider, target string) error {
	switch provider {
	case "cloudflared":
		return cloudflaredQuick(ctx, target)
	default:
		return fmt.Errorf("unsupported tunnel provider %q (supported: cloudflared)", provider)
	}
}

func cloudflaredQuick(ctx context.Context, target string) error {
	if _, err := exec.LookPath("cloudflared"); err != nil {
		return errors.New("cloudflared not found; install it with: pkg install cloudflared")
	}
	cmd := exec.CommandContext(ctx, "cloudflared", "tunnel", "--url", "http://"+target, "--no-autoupdate")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cloudflared: %w", err)
	}

	go scanLines(stdout, "cloudflared", printURL)
	go scanLines(stderr, "cloudflared", printURL)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil // expected shutdown
		}
		return fmt.Errorf("cloudflared exited: %w", err)
	}
	return nil
}

func scanLines(r interface{ Read([]byte) (int, error) }, tag string, onLine func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		onLine(sc.Text())
	}
}

// printURL prints the public tunnel URL when cloudflared announces it.
func printURL(line string) {
	if strings.Contains(line, "trycloudflare.com") {
		fmt.Println("PUBLIC URL: " + strings.TrimSpace(line))
	}
}
