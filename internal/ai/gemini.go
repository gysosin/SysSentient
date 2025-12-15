package ai

import (
	"context"
	"fmt"
	"sys-sentient/internal/config"
	"sys-sentient/internal/models"

	"google.golang.org/genai"
)

type AIService struct {
	client *genai.Client
	model  string
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
		client: client,
		model:  cfg.ModelName,
	}, nil
}

func (s *AIService) AnalyzeSystemState(ctx context.Context, state models.SystemState, logs string) (string, error) {
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
		logs,
	)

	resp, err := s.client.Models.GenerateContent(ctx, s.model, genai.Text(prompt), nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	return resp.Text(), nil
}
