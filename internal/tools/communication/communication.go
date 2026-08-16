// Package communication implements Module B: SMS, contacts, calls,
// notifications and toasts (sensitive tier, opt-in).
package communication

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

// All returns the communication module's tools.
func All(k *kit.Kit) []registry.Tool {
	return []registry.Tool{
		{Def: mcp.NewTool("list_sms",
			mcp.WithDescription("List recent SMS/MMS messages via termux-sms-list."),
			mcp.WithNumber("limit", mcp.Description("Max messages, 1-100 (default 10)"), mcp.Min(1), mcp.Max(100)),
			mcp.WithString("type", mcp.Description("Filter: all|inbox|sent|draft|outbox (default all)"),
				mcp.Enum("all", "inbox", "sent", "draft", "outbox"))),
			Handler: listSMS(k),
			Meta:    registry.Meta{Module: "communication", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("send_sms",
			mcp.WithDescription("Send an SMS via termux-sms-send."),
			mcp.WithString("number", mcp.Required(), mcp.Description("Recipient phone number")),
			mcp.WithString("text", mcp.Required(), mcp.Description("Message body"))),
			Handler: sendSMS(k),
			Meta:    registry.Meta{Module: "communication", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("list_contacts",
			mcp.WithDescription("List device contacts via termux-contact-list.")),
			Handler: jsonCmd(k, "termux-contact-list", nil),
			Meta:    registry.Meta{Module: "communication", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("get_call_log",
			mcp.WithDescription("Get recent call log entries via termux-call-log."),
			mcp.WithNumber("limit", mcp.Description("Max entries, 1-100 (default 20)"), mcp.Min(1), mcp.Max(100))),
			Handler: getCallLog(k),
			Meta:    registry.Meta{Module: "communication", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("make_call",
			mcp.WithDescription("Place a phone call via termux-telephony-call."),
			mcp.WithString("number", mcp.Required(), mcp.Description("Phone number to call"))),
			Handler: makeCall(k),
			Meta:    registry.Meta{Module: "communication", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("get_notifications",
			mcp.WithDescription("List current notifications via termux-notification-list.")),
			Handler: jsonCmd(k, "termux-notification-list", nil),
			Meta:    registry.Meta{Module: "communication", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("send_notification",
			mcp.WithDescription("Post a notification via termux-notification."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Notification title")),
			mcp.WithString("content", mcp.Required(), mcp.Description("Notification body")),
			mcp.WithString("id", mcp.Description("Notification id (replaces an existing notification with the same id)"))),
			Handler: sendNotification(k),
			Meta:    registry.Meta{Module: "communication", Tier: registry.TierSensitive}},

		{Def: mcp.NewTool("show_toast",
			mcp.WithDescription("Show a transient toast message via termux-toast."),
			mcp.WithString("text", mcp.Required(), mcp.Description("Toast text")),
			mcp.WithString("length", mcp.Description("short (default) or long"), mcp.Enum("short", "long"))),
			Handler: showToast(k),
			Meta:    registry.Meta{Module: "communication", Tier: registry.TierSensitive}},
	}
}

func listSMS(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		limit := int(math.Round(kit.NumArg(req, "limit")))
		if limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			limit = 100
		}
		args := []string{"-l", itoa(limit)}
		if t := kit.StrArg(req, "type"); t != "" && t != "all" {
			args = append(args, "-t", t)
		}
		return jsonCmd(k, "termux-sms-list", args)(ctx, req)
	}
}

func sendSMS(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		num, err := kit.RequireStr(req, "number")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		text, err := kit.RequireStr(req, "text")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		res, err := k.Run(ctx, "termux-sms-send", []string{"-n", num, text}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-sms-send exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("SMS sent to %s", num), nil
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

func getCallLog(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		limit := int(math.Round(kit.NumArg(req, "limit")))
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
		return jsonCmd(k, "termux-call-log", []string{"-l", itoa(limit)})(ctx, req)
	}
}

func makeCall(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		num, err := kit.RequireStr(req, "number")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		res, err := k.Run(ctx, "termux-telephony-call", []string{num}, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-telephony-call exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("calling %s", num), nil
	}
}

func sendNotification(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := kit.RequireStr(req, "title")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		content, err := kit.RequireStr(req, "content")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		args := []string{"--title", title, "--content", content}
		if id := kit.StrArg(req, "id"); id != "" {
			args = append(args, "--id", id)
		}
		res, err := k.Run(ctx, "termux-notification", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-notification exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("notification posted: %s", title), nil
	}
}

func showToast(k *kit.Kit) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := kit.RequireStr(req, "text")
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		args := []string{text}
		if l := kit.StrArg(req, "length"); l != "" {
			args = append([]string{"-s", l}, args...)
		}
		res, err := k.Run(ctx, "termux-toast", args, 0)
		if err != nil {
			return kit.ResultError("%v", err), nil
		}
		if res.ExitCode != 0 {
			return kit.ResultError("termux-toast exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr)), nil
		}
		return kit.ResultText("toast shown"), nil
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
