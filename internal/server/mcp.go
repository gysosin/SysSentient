package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sys-sentient/internal/version"
)

// MCP exposes the assistant's tools to outside clients.
//
// The same read-only surface the in-dashboard assistant uses, so a question
// answerable in the console is answerable from Claude Desktop without a second
// implementation to keep in step.

type mcpWindowArgs struct {
	HostID string `json:"host_id,omitempty" jsonschema:"Host id to scope to. Omit for all hosts."`
	From   string `json:"from" jsonschema:"Start of the window, RFC3339."`
	To     string `json:"to" jsonschema:"End of the window, RFC3339."`
}

type mcpInstantArgs struct {
	HostID string `json:"host_id,omitempty" jsonschema:"Host id to scope to. Omit for all hosts."`
	At     string `json:"at" jsonschema:"The moment to look at, RFC3339."`
}

type mcpLimitArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"How many to return."`
}

type mcpNoArgs struct{}

// toolFailure reports a tool error to the client as *content*, not as a
// protocol error.
//
// This is the MCP contract, and it is the more useful behaviour: the client's
// model can read the message and try something else, where a transport error
// aborts the whole call. The returned Go error is deliberately nil — the call
// itself succeeded, and its result says the tool did not.
func toolFailure(err error) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: "error: " + err.Error()}},
	}, nil, nil
}

// toolText wraps successful output in the shape MCP expects.
func toolText(out string) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(out) == "" {
		// Distinguished from a failure: "nothing there" is an answer.
		out = "no data for that request"
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil, nil
}

// reply routes a toolbox call to whichever of the two applies.
func reply(out string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return toolFailure(err)
	}
	return toolText(out)
}

func parseRFC3339(field, raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339, got %q", field, raw)
	}
	return t, nil
}

// newMCPServer builds the server for one connection.
func (s *Server) newMCPServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "sys-sentient",
		Title:   "SysSentient",
		Version: version.Get().Version,
	}, nil)

	box := s.toolbox()

	mcp.AddTool(srv, &mcp.Tool{
		Name: "query_metrics",
		Description: "Summarise CPU, memory, swap, load, disk and network over a time window: " +
			"averages, peaks and direction. Use this to answer what a machine was doing in the past.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpWindowArgs) (*mcp.CallToolResult, any, error) {
		from, err := parseRFC3339("from", in.From)
		if err != nil {
			return toolFailure(err)
		}
		to, err := parseRFC3339("to", in.To)
		if err != nil {
			return toolFailure(err)
		}
		return reply(box.QueryMetrics(ctx, in.HostID, from, to))
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "top_processes",
		Description: "The heaviest processes by CPU and by memory at a moment in time. " +
			"Use after query_metrics shows a spike, to find what caused it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpInstantArgs) (*mcp.CallToolResult, any, error) {
		at, err := parseRFC3339("at", in.At)
		if err != nil {
			return toolFailure(err)
		}
		return reply(box.TopProcesses(ctx, in.HostID, at))
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_hosts",
		Description: "The machines reporting to this server, with their ids and when each was last seen.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoArgs) (*mcp.CallToolResult, any, error) {
		return reply(box.ListHosts(ctx))
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "recent_alerts",
		Description: "Recent alert transitions: what fired, when, on which host, and whether it resolved.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpLimitArgs) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		return reply(box.RecentAlerts(ctx, limit))
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "recent_logs",
		Description: "Recent system log lines, useful for correlating a spike with an event.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoArgs) (*mcp.CallToolResult, any, error) {
		return reply(box.RecentLogs(ctx))
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "recent_insights",
		Description: "Past AI analyses of this system, newest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpLimitArgs) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		return reply(box.RecentInsights(ctx, limit))
	})

	return srv
}

// mcpHandler serves MCP over HTTP.
//
// Authenticated exactly like every other API route — a session cookie or an
// API key — so an MCP client uses a credential the operator already
// understands and can revoke, rather than a second, parallel notion of access.
func (s *Server) mcpHandler() http.Handler {
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s.newMCPServer() },
		nil,
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.authenticate(r); !ok {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		handler.ServeHTTP(w, r)
	})
}
