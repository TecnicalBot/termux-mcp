package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	vpkg "termux-mcp/internal/version"
)

const defaultRepo = "TecnicalBot/termux-mcp"

type latestRelease struct {
	TagName string `json:"tag_name"`
}

func runUpdate(requestedVersion string) error {
	repo := repoSlug(envOr("TERMUX_MCP_REPO", defaultRepo))
	if repo == "" {
		return fmt.Errorf("invalid TERMUX_MCP_REPO")
	}

	tag := strings.TrimSpace(requestedVersion)
	if tag == "" {
		var err error
		tag, err = fetchLatestVersion(repo)
		if err != nil {
			return err
		}
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	if tag == vpkg.Version {
		fmt.Printf("termux-mcp is already up to date (%s)\n", tag)
		return nil
	}

	arch, err := releaseArch(runtime.GOARCH)
	if err != nil {
		return err
	}
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		return fmt.Errorf("PREFIX is unset; run update inside Termux")
	}
	destination := filepath.Join(prefix, "bin", "termux-mcp")
	temporary := destination + ".new"
	asset := fmt.Sprintf("termux-mcp-%s-android-%s", tag, arch)
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, asset)

	fmt.Printf("Downloading termux-mcp %s (%s)...\n", tag, arch)
	if err := downloadFile(url, temporary); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Chmod(temporary, 0o755); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("make update executable: %w", err)
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("install update: %w", err)
	}

	fmt.Printf("Updated termux-mcp from %s to %s\n", vpkg.Version, tag)
	if _, err := exec.LookPath("sv"); err == nil {
		cmd := exec.Command("sh", "-c", "sleep 1; sv restart termux-mcp")
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "Update installed, but the service could not be restarted. Run: sv restart termux-mcp")
		} else {
			fmt.Println("The termux-mcp service will restart now.")
		}
	} else {
		fmt.Println("Restart the running termux-mcp process to use the update.")
	}
	return nil
}

func fetchLatestVersion(repo string) (string, error) {
	var release latestRelease
	if err := getJSON("https://api.github.com/repos/"+repo+"/releases/latest", &release); err != nil {
		return "", fmt.Errorf("get latest release: %w", err)
	}
	if release.TagName == "" {
		return "", fmt.Errorf("latest release has no tag")
	}
	return release.TagName, nil
}

func getJSON(url string, target any) error {
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func downloadFile(url, destination string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download update: server returned HTTP %d", response.StatusCode)
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create update: %w", err)
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("write update: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close update: %w", closeErr)
	}
	return nil
}

func repoSlug(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, ".git"))
	value = strings.TrimPrefix(value, "git@github.com:")
	value = strings.TrimPrefix(value, "https://github.com/")
	value = strings.TrimPrefix(value, "http://github.com/")
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func releaseArch(goarch string) (string, error) {
	switch goarch {
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("no prebuilt release for %s; update with: TERMUX_MCP_SOURCE=yes bash scripts/install.sh", goarch)
	}
}
