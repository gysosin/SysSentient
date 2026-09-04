package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/genai"
)

// ChatMessage is one turn of a conversation.
type ChatMessage struct {
	// Role is "user" or "model".
	Role string `json:"role"`
	Text string `json:"text"`
}

// ChatReply is the assistant's answer, with the work it did to reach it.
type ChatReply struct {
	Text string `json:"text"`
	// ToolCalls names the tools consulted, in order. Shown to the operator so
	// an answer can be checked against the data it came from rather than
	// taken on trust — the whole difference between an assistant and a
	// plausible-sounding paragraph.
	ToolCalls []string `json:"tool_calls"`
}

// chatSystemPrompt frames what the assistant is and what it must not do.
const chatSystemPrompt = `You are the assistant inside SysSentient, a self-hosted server monitor.

Answer questions about the machines this server is monitoring using the tools
provided. The tools are the only source of truth you have: never invent a
number, a hostname or a timestamp. If a tool returns no data, say so plainly.

Guidance:
- Call list_hosts first when a question names a machine you have not seen.
- For "what happened at X", query a window around X, then look at the processes
  at the moment of interest.
- Prefer specifics: name the process, the value and the time.
- Be brief. An operator reading this is usually mid-incident.
- You may suggest shell commands, but say plainly that they are suggestions and
  that nothing here runs them. You have no ability to change this system.`

// Chat answers a question, calling tools as needed.
//
// This is the difference between the one-shot analysis and an assistant: the
// model can look things up, then look up more based on what it found, rather
// than reasoning over a single snapshot it was handed.
func (s *AIService) Chat(
	ctx context.Context,
	box Toolbox,
	history []ChatMessage,
	question string,
) (ChatReply, error) {
	if s == nil || s.client == nil {
		return ChatReply{}, fmt.Errorf("AI service is not configured")
	}
	if strings.TrimSpace(question) == "" {
		return ChatReply{}, fmt.Errorf("question is empty")
	}
	// The same daily cap as the one-shot analysis: a chat loop can call the
	// model many times per question, so an uncapped one is the easiest way to
	// spend real money by accident.
	if err := s.budget.Check(); err != nil {
		return ChatReply{}, err
	}

	contents := make([]*genai.Content, 0, len(history)+2)
	for _, msg := range history {
		role := genai.Role(genai.RoleUser)
		if msg.Role == "model" {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(msg.Text, role))
	}
	contents = append(contents, genai.NewContentFromText(question, genai.RoleUser))

	config := &genai.GenerateContentConfig{
		Tools:             toolDeclarations(),
		SystemInstruction: genai.NewContentFromText(chatSystemPrompt, genai.RoleUser),
	}

	var called []string
	for range maxToolCalls {
		resp, err := s.client.Models.GenerateContent(ctx, s.model, contents, config)
		if err != nil {
			return ChatReply{}, fmt.Errorf("chat request failed: %w", err)
		}
		s.recordUsage(resp)

		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return ChatReply{}, fmt.Errorf("the model returned nothing")
		}
		content := resp.Candidates[0].Content

		calls := functionCalls(content)
		if len(calls) == 0 {
			// No more tools wanted: this is the answer.
			return ChatReply{Text: strings.TrimSpace(resp.Text()), ToolCalls: called}, nil
		}

		// Carry the model's own turn forward, or the next request loses the
		// context in which it asked for these tools.
		contents = append(contents, content)

		results := make([]*genai.Part, 0, len(calls))
		for _, call := range calls {
			slog.Debug("assistant tool call", "tool", call.Name, "args", call.Args)
			called = append(called, call.Name)
			output := dispatchTool(ctx, box, call.Name, call.Args)
			results = append(results, genai.NewPartFromFunctionResponse(
				call.Name, map[string]any{"result": output}))
		}
		contents = append(contents, genai.NewContentFromParts(results, genai.RoleUser))
	}

	// The loop bound was reached. Saying so is better than returning the last
	// half-formed thought as if it were an answer.
	return ChatReply{
		Text: "I could not reach an answer within the tool-call limit for one question. " +
			"Try asking something narrower — a specific host, or a shorter window.",
		ToolCalls: called,
	}, nil
}

// functionCalls extracts the tool calls from a model turn.
func functionCalls(content *genai.Content) []*genai.FunctionCall {
	calls := make([]*genai.FunctionCall, 0, 2)
	for _, part := range content.Parts {
		if part.FunctionCall != nil {
			calls = append(calls, part.FunctionCall)
		}
	}
	return calls
}

// recordUsage charges a response against the daily budget.
//
// Shared with the one-shot path so a chat turn is not free money: a
// conversation can make many model calls, and each one costs.
func (s *AIService) recordUsage(resp *genai.GenerateContentResponse) {
	if resp == nil || resp.UsageMetadata == nil {
		return
	}
	s.budget.Record(
		int64(resp.UsageMetadata.PromptTokenCount),
		int64(resp.UsageMetadata.CandidatesTokenCount),
	)
}
