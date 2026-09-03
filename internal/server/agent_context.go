package server

import (
	"context"

	"sys-sentient/internal/storage"
)

type agentContextKey struct{}

// withAgent attaches the credential that authenticated an ingest request, so
// the handler can record last-seen without looking it up a second time.
func withAgent(ctx context.Context, a *storage.Agent) context.Context {
	return context.WithValue(ctx, agentContextKey{}, a)
}

// agentFrom returns the authenticated agent, if the request carried one.
// A request authenticated by the shared key has none.
func agentFrom(ctx context.Context) (*storage.Agent, bool) {
	a, ok := ctx.Value(agentContextKey{}).(*storage.Agent)
	return a, ok
}
