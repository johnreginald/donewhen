package push

import (
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
)

func TestBuildNamesTheAIActor(t *testing.T) {
	n := &Notifier{}
	is := &models.Issue{ID: "i1", Key: "PP-1", Title: "Do it"}
	to := &models.WorkflowState{Name: "In Review"}

	p, ok := n.build(events.Event{Type: events.IssueStateChanged, Actor: "ai", Issue: is, To: to}, "Claude")
	if !ok || !strings.Contains(p.Body, "moved by Claude") {
		t.Fatalf("ai move: ok=%v body=%q", ok, p.Body)
	}
	p, _ = n.build(events.Event{Type: events.IssueStateChanged, Actor: "human", Issue: is, To: to}, "Claude")
	if !strings.Contains(p.Body, "moved by you") {
		t.Fatalf("human move: body=%q", p.Body)
	}
}
