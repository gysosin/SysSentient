package ai

import (
	"context"
	"fmt"
	"sys-sentient/internal/config"
	"sys-sentient/internal/models"

	"google.golang.org/genai"
)

type AIService struct {
	client   *genai.Client
	model    string
	ragStore *RAGStore
}

func NewAIService(ctx context.Context, cfg config.GeminiConfig) (*AIService, error) {
	// If API Key is empty, we might want to return nil or error, but let's allow it for now
	// and fail on request if needed, or error out here.
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is missing")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	return &AIService{
		client:   client,
		model:    cfg.ModelName,
		ragStore: NewRAGStore(),
	}, nil
}

func (s *AIService) AnalyzeSystemState(ctx context.Context, state models.SystemState, logs string) (string, error) {
	// 1. Optimize Logs
	optimizedLogs := CollapseLogs(logs)

	// 2. Check RAG Cache (Deduplication)
	// We use the combination of TopProcesses and Logs as the "signature" of the state
	// Metrics themselves (CPU%) change too often to be part of the cache key usually,
	// but significantly different states should trigger different insights.
	// For this phase, we'll cache based on the LOGS + PROCESSES pattern.
	signatureContent := state.TopProcesses + "\n" + optimizedLogs
	if cached, found := s.ragStore.GetCachedInsight(signatureContent); found {
		return cached + "\n[Cached Analysis]", nil
	}

	prompt := fmt.Sprintf(`
Analyze the following system metrics and logs for potential issues.
Timestamp: %s
CPU Usage: %.2f%%
Memory: %d / %d bytes
Disk I/O: Read %d, Write %d
Network: Sent %d, Recv %d

Top Processes:
%s

Recent Logs:
%s

Provide a concise summary of the system health and any recommendations.
`,
		state.Timestamp,
		state.CPUUsage,
		state.MemoryUsed, state.MemoryTotal,
		state.DiskReadBytes, state.DiskWriteBytes,
		state.NetSentBytes, state.NetRecvBytes,
		state.TopProcesses,
		optimizedLogs,
	)

	resp, err := s.client.Models.GenerateContent(ctx, s.model, genai.Text(prompt), nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	result := resp.Text()
	
	// 3. Save to Cache
	s.ragStore.SaveInsight(signatureContent, result)

	return result, nil
}
