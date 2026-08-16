// Package device implements Module A: device & system tools (safe tier).
package device

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/kit"
)

// All returns the device module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("get_battery_status",
			mcp.WithDescription("Get battery status (percentage, temperature, health, charging state) via termux-battery-status.")),
			Handler: jsonCmd(k, "termux-battery-status", nil, true),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_device_info",
			mcp.WithDescription("Get device info (model, manufacturer, SIM/network data) via termux-telephony-deviceinfo.")),
			Handler: deviceInfo(k),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_storage_info",
			mcp.WithDescription("Get storage usage per mount point (df -h).")),
			Handler: rawCmd(k, "df", []string{"-h"}),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_screen_size",
			mcp.WithDescription("Get the physical screen size (wm size). Requires android-tools or adb shell access.")),
			Handler: rawCmd(k, "wm", []string{"size"}),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_screen_brightness",
			mcp.WithDescription("Get current screen brightness (0-255) via termux-brightness.")),
			Handler: rawCmd(k, "termux-brightness", nil),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("set_screen_brightness",
			mcp.WithDescription("Set screen brightness, value 0-255, via termux-brightness."),
			mcp.WithNumber("value", mcp.Required(), mcp.Description("Brightness 0-255"), mcp.Min(0), mcp.Max(255))),
			Handler: setBrightness(k),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_volume",
			mcp.WithDescription("Get volume levels for all audio streams via termux-volume.")),
			Handler: rawCmd(k, "termux-volume", nil),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("set_volume",
			mcp.WithDescription("Set volume for an audio stream, 0-100, via termux-volume."),
			mcp.WithString("stream", mcp.Required(), mcp.Description("Stream name, e.g. music, ring, notification, system, alarm")),
			mcp.WithNumber("volume", mcp.Required(), mcp.Description("Volume 0-100"), mcp.Min(0), mcp.Max(100))),
			Handler: setVolume(k),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("vibrate",
			mcp.WithDescription("Vibrate the device via termux-vibrate."),
			mcp.WithNumber("duration", mcp.Description("Duration in ms (default 500)"), mcp.Min(0), mcp.Max(10000)),
			mcp.WithBoolean("force", mcp.Description("Force vibration even if disabled (requires root on some devices)"))),
			Handler: vibrate(k),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("flashlight",
			mcp.WithDescription("Toggle the camera flashlight via termux-flashlight."),
			mcp.WithBoolean("on", mcp.Required(), mcp.Description("true = on, false = off"))),
			Handler: flashlight(k),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_wifi_info",
			mcp.WithDescription("Get current Wi-Fi connection info via termux-wifi-connectioninfo.")),
			Handler: jsonCmd(k, "termux-wifi-connectioninfo", nil, true),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("scan_wifi",
			mcp.WithDescription("Scan nearby Wi-Fi networks via termux-wifi-scaninfo.")),
			Handler: jsonCmd(k, "termux-wifi-scaninfo", nil, true),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_telephony_info",
			mcp.WithDescription("Get cellular/telephony info (network, signal, cell towers) via termux-telephony-cellinfo.")),
			Handler: jsonCmd(k, "termux-telephony-cellinfo", nil, true),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("get_sensor_list",
			mcp.WithDescription("List available sensors via termux-sensor -l.")),
			Handler: jsonCmd(k, "termux-sensor", []string{"-l"}, true),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},

		{Def: mcp.NewTool("read_sensor",
			mcp.WithDescription("Read values from a sensor for a duration via termux-sensor."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Sensor name from get_sensor_list")),
			mcp.WithNumber("duration", mcp.Description("Seconds to sample, 1-10 (default 1)"), mcp.Min(1), mcp.Max(10))),
			Handler: readSensor(k),
			Meta:    registry.Meta{Module: "device", Tier: registry.TierSafe}},
	}
}

// jsonCmd runs a termux-api command and returns its parsed JSON.
func jsonCmd(k *kit.Kit, name string, args []string, retry bool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := k.RunJSON(ctx, name, args, 0, retry)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return kit.ResultError("%v", err), nil
		}
		return kit.ResultJSON(v), nil
	}
}

// rawCmd runs a command and returns its stdout verbatim.
func rawCmd(k *kit.Kit, name string, args []string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := k.Run(ctx, name, args, 0)
		if err != nil {
			return kit.ResultError("%v (install android-tools if this is an adb/wm command)", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("%s exited %d: %s", name, res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("%s", strings.TrimRight(res.Stdout, "\n")), nil
	}
}

func deviceInfo(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := k.RunJSON(ctx, "termux-telephony-deviceinfo", nil, 0, true)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res, err := k.Run(ctx, "getprop", []string{"ro.build.version.release"}, 5); err == nil && res.ExitCode == 0 {
			if v := strings.TrimSpace(res.Stdout); v != "" {
				m["android_release"] = v
			}
		}
		return kit.ResultJSON(m), nil
	}
}

func setBrightness(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		v := int(math.Round(kit.NumArg(req, "value")))
		res, err := k.Run(ctx, "termux-brightness", []string{itoa(v)}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-brightness exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("brightness set to %d", v), nil
	}
}

func setVolume(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stream, err := kit.RequireStr(req, "stream")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		v := int(math.Round(kit.NumArg(req, "volume")))
		res, err := k.Run(ctx, "termux-volume", []string{stream, itoa(v)}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-volume exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("volume %s set to %d", stream, v), nil
	}
}

func vibrate(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ms := int(math.Round(kit.NumArg(req, "duration")))
		if ms <= 0 {
			ms = 500
		}
		args := []string{"-d", itoa(ms)}
		if kit.BoolArg(req, "force") {
			args = append(args, "-f")
		}
		res, err := k.Run(ctx, "termux-vibrate", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-vibrate exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("vibrated for %d ms", ms), nil
	}
}

func flashlight(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		state := "off"
		if kit.BoolArg(req, "on") {
			state = "on"
		}
		res, err := k.Run(ctx, "termux-flashlight", []string{state}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-flashlight exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("flashlight %s", state), nil
	}
}

func readSensor(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := kit.RequireStr(req, "name")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		d := int(math.Round(kit.NumArg(req, "duration")))
		if d < 1 {
			d = 1
		}
		if d > 10 {
			d = 10
		}
		raw, err := k.RunJSON(ctx, "termux-sensor", []string{"-s", name, "-d", itoa(d)}, 0, true)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return kit.ResultError("%v", err), nil
		}
		return kit.ResultJSON(v), nil
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
