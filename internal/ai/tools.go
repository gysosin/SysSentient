package ai

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genai"
)

// Toolbox is the read-only surface the assistant can call.
//
// An interface rather than a direct dependency on storage: the same surface is
// exposed over MCP, and neither caller should have to reach through the AI
// package to get at the database.
//
// Every method returns text the model reads directly. Compact prose and small
// tables cost far fewer tokens than raw JSON, and the model is being asked to
// reason about the numbers, not to parse them.
type Toolbox interface {
	// QueryMetrics summarises a window: averages, peaks and the trend.
	QueryMetrics(ctx context.Context, hostID string, from, to time.Time) (string, error)
	// TopProcesses reports what was running at an instant.
	TopProcesses(ctx context.Context, hostID string, at time.Time) (string, error)
	// ListHosts names the machines reporting.
	ListHosts(ctx context.Context) (string, error)
	// RecentAlerts lists alert transitions, newest first.
	RecentAlerts(ctx context.Context, limit int) (string, error)
	// RecentLogs returns collected system log lines.
	RecentLogs(ctx context.Context) (string, error)
	// RecentInsights returns past analyses.
	RecentInsights(ctx context.Context, limit int) (string, error)
}

// toolNames are declared once so the dispatcher and the declarations cannot
// drift apart.
const (
	toolQueryMetrics   = "query_metrics"
	toolTopProcesses   = "top_processes"
	toolListHosts      = "list_hosts"
	toolRecentAlerts   = "recent_alerts"
	toolRecentLogs     = "recent_logs"
	toolRecentInsights = "recent_insights"
)

// maxToolCalls bounds one conversation turn.
//
// A model that keeps calling tools without answering would otherwise spend the
// operator's budget in a loop. Ten is generous for "why was it slow at 3pm?"
// and cheap to cap.
const maxToolCalls = 10

// toolDeclarations describes the surface to the model.
//
// Every description says what the tool is *for*, not only what it takes: a
// model choosing between six tools does so from these sentences.
func toolDeclarations() []*genai.Tool {
	str := func(desc string) *genai.Schema {
		return &genai.Schema{Type: genai.TypeString, Description: desc}
	}
	hostParam := str("Host id to scope to. Omit for all hosts. Get ids from list_hosts.")
	rfc3339 := func(what string) *genai.Schema {
		return str(what + " as an RFC3339 timestamp, for example 2026-09-04T14:00:00Z.")
	}

	return []*genai.Tool{{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: toolQueryMetrics,
				Description: "Summarise CPU, memory, swap, load, disk and network over a time " +
					"window: averages, peaks and direction. Use this to answer what the machine " +
					"was doing at some point in the past.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"host_id": hostParam,
						"from":    rfc3339("Start of the window"),
						"to":      rfc3339("End of the window"),
					},
					Required: []string{"from", "to"},
				},
			},
			{
				Name: toolTopProcesses,
				Description: "The heaviest processes by CPU and by memory at a moment in time. " +
					"Use this after query_metrics shows a spike, to find what caused it.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"host_id": hostParam,
						"at":      rfc3339("The moment to look at"),
					},
					Required: []string{"at"},
				},
			},
			{
				Name:        toolListHosts,
				Description: "The machines reporting to this server, with their ids and when each was last seen.",
				Parameters:  &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}},
			},
			{
				Name:        toolRecentAlerts,
				Description: "Recent alert transitions — what fired, when, on which host, and whether it resolved.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"limit": {Type: genai.TypeInteger, Description: "How many to return. Default 20."},
					},
				},
			},
			{
				Name:        toolRecentLogs,
				Description: "Recent system log lines from the host. Useful for correlating a spike with an event.",
				Parameters:  &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}},
			},
			{
				Name:        toolRecentInsights,
				Description: "Past AI analyses of this system, newest first.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"limit": {Type: genai.TypeInteger, Description: "How many to return. Default 5."},
					},
				},
			},
		},
	}}
}

// dispatchTool runs one call and returns what the model should see.
//
// A failure is returned as text rather than an error: the model can say "I
// could not read the logs" and carry on, where aborting the turn would lose
// the work it had already done.
func dispatchTool(ctx context.Context, box Toolbox, name string, args map[string]any) string {
	hostID, _ := args["host_id"].(string)

	parseTime := func(key string) (time.Time, error) {
		raw, ok := args[key].(string)
		if !ok || raw == "" {
			return time.Time{}, fmt.Errorf("%s is required", key)
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s must be RFC3339, got %q", key, raw)
		}
		return t, nil
	}

	limit := func(fallback int) int {
		if v, ok := args["limit"].(float64); ok && v > 0 {
			return int(v)
		}
		return fallback
	}

	var (
		out string
		err error
	)
	switch name {
	case toolQueryMetrics:
		from, ferr := parseTime("from")
		if ferr != nil {
			return "error: " + ferr.Error()
		}
		to, terr := parseTime("to")
		if terr != nil {
			return "error: " + terr.Error()
		}
		out, err = box.QueryMetrics(ctx, hostID, from, to)
	case toolTopProcesses:
		at, aerr := parseTime("at")
		if aerr != nil {
			return "error: " + aerr.Error()
		}
		out, err = box.TopProcesses(ctx, hostID, at)
	case toolListHosts:
		out, err = box.ListHosts(ctx)
	case toolRecentAlerts:
		out, err = box.RecentAlerts(ctx, limit(20))
	case toolRecentLogs:
		out, err = box.RecentLogs(ctx)
	case toolRecentInsights:
		out, err = box.RecentInsights(ctx, limit(5))
	default:
		return "error: unknown tool " + name
	}

	if err != nil {
		return "error: " + err.Error()
	}
	if out == "" {
		// Distinguished from a failure: "nothing there" is an answer.
		return "no data for that request"
	}
	return out
}
