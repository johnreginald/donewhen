package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"raenil/internal/models"
)

func TestChatPrompts(t *testing.T) {
	agentID := "ag-1"
	a := models.Agent{ID: agentID, Name: "Backend Engineer", Role: "Go", InstructionsMD: "Run go test before you say done."}
	is := models.Issue{Key: "RAE-12", Title: "Add rate limiting", DescriptionMD: "Per token."}
	t0 := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	comments := []models.Comment{
		{BodyMD: "How should we limit?", Actor: "human", CreatedAt: t0},
		{BodyMD: "I have questions.", Actor: "ai", AgentID: &agentID, CreatedAt: t0.Add(time.Minute)},
		{BodyMD: "Also cover the MCP endpoint.", Actor: "human", CreatedAt: t0.Add(3 * time.Minute)},
	}
	resolved := t0.Add(2 * time.Minute)
	qs, _ := json.Marshal(map[string]any{"questions": []models.Question{{ID: "q1", Text: "Window?", Options: []string{"minute", "hour"}}}})
	ans, _ := json.Marshal(map[string]any{"answers": []models.Answer{{QuestionID: "q1", Choices: []string{"minute"}}}})
	its := []models.Interaction{{ID: "it-1", Kind: "questions", Payload: qs, Status: "answered", Response: ans,
		CreatedAt: t0.Add(90 * time.Second), ResolvedAt: &resolved}}

	brief := chatBrief(a, is, []models.Criterion{{Body: "go test passes"}}, comments, its, chatInput{Reason: "message"})
	for _, want := range []string{"You are Backend Engineer (Go)", "Run go test before you say done.", "do not change any file",
		"ask_user", "## Ticket RAE-12: Add rate limiting", "- [ ] go test passes", "**User:** How should we limit?",
		"**You:** I have questions.", "Window? (options: minute / hour)", "Window? → minute", "Reply to the user's latest message."} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief lacks %q\n---\n%s", want, brief)
		}
	}
	// Time order: the question comes after the agent's reply and before the last message.
	if strings.Index(brief, "I have questions.") > strings.Index(brief, "Window? (options") ||
		strings.Index(brief, "Window? →") > strings.Index(brief, "Also cover the MCP endpoint.") {
		t.Errorf("thread is out of order:\n%s", brief)
	}

	follow := chatFollowUp(a, comments, its, chatInput{Reason: "response", InteractionID: "it-1"})
	if strings.Contains(follow, "How should we limit?") || strings.Contains(follow, "I have questions.") {
		t.Errorf("follow-up repeats what the agent already saw:\n%s", follow)
	}
	for _, want := range []string{"Also cover the MCP endpoint.", "Window? → minute", "The user answered your questions"} {
		if !strings.Contains(follow, want) {
			t.Errorf("follow-up lacks %q\n---\n%s", want, follow)
		}
	}
}
