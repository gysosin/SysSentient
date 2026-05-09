package ai

import (
	"context"
	"fmt"
	"sys-sentient/internal/config"
	"sys-sentient/internal/models"
	"time"

	"google.golang.org/genai"
)

type AIService struct {
	client         *genai.Client
	model          string
	ragStore       *RAGStore
	circuitBreaker *CircuitBreaker
}

func NewAIService(ctx context.Context, cfg config.GeminiConfig) (*AIService, error) {
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
		client:         client,
		model:          cfg.ModelName,
		ragStore:       NewRAGStore(),
		circuitBreaker: NewCircuitBreaker(3, 2*time.Minute), // 3 failures, 2min reset
	}, nil
}

func (s *AIService) AnalyzeSystemState(ctx context.Context, state models.SystemState, logs string) (string, error) {
	optimizedLogs := CollapseLogs(logs)

	signatureContent := state.TopProcesses + "\n" + optimizedLogs
	if cached, found := s.ragStore.GetCachedInsight(signatureContent); found {
		// Cached insight is already a JSON string
		return cached, nil
	}

	// Use circuit breaker to protect against repeated API failures
	var result string
	err := s.circuitBreaker.Execute(func() error {
		prompt := fmt.Sprintf(`
You are a system administrator AI. Analyze the metrics below.
Timestamp: %s
CPU: %.2f%%
RAM: %d / %d bytes
Disk: Read %d, Write %d
Net: Sent %d, Recv %d
Temp: %.1f C

Top Processes:
%s

Logs:
%s

Respond STRICTLY in JSON format with this structure:
{
  "status": "Healthy" | "Warning" | "Critical",
  "summary": "One sentence summary.",
  "detailedAnalysis": "Use bullet points and short paragraphs. No markdown bolding (**). Plain text or simple formatting.",
  "recommendedActions": [
    { "id": "unique_id", "command": "suggested shell command", "description": "what it does", "isSafe": boolean }
  ]
}
`,
			state.Timestamp,
			state.CPUUsage,
			state.MemoryUsed, state.MemoryTotal,
			state.DiskReadBytes, state.DiskWriteBytes,
			state.NetSentBytes, state.NetRecvBytes,
			state.Temperature,
			state.TopProcesses,
			optimizedLogs,
		)

		// Configure for JSON response
		resp, apiErr := s.client.Models.GenerateContent(ctx, s.model, genai.Text(prompt), &genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
		})
		if apiErr != nil {
			return fmt.Errorf("failed to generate content: %w", apiErr)
		}

		result = resp.Text()
		return nil
	})

	if err != nil {
		// Check if circuit is open
		if err == ErrCircuitOpen {
			return "", fmt.Errorf("AI service unavailable (circuit breaker open): too many recent failures")
		}
		return "", err
	}

	s.ragStore.SaveInsight(signatureContent, result)

	return result, nil
}
