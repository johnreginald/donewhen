package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

// Tools an agent uses to talk to the human on a ticket, the way Paperclip's
// agents do: structured questions answered in the web UI instead of a
// terminal.
func (d *deps) registerAgents(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("ask_user",
		mcp.WithDescription("Ask the human structured questions on a ticket. They appear as a question card "+
			"on the ticket in Raenil; the answers come back to you as your next turn. Prefer a few questions "+
			"with concrete options over open-ended ones. After asking, END YOUR TURN: say briefly what you "+
			"asked and stop — do not guess the answers."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue key, e.g. RAE-12")),
		mcp.WithArray("questions", mcp.Required(),
			mcp.Description("At most 8. Each: {text, options?, multi?, allowOther?}. A question with no options is answered in words."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text":       map[string]any{"type": "string", "description": "The question"},
					"options":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Choices to pick from"},
					"multi":      map[string]any{"type": "boolean", "description": "Allow several choices"},
					"allowOther": map[string]any{"type": "boolean", "description": "Also allow a written answer"},
				},
				"required": []any{"text"},
			})),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref := req.GetString("issue", "")
		wsID, err := d.scopeOne(ctx, req, issueRef(ref))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var qs []models.Question
		raw, _ := json.Marshal(req.GetArguments()["questions"])
		if err := json.Unmarshal(raw, &qs); err != nil {
			return mcp.NewToolResultError("questions must be a list of {text, options, multi, allowOther}"), nil
		}
		qs, err = store.NormaliseQuestions(qs)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		it, err := d.store.CreateInteraction(ctx, wsID, is.ID, agentFrom(ctx), "questions", map[string]any{"questions": qs})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		d.svc.Bus.Publish(events.Event{Type: "interaction.created", WorkspaceID: wsID, Actor: "ai", IssueID: is.ID, Interaction: &it})
		return mcp.NewToolResultText(fmt.Sprintf("Posted %d question(s) on %s. End your turn now; "+
			"you will be resumed with the answers.", len(qs), is.Key)), nil
	})

	s.AddTool(mcp.NewTool("add_blocker",
		mcp.WithDescription("Record that a ticket cannot be done until another one is: use it when the work "+
			"needs code or tables another ticket delivers and that ticket is not listed as a blocker. The link is "+
			"saved, the ticket waits, and it starts again on its own once the blocker is In Review or Done, "+
			"building on that ticket's branch. After calling it, END YOUR TURN — say what is missing and stop; do "+
			"not work around the gap."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("The ticket that has to wait, e.g. RAE-21")),
		mcp.WithString("blocker", mcp.Required(), mcp.Description("The ticket it waits on, e.g. RAE-20")),
		mcp.WithString("reason", mcp.Required(), mcp.Description("What this ticket needs from it, in a sentence")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref := req.GetString("issue", "")
		wsID, err := d.scopeOne(ctx, req, issueRef(ref))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		b, err := d.store.AddBlocker(ctx, wsID, is.ID, req.GetString("blocker", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		body := fmt.Sprintf("Waiting on **%s** (%s): %s\n\nThis ticket starts again on its own once %s is In Review or Done.",
			b.Key, b.State, req.GetString("reason", ""), b.Key)
		c, err := d.store.CreateComment(ctx, wsID, is.ID, body, "ai", agentFrom(ctx))
		if err == nil {
			d.svc.Bus.Publish(events.Event{Type: "comment.added", WorkspaceID: wsID, Actor: "ai", IssueID: is.ID, Comment: &c})
		}
		d.svc.Bus.Publish(events.Event{Type: "issue.blockers", WorkspaceID: wsID, Actor: "ai", IssueID: is.ID})
		state := "it is " + b.State + " — "
		if b.Done {
			state = "it is already " + b.State + ", so this ticket can be run again now — "
		}
		return mcp.NewToolResultText(fmt.Sprintf("%s now waits on %s (%s). End your turn now.", is.Key, b.Key, strings.TrimSuffix(state, " — "))), nil
	})
}

func (d *deps) registerProposals(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("propose_tickets",
		mcp.WithDescription("Propose splitting the work discussed on a ticket into new tickets. The human sees an "+
			"approval card; approving creates them under this ticket, in Ready. Every ticket needs at least one "+
			"criterion a program can check: kind \"deterministic\" with {\"cmd\": \"...\", \"expect_exit\": 0}, or kind "+
			"\"policy\" with {\"policy\": \"paths_within\", \"args\": [\"src/**\"]}. Then end your turn."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("The ticket being discussed, e.g. RAE-12")),
		mcp.WithString("summary", mcp.Description("One or two sentences on how the work is split")),
		mcp.WithArray("tickets", mcp.Required(),
			mcp.Description("Each: {title, description, criteria:[{text, kind, check}]}"),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":       map[string]any{"type": "string"},
					"description": map[string]any{"type": "string", "description": "Markdown: what and why, enough to build from"},
					"criteria": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text":  map[string]any{"type": "string"},
							"kind":  map[string]any{"type": "string", "enum": []any{"manual", "deterministic", "policy", "judgment"}},
							"check": map[string]any{"type": "object"},
						},
						"required": []any{"text", "kind"},
					}},
				},
				"required": []any{"title", "criteria"},
			})),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref := req.GetString("issue", "")
		wsID, err := d.scopeOne(ctx, req, issueRef(ref))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var p store.Proposal
		p.Summary = req.GetString("summary", "")
		raw, _ := json.Marshal(req.GetArguments()["tickets"])
		if err := json.Unmarshal(raw, &p.Tickets); err != nil {
			return mcp.NewToolResultError("tickets must be a list of {title, description, criteria}"), nil
		}
		p, err = store.NormaliseProposal(p)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		it, err := d.store.CreateInteraction(ctx, wsID, is.ID, agentFrom(ctx), "proposal", p)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		d.svc.Bus.Publish(events.Event{Type: "interaction.created", WorkspaceID: wsID, Actor: "ai", IssueID: is.ID, Interaction: &it})
		return mcp.NewToolResultText(fmt.Sprintf("Proposed %d ticket(s) on %s for approval. End your turn now.", len(p.Tickets), is.Key)), nil
	})
}
