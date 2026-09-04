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
  "detailedAnalysis": "A SINGLE STRING, not a list. Separate bullet points with newlines. No markdown bolding (**).",
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

		// Constrain the shape as well as the encoding. Asking for JSON alone
		// left the model free to answer "use bullet points" with an array of
		// bullets, which is a reasonable reading of the prompt and a type
		// error for the decoder. The schema removes the ambiguity; FlexText
		// still absorbs it if a model disregards the schema anyway.
		resp, apiErr := s.client.Models.GenerateContent(ctx, s.model, genai.Text(prompt), &genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   analysisSchema(),
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

// analysisSchema pins the response shape the decoder expects.
func analysisSchema() *genai.Schema {
	str := func(description string) *genai.Schema {
		return &genai.Schema{Type: genai.TypeString, Description: description}
	}
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"status": {
				Type: genai.TypeString,
				Enum: []string{"Healthy", "Warning", "Critical"},
			},
			"summary": str("One sentence."),
			"detailedAnalysis": str(
				"Plain text. Use newline-separated lines for bullet points; " +
					"this must be a single string, not a list."),
			"recommendedActions": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"id":          str("Unique identifier."),
						"command":     str("Suggested shell command."),
						"description": str("What the command does."),
						"isSafe":      {Type: genai.TypeBoolean},
					},
					Required: []string{"id", "command", "description", "isSafe"},
				},
			},
		},
		Required: []string{"status", "summary", "detailedAnalysis", "recommendedActions"},
	}
}
