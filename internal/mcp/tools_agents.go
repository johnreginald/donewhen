package mcp

import (
	"context"
	"encoding/json"
	"fmt"

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
