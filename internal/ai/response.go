package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"sys-sentient/internal/models"
)

func NormalizeAnalysisResponse(raw string) (string, error) {
	var analysis models.AIAnalysis
	if err := json.Unmarshal([]byte(raw), &analysis); err != nil {
		return "", fmt.Errorf("invalid AI analysis JSON: %w", err)
	}

	switch analysis.Status {
	case "Healthy", "Warning", "Critical":
	default:
		analysis.Status = "Warning"
	}

	analysis.Summary = models.FlexText(strings.TrimSpace(analysis.Summary.String()))
	if analysis.Summary == "" {
		analysis.Summary = "AI Analysis Generated"
	}
	analysis.DetailedAnalysis = models.FlexText(strings.TrimSpace(analysis.DetailedAnalysis.String()))
	if analysis.DetailedAnalysis == "" {
		analysis.DetailedAnalysis = analysis.Summary
	}

	actions := make([]models.AIAction, 0, len(analysis.RecommendedActions))
	seenActionIDs := make(map[string]int)
	for idx, action := range analysis.RecommendedActions {
		action.ID = strings.TrimSpace(action.ID)
		action.Command = strings.TrimSpace(action.Command)
		action.Description = strings.TrimSpace(action.Description)
		if action.Command == "" && action.Description == "" {
			continue
		}
		if action.ID == "" {
			action.ID = fmt.Sprintf("action_%d", idx+1)
		}
		baseID := action.ID
		seenActionIDs[baseID]++
		if seenActionIDs[baseID] > 1 {
			for suffix := seenActionIDs[baseID]; ; suffix++ {
				candidate := fmt.Sprintf("%s_%d", baseID, suffix)
				if seenActionIDs[candidate] == 0 {
					action.ID = candidate
					break
				}
			}
		}
		seenActionIDs[action.ID] = 1
		actions = append(actions, action)
	}
	analysis.RecommendedActions = actions

	normalized, err := json.Marshal(analysis)
	if err != nil {
		return "", fmt.Errorf("failed to normalize AI analysis: %w", err)
	}
	return string(normalized), nil
}
