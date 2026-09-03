package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	budget         *CostBudget
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
		// Enforces gemini.max_daily_cost, which the config has always
		// advertised and nothing previously read.
		budget: NewCostBudget(cfg.MaxDailyCost),
	}, nil
}

func (s *AIService) AnalyzeSystemState(ctx context.Context, state models.SystemState, logs string) (string, error) {
	optimizedLogs := CollapseLogs(logs)

	signatureContent := state.TopProcesses + "\n" + optimizedLogs
	if cached, found := s.ragStore.GetCachedInsight(signatureContent); found {
		// Cached insight is already a JSON string
		return cached, nil
	}

	// Refuse to spend past the operator's daily cap. Checked after the cache
	// so a cached answer is still served once the budget is exhausted.
	if err := s.budget.Check(); err != nil {
		return "", err
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

		// Record spend from the API's own usage metadata when present, and
		// fall back to a length estimate so an unreported call still counts.
		var inputTokens, outputTokens int64
		if resp.UsageMetadata != nil {
			inputTokens = int64(resp.UsageMetadata.PromptTokenCount)
			outputTokens = int64(resp.UsageMetadata.CandidatesTokenCount)
		}
		if inputTokens == 0 {
			inputTokens = EstimateTokens(prompt)
		}
		if outputTokens == 0 {
			outputTokens = EstimateTokens(resp.Text())
		}
		cost := s.budget.Record(inputTokens, outputTokens)
		if spent, limit := s.budget.Spent(); limit > 0 {
			slog.Debug("gemini call accounted",
				"cost_usd", cost, "spent_today_usd", spent, "daily_limit_usd", limit)
		}

		normalized, err := NormalizeAnalysisResponse(resp.Text())
		if err != nil {
			return err
		}
		result = normalized
		return nil
	})

	if err != nil {
		// Check if circuit is open
		if errors.Is(err, ErrCircuitOpen) {
			return "", fmt.Errorf("AI service unavailable (circuit breaker open): too many recent failures")
		}
		return "", err
	}

	s.ragStore.SaveInsight(signatureContent, result)

	return result, nil
}

// BudgetStatus reports today's AI spend against the configured cap, so the
// operator can see whether the limit is doing anything.
func (s *AIService) BudgetStatus() (spent, limit float64) {
	if s == nil {
		return 0, 0
	}
	return s.budget.Spent()
}
