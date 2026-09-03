// Command termux-mcp is the MCP server for Termux devices.
//
// Usage:
//
//	termux-mcp                       # serve stdio (default)
//	termux-mcp serve stdio           # local stdio mode
//	termux-mcp serve http            # Streamable HTTP mode (LAN)
//	termux-mcp token new [--write]   # generate a bearer token
//	termux-mcp token rotate          # rotate the token in config
//	termux-mcp tunnel start          # cloudflared quick tunnel
//	termux-mcp doctor                # environment diagnostics
//	termux-mcp version
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"

	"termux-mcp/internal/audit"
	"termux-mcp/internal/config"
	"termux-mcp/internal/exec"
	"termux-mcp/internal/registry"
	"termux-mcp/internal/server"
	"termux-mcp/internal/tasks"
	"termux-mcp/internal/tools"
	"termux-mcp/internal/tunnel"
	vpkg "termux-mcp/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	if len(os.Args) < 2 {
		runServe("stdio", "")
		return
	}
	switch os.Args[1] {
	case "serve":
		mode := "stdio"
		rest := os.Args[2:]
		if len(rest) > 0 && (rest[0] == "stdio" || rest[0] == "http") {
			mode = rest[0]
			rest = rest[1:]
		}
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		cfgPath := fs.String("config", envOr("TERMUX_MCP_CONFIG", ""), "path to config.yaml")
		_ = fs.Parse(rest)
		runServe(mode, *cfgPath)

	case "version":
		fmt.Printf("%s %s\n", vpkg.Name, vpkg.Version)

	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ExitOnError)
		cfgPath := fs.String("config", envOr("TERMUX_MCP_CONFIG", ""), "path to config.yaml")
		_ = fs.Parse(os.Args[2:])
		runDoctor(*cfgPath)

	case "token":
		sub, write, cfgPath := parseTokenArgs(os.Args[2:])
		switch sub {
		case "new":
			runToken(write, cfgPath)
		case "rotate":
			runToken(true, cfgPath)
		default:
			usage()
		}

	case "tunnel":
		rest := os.Args[2:]
		if len(rest) > 0 && rest[0] == "start" {
			rest = rest[1:] // strip subcommand so flags parse (flag stops at first non-flag)
		} else {
			usage()
		}
		fs := flag.NewFlagSet("tunnel", flag.ExitOnError)
		provider := fs.String("provider", "cloudflared", "cloudflared")
		cfgPath := fs.String("config", envOr("TERMUX_MCP_CONFIG", ""), "path to config.yaml")
		_ = fs.Parse(rest)
		runTunnel(*provider, *cfgPath)

	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `termux-mcp - MCP server for Termux

Usage:
  termux-mcp serve stdio [--config PATH]   local stdio mode (default)
  termux-mcp serve http  [--config PATH]   Streamable HTTP mode
  termux-mcp token new [--write]           generate a bearer token
  termux-mcp token rotate                  rotate token in config
  termux-mcp tunnel start [--provider cloudflared]
  termux-mcp doctor                        environment diagnostics
  termux-mcp version`)
	os.Exit(2)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseTokenArgs parses `token` subcommand arguments. Unlike flag.FlagSet,
// it accepts flags on either side of the subcommand, so both
// `token new --write --config X` and `token --write new --config X` work.
func parseTokenArgs(args []string) (sub string, write bool, cfgPath string) {
	sub = "new"
	cfgPath = envOr("TERMUX_MCP_CONFIG", "")
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--write":
			write = true
		case a == "--config" || a == "-c":
			if i+1 < len(args) {
				cfgPath = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--config="):
			cfgPath = strings.TrimPrefix(a, "--config=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", a)
			usage()
		default:
			sub = a
		}
	}
	return sub, write, cfgPath
}

// buildServer loads config, opens the audit log, registers all tools and
// builds the MCP server. It also takes a wake lock (best effort).
func buildServer(mode, cfgPath string) (*serverContext, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	// CLI mode wins over config, unless env set it.
	if v := os.Getenv("TERMUX_MCP_SERVER_MODE"); v == "stdio" || v == "http" {
		mode = v
	}
	cfg.Server.Mode = mode

	var al *audit.Logger
	if cfg.Audit.Enabled {
		path := filepath.Join(cfg.Audit.Dir, "audit.jsonl")
		al, err = audit.New(path)
		if err != nil {
			return nil, err
		}
		log.Printf("audit log: %s", path)
	} else {
		al = audit.Disabled()
	}

	mgr := tasks.New(filepath.Join(cfg.Audit.Dir, "tasks"), 8, cfg.Exec.MaxTaskLogBytes)

	reg := registry.New()
	if err := tools.RegisterAll(reg, cfg, al, mgr); err != nil {
		return nil, err
	}
	enabled := len(reg.Names(&cfg.Tools))
	log.Printf("%s %s starting (%s mode, %d tools enabled of %d registered)",
		vpkg.Name, vpkg.Version, mode, enabled, len(reg.All()))

	// Keep the device awake while serving (best effort, ignore failure).
	_ = takeWakeLock()

	return &serverContext{cfg: cfg, al: al, reg: reg, tasks: mgr}, nil
}

