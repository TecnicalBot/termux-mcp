// Package media implements Module C: camera, audio, media player and
// text-to-speech tools (sensitive tier, opt-in).
package media

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"termux-mcp/internal/registry"
	"termux-mcp/internal/tools/files"
	"termux-mcp/internal/tools/kit"
)

// All returns the media module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("take_photo",
			mcp.WithDescription("Take a photo with the camera via termux-camera-photo."),
			mcp.WithString("output", mcp.Required(), mcp.Description("Output file path (must be inside a configured file root)")),
			mcp.WithNumber("camera", mcp.Description("Camera id, 0 = back, 1 = front (default 0)"), mcp.Min(0))),
			Handler: takePhoto(k),
			Meta:    registry.Meta{Module: "media", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("get_camera_info",
			mcp.WithDescription("List cameras and their capabilities via termux-camera-info.")),
			Handler: jsonCmd(k, "termux-camera-info", nil),
			Meta:    registry.Meta{Module: "media", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("record_audio",
			mcp.WithDescription("Record microphone audio via termux-microphone-record."),
			mcp.WithString("action", mcp.Description("start (default) or stop"), mcp.Enum("start", "stop")),
			mcp.WithString("output", mcp.Description("Output file path for start (must be inside a configured file root)")),
			mcp.WithNumber("limit", mcp.Description("Max recording time in seconds (stop automatically)"), mcp.Min(1))),
			Handler: recordAudio(k),
			Meta:    registry.Meta{Module: "media", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("media_player_control",
			mcp.WithDescription("Control the media player via termux-media-player."),
			mcp.WithString("action", mcp.Required(), mcp.Description("info, play, pause, stop, previous, next"),
				mcp.Enum("info", "play", "pause", "stop", "previous", "next")),
			mcp.WithString("file", mcp.Description("File to play (required for action=play)"))),
			Handler: mediaPlayer(k),
			Meta:    registry.Meta{Module: "media", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("text_to_speech",
			mcp.WithDescription("Speak text aloud via termux-tts-speak."),
			mcp.WithString("text", mcp.Required(), mcp.Description("Text to speak")),
			mcp.WithNumber("rate", mcp.Description("Speech rate, e.g. 1.0 (default)")),
			mcp.WithNumber("pitch", mcp.Description("Speech pitch, e.g. 1.0 (default)"))),
			Handler: textToSpeech(k),
			Meta:    registry.Meta{Module: "media", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("speech_to_text",
			mcp.WithDescription("Transcribe speech from the microphone via termux-speech-to-text."),
			mcp.WithString("lang", mcp.Description("Language code, e.g. en-US")),
			mcp.WithNumber("duration", mcp.Description("Max listening time in seconds, 1-60 (default 5)"), mcp.Min(1), mcp.Max(60))),
			Handler: speechToText(k),
			Meta:    registry.Meta{Module: "media", Tier: registry.TierSensitive}},
	}
}

func takePhoto(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		out, err := kit.RequireStr(req, "output")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		p, err := files.Resolve(out, files.Roots(&k.Cfg.Tools))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		args := []string{}
		if c := int(math.Round(kit.NumArg(req, "camera"))); c > 0 {
			args = append(args, "-c", itoa(c))
		}
		args = append(args, p)
		res, err := k.Run(ctx, "termux-camera-photo", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-camera-photo exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("photo saved to %s", p), nil
	}
}

func recordAudio(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action := kit.StrArg(req, "action")
		if action == "" {
			action = "start"
		}
		if action == "stop" {
			res, err := k.Run(ctx, "termux-microphone-record", []string{"-q"}, 0)
			if err != nil {
				return kit.ResultError("%v", err), nil
			}
			if res.ExitCode != 0 {
				return kit.ResultError("termux-microphone-record exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
			}
			return kit.ResultText("recording stopped"), nil
		}
		out, err := kit.RequireStr(req, "output")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		p, err := files.Resolve(out, files.Roots(&k.Cfg.Tools))
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		args := []string{"-f", p}
		if l := int(math.Round(kit.NumArg(req, "limit"))); l > 0 {
			args = append(args, "-l", itoa(l))
		}
		res, err := k.Run(ctx, "termux-microphone-record", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-microphone-record exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("recording started -> %s", p), nil
	}
}

func mediaPlayer(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action, err := kit.RequireStr(req, "action")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		args := []string{action}
		if f := kit.StrArg(req, "file"); f != "" {
			args = append(args, f)
		}
		res, err := k.Run(ctx, "termux-media-player", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-media-player exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		out := strings.TrimSpace(res.Stdout)
		if out == "" {
			out = "ok"
		}
		return kit.ResultText("%s", out), nil
	}
}

func textToSpeech(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := kit.RequireStr(req, "text")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		args := []string{}
		if r := kit.NumArg(req, "rate"); r > 0 {
			args = append(args, "--rate", ftoa(r))
		}
		if p := kit.NumArg(req, "pitch"); p > 0 {
			args = append(args, "--pitch", ftoa(p))
		}
		args = append(args, text)
		res, err := k.Run(ctx, "termux-tts-speak", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-tts-speak exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("spoken: %s", text), nil
	}
}

func speechToText(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := []string{}
		if l := kit.StrArg(req, "lang"); l != "" {
			args = append(args, "-l", l)
		}
		if d := int(math.Round(kit.NumArg(req, "duration"))); d > 0 {
			args = append(args, "-d", itoa(d))
		}
		res, err := k.Run(ctx, "termux-speech-to-text", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-speech-to-text exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("%s", strings.TrimSpace(res.Stdout)), nil
	}
}

func jsonCmd(k *kit.Kit, name string, args []string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := k.RunJSON(ctx, name, args, 0, true)
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
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func ftoa(f float64) string {
	return strings.TrimRight(strings.TrimRight(strings.TrimSuffix(strings.TrimSpace(
		// avoid scientific notation for common rates
		strconv.FormatFloat(f, 'f', -1, 64)), "0"), "."), ".")
}