type serverContext struct {
	cfg   *config.Config
	al    *audit.Logger
	reg   *registry.Registry
	tasks *tasks.Manager
}

func runServe(mode, cfgPath string) {
	sc, err := buildServer(mode, cfgPath)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	defer sc.al.Close()

	srv := server.BuildMCP(sc.reg, sc.cfg, sc.al)
	switch sc.cfg.Server.Mode {
	case "stdio":
		if err := server.RunStdio(srv); err != nil {
			log.Fatalf("stdio: %v", err)
		}
	case "http":
		if err := server.RunHTTP(srv, sc.cfg, sc.tasks); err != nil {
			log.Fatalf("http: %v", err)
		}
	}
}

func runToken(write bool, cfgPath string) {
	tok := newToken()
	if write {
		if err := persistToken(cfgPath, tok); err != nil {
			log.Fatalf("persist token: %v", err)
		}
		fmt.Println("token written to config (chmod 600 recommended):")
	} else {
		fmt.Println("new token (set TERMUX_MCP_AUTH_TOKEN or write it into config.yaml):")
	}
	fmt.Println(tok)
}

func runDoctor(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("config: %v", err)
	} else {
		san := cfg.Sanitized()
		out, _ := yaml.Marshal(san)
		fmt.Println("=== config ===")
		fmt.Println(string(out))
	}

	fmt.Println("=== commands ===")
	for _, c := range []string{
		"termux-battery-status", "termux-brightness", "termux-volume",
		"termux-clipboard-get", "termux-clipboard-set", "termux-sms-list",
		"termux-sms-send", "termux-contact-list", "termux-call-log",
		"termux-telephony-call", "termux-notification", "termux-notification-list",
		"termux-toast", "termux-camera-info", "termux-camera-photo",
		"termux-microphone-record", "termux-media-player", "termux-tts-speak",
		"termux-speech-to-text", "termux-download", "termux-share",
		"termux-media-scan", "termux-sensor", "termux-wifi-connectioninfo",
		"termux-wifi-scaninfo", "termux-telephony-deviceinfo", "termux-telephony-cellinfo",
		"termux-vibrate", "termux-flashlight", "termux-wake-lock",
		"termux-open-url", "input", "screencap", "uiautomator", "cloudflared",
	} {
		status := "missing"
		if exec.CommandExists(c) {
			status = "ok"
		}
		fmt.Printf("  %-30s %s\n", c, status)
	}

	fmt.Println("=== storage ===")
	if fi, err := os.Stat("/sdcard"); err == nil && fi.IsDir() {
		fmt.Println("  /sdcard accessible: yes")
	} else {
		fmt.Println("  /sdcard accessible: no (run: termux-setup-storage)")
	}
}

func runTunnel(provider, cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	// A tunnel exposes the device to the public internet, and tunnel traffic
	// arrives from the local cloudflared proxy (127.0.0.1), which the auth
	// middleware treats as loopback. Loopback is only exempt when
	// auth.require is false — so a tunnel MUST have the token enforced.
	if cfg.Auth.Token == "" || !cfg.Auth.Require {
		log.Fatalf("refusing to start a tunnel without enforced auth: " +
			"set auth.require: true and auth.token in the config " +
			"(generate a token with: termux-mcp token new --write)")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("starting %s tunnel -> http://%s\n", provider, cfg.HTTPAddr())
	if err := tunnel.Start(ctx, provider, cfg.HTTPAddr()); err != nil {
		log.Fatalf("tunnel: %v", err)
	}
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}

// persistToken sets auth.token in the YAML config file, preserving the rest.
func persistToken(cfgPath string, token string) error {
	if cfgPath == "" {
		return fmt.Errorf("no config file given (use --config or TERMUX_MCP_CONFIG)")
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return err
	}
	authMap, _ := m["auth"].(map[string]any)
	if authMap == nil {
		authMap = map[string]any{}
	}
	authMap["token"] = token
	m["auth"] = authMap
	out, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o600)
}

func takeWakeLock() error {
	res, err := exec.Run(context.Background(), "termux-wake-lock", nil, exec.Config{Timeout: 5e9})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("termux-wake-lock exit %d", res.ExitCode)
	}
	return nil
}
