// Package demo seeds a sample workspace: a small fictional "Acme" product with
// issues in every state, done-when checklists, blockers, documents, comments
// and activity. The content is plain data at the top of this file. Seeding goes
// through the service layer, so it obeys the same rules as real use, including
// the done-when gate: an issue gets its checklist ticked before it moves to In
// Review or Done.
package demo

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"

	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/service"
	"github.com/johnreginald/donewhen/internal/store"
)

// Defaults for the demo workspace.
const (
	Name   = "Demo"
	Slug   = "demo"
	Prefix = "DEMO"
)

// prefix is a variable only so tests can pick a unique one.
var prefix = Prefix

// ErrExists means the demo workspace is already there; nothing was changed.
var ErrExists = errors.New("demo already exists")

type crit struct {
	Text string
	Done bool
}

type comment struct {
	Actor string // human | ai
	Body  string
}

type doc struct {
	Title string
	Type  string
	Body  string
}

type issue struct {
	Ref      string
	Title    string
	Desc     string
	State    string
	Priority int
	Epic     int // index into epics
	Labels   []string
	Criteria []crit
	Blocked  []string // refs of issues this one waits on
	Comments []comment
	Commits  []string // commit messages (fake shas are derived)
	Doc      *doc
}

type epic struct{ Name, Desc string }

var domainLabels = []string{"onboarding", "billing", "reports"}

var epics = []epic{
	{"Onboarding flow", "Get a new Acme customer from sign-up to a first working dashboard."},
	{"Billing and invoices", "Plans, payments and invoices for Acme accounts."},
	{"Reports and exports", "Usage reports, CSV export and scheduled email summaries."},
}

const initiativeName = "Acme web app"
const initiativeDesc = "The Acme customer web app: sign-up, billing and reporting."

var issues = []issue{
	// ---- Done ----
	{
		Ref: "signup", Title: "Email sign-up with verification", State: "Done", Priority: 2, Epic: 0,
		Labels: []string{"feature", "onboarding"},
		Desc:   "## Goal\nA visitor creates an Acme account with an email and password, and verifies the address from a link.",
		Criteria: []crit{
			{"Sign-up form creates an unverified account", true},
			{"Verification email carries a single-use link", true},
			{"An expired link shows a clear message and offers a resend", true},
		},
		Comments: []comment{
			{"ai", "Built the form, the token table and the email template. All three checks pass."},
			{"human", "Looks good. Merged."},
		},
		Commits: []string{"Add email sign-up and verification links"},
		Doc: &doc{Title: "Sign-up and email verification", Type: "change", Body: `## Summary
A visitor signs up with an email and password. Acme sends a verification link. The account stays locked until the link is opened.

## How it works
` + "```mermaid" + `
sequenceDiagram
  participant U as Visitor
  participant W as Web app
  participant A as API
  participant M as Mailer
  U->>W: Submit sign-up form
  W->>A: POST /signup
  A->>A: Store user and single-use token
  A->>M: Send verification email
  M-->>U: Email with link
  U->>A: GET /verify?token=abc
  A-->>U: Account unlocked
` + "```" + `

## Key files
- ` + "`api/signup.go`" + ` handles the form post
- ` + "`api/verify.go`" + ` checks the token and unlocks the account

## Decisions
- Tokens expire after 24 hours, so a leaked email is harmless later.
- The link is single use. A second click shows the sign-in page.
`},
	},
	{
		Ref: "invoicepdf", Title: "Render invoices as PDF", State: "Done", Priority: 3, Epic: 1,
		Labels: []string{"feature", "billing"},
		Desc:   "Customers download each invoice as a PDF from the billing page.",
		Criteria: []crit{
			{"Invoice page has a Download PDF button", true},
			{"PDF shows line items, tax and total", true},
			{"Amounts match the invoice screen to the cent", true},
		},
		Comments: []comment{{"ai", "Used the server-side renderer. Totals are computed once and reused by the screen and the PDF."}},
		Commits:  []string{"Render invoices as PDF"},
		Doc: &doc{Title: "Invoice PDF rendering", Type: "change", Body: `## Summary
Invoices can be downloaded as PDF. The screen and the PDF read the same totals, so they cannot disagree.

## How it works
` + "```mermaid" + `
flowchart LR
  page["Billing page"] -->|"GET /invoices/:id.pdf"| api["Invoice API"]
  api --> totals["Totals calculator"]
  totals --> render["PDF renderer"]
  render --> page
` + "```" + `

## Key files
- ` + "`billing/totals.go`" + ` computes line totals and tax once
- ` + "`billing/pdf.go`" + ` lays out the PDF

## Decisions
- Money is stored as integer cents. No floating point anywhere in the totals.
`},
	},
	{
		Ref: "csvexport", Title: "Export a report to CSV", State: "Done", Priority: 3, Epic: 2,
		Labels: []string{"feature", "reports"},
		Desc:   "Any report can be downloaded as a CSV file.",
		Criteria: []crit{
			{"Export button on every report", true},
			{"CSV opens cleanly in a spreadsheet, with a header row", true},
			{"Large reports stream instead of loading into memory", true},
		},
		Comments: []comment{{"ai", "Streams rows straight from the query to the response."}},
		Commits:  []string{"Stream report rows as CSV"},
		Doc: &doc{Title: "CSV export", Type: "feature", Body: `## Summary
Every report has an Export button. The file streams from the database, so big reports do not use much memory.

## How it works
` + "```mermaid" + `
sequenceDiagram
  participant U as User
  participant A as Report API
  participant D as Database
  U->>A: GET /reports/42.csv
  A->>D: Open row cursor
  loop For each batch
    D-->>A: 500 rows
    A-->>U: CSV chunk
  end
  A-->>U: End of file
` + "```" + `

## Key files
- ` + "`reports/export.go`" + ` writes the CSV header and rows

## Decisions
- Batches of 500 rows keep memory flat for any report size.
`},
	},
	{
		Ref: "welcomemail", Title: "Welcome email after verification", State: "Done", Priority: 4, Epic: 0,
		Labels: []string{"chore", "onboarding"},
		Desc:   "Send one friendly email when an account is verified.",
		Criteria: []crit{
			{"Email is sent once, right after verification", true},
			{"Email links to the first-run checklist", true},
		},
		Commits: []string{"Send welcome email after verification"},
	},
	{
		Ref: "taxfix", Title: "Fix rounding of tax on small invoices", State: "Done", Priority: 2, Epic: 1,
		Labels: []string{"bug", "billing"},
		Desc:   "A 0.5 cent rounding error showed up on invoices under 10.00.",
		Criteria: []crit{
			{"A regression test covers the 9.99 invoice", true},
			{"Tax rounds half up on the invoice total, not per line", true},
		},
		Comments: []comment{{"ai", "Rounding now happens once on the total. Added the 9.99 case as a test."}},
		Commits:  []string{"Round tax once on the invoice total"},
	},

	// ---- In Review ----
	{
		Ref: "passwordreset", Title: "Password reset by email", State: "In Review", Priority: 2, Epic: 0,
		Labels: []string{"feature", "onboarding"},
		Desc:   "A user who forgot a password gets a reset link by email.",
		Criteria: []crit{
			{"Reset request never reveals whether an email has an account", true},
			{"Reset link works once and expires in 1 hour", true},
			{"Changing the password signs out other sessions", true},
		},
		Comments: []comment{{"ai", "Ready for review. The request always answers with the same message, so account emails cannot be guessed."}},
		Commits:  []string{"Add password reset by email", "Sign out other sessions on password change"},
	},
	{
		Ref: "plancheckout", Title: "Upgrade plan from the billing page", State: "In Review", Priority: 1, Epic: 1,
		Labels: []string{"feature", "billing"},
		Desc:   "A customer picks a bigger plan and pays for it without leaving Acme.",
		Criteria: []crit{
			{"Plan picker shows price per month and per year", true},
			{"Payment failure keeps the old plan active", true},
			{"Receipt email is sent after a successful payment", true},
			{"Upgrade shows up on the next invoice, prorated", true},
		},
		Comments: []comment{{"ai", "All four checks are ticked. The failure path is covered by a test with a declined card."}},
		Commits:  []string{"Add plan upgrade checkout", "Prorate upgrades on the next invoice"},
	},
	{
		Ref: "weeklydigest", Title: "Weekly usage digest email", State: "In Review", Priority: 3, Epic: 2,
		Labels: []string{"feature", "reports"},
		Desc:   "Owners get a short usage summary every Monday morning.",
		Criteria: []crit{
			{"Digest is sent Monday 08:00 in the account time zone", true},
			{"Digest lists the top 5 pages by visits", true},
			{"Every email has a one-click unsubscribe link", true},
		},
		Comments: []comment{{"ai", "Scheduler and template are in. Unsubscribe uses a signed link, no login needed."}},
		Commits:  []string{"Send weekly usage digest"},
	},

	// ---- In Progress ----
	{
		Ref: "importwizard", Title: "Import contacts wizard", State: "In Progress", Priority: 2, Epic: 0,
		Labels: []string{"feature", "onboarding"},
		Desc:   "A three-step wizard to import contacts from a CSV file.",
		Criteria: []crit{
			{"Step 1 accepts a CSV up to 10 MB", true},
			{"Step 2 maps columns to fields, with a preview", false},
			{"Step 3 shows how many rows were imported and skipped", false},
		},
		Comments: []comment{{"ai", "Upload step is done. Working on the column mapping preview."}},
	},
	{
		Ref: "tax", Title: "Support VAT numbers on invoices", State: "In Progress", Priority: 2, Epic: 1,
		Labels: []string{"feature", "billing"},
		Desc:   "Business customers add a VAT number that appears on every invoice.",
		Criteria: []crit{
			{"VAT number field on the billing details form", true},
			{"Number is validated against the country format", false},
			{"Number appears on the invoice screen and PDF", false},
		},
	},
	{
		Ref: "chartspeed", Title: "Speed up the traffic chart", State: "In Progress", Priority: 3, Epic: 2,
		Labels: []string{"tech-debt", "reports"},
		Desc:   "The traffic chart takes about 4 seconds on large accounts. Target: under 1 second.",
		Criteria: []crit{
			{"Daily totals come from a rollup table", false},
			{"Chart loads in under 1 second on the large test account", false},
		},
	},

	// ---- Blocked ----
	{
		Ref: "sso", Title: "Single sign-on with Google", State: "Blocked", Priority: 3, Epic: 0,
		Labels: []string{"feature", "onboarding"}, Blocked: []string{"importwizard"},
		Desc: "Let users sign in with a Google account. Waits on the import wizard so the sign-in screen is redesigned only once.",
		Criteria: []crit{
			{"Google button on the sign-in and sign-up screens", false},
			{"First Google sign-in creates a verified account", false},
		},
		Comments: []comment{{"ai", "Waiting on the import wizard. It changes the same sign-in layout."}},
	},
	{
		Ref: "taxreport", Title: "Tax summary report by month", State: "Blocked", Priority: 3, Epic: 2,
		Labels: []string{"feature", "reports"}, Blocked: []string{"tax"},
		Desc: "A monthly report of tax collected, per country. Needs VAT numbers first.",
		Criteria: []crit{
			{"Report groups tax by country and month", false},
			{"Report can be exported to CSV", false},
		},
		Comments: []comment{{"ai", "Cannot start until VAT numbers are stored. Picking this up after that ships."}},
	},

	// ---- Ready ----
	{
		Ref: "dunning", Title: "Retry failed payments", State: "Ready", Priority: 2, Epic: 1,
		Labels: []string{"feature", "billing"},
		Desc:   "Retry a failed card payment after 1, 3 and 7 days, then pause the account.",
		Criteria: []crit{
			{"Three retries at 1, 3 and 7 days", false},
			{"Customer gets an email after each failed try", false},
			{"Account pauses after the last failure", false},
		},
	},
	{
		Ref: "emptystate", Title: "Friendly empty states on the dashboard", State: "Ready", Priority: 4, Epic: 0,
		Labels: []string{"chore", "onboarding"},
		Desc:   "New accounts see a helpful hint instead of an empty chart.",
		Criteria: []crit{
			{"Each dashboard card has an empty state with one next action", false},
			{"Empty state disappears after the first data point", false},
		},
	},
	{
		Ref: "scheduleexport", Title: "Schedule a report export", State: "Ready", Priority: 3, Epic: 2,
		Labels: []string{"feature", "reports"},
		Desc:   "Send a CSV of a report to an email address every week or month.",
		Criteria: []crit{
			{"User picks weekly or monthly and a recipient", false},
			{"The CSV is attached to the scheduled email", false},
		},
	},

	// ---- Aligning ----
	{
		Ref: "teams", Title: "Team members and roles", State: "Aligning", Priority: 2, Epic: 0,
		Labels: []string{"feature", "onboarding"},
		Desc:   "Invite colleagues to an account. Open question: do we need more than Owner and Member?",
		Criteria: []crit{
			{"Owner can invite by email", false},
			{"Members cannot see billing", false},
			{"Roles are listed in the settings page", false},
		},
		Comments: []comment{{"human", "Keep it to two roles for now. We can add more when customers ask."}},
	},
	{
		Ref: "coupons", Title: "Discount codes", State: "Aligning", Priority: 4, Epic: 1,
		Labels: []string{"feature", "billing"},
		Desc:   "Percentage and fixed-amount codes at checkout.",
	},

	// ---- Backlog ----
	{Ref: "dark", Title: "Dark mode for the web app", State: "Backlog", Priority: 4, Epic: 0, Labels: []string{"feature"}},
	{Ref: "multicurrency", Title: "Bill in euros and pounds", State: "Backlog", Priority: 3, Epic: 1, Labels: []string{"feature", "billing"}},
	{Ref: "pdfreport", Title: "Print-friendly report layout", State: "Backlog", Priority: 4, Epic: 2, Labels: []string{"chore", "reports"}},

	// ---- Triage ----
	{Ref: "slowlogin", Title: "Sign-in sometimes takes 5 seconds", State: "Triage", Priority: 0, Epic: -1, Labels: []string{"bug", "needs-info"},
		Desc: "Reported by one customer. No steps to reproduce yet."},
	{Ref: "webhooks", Title: "Webhooks for new invoices", State: "Triage", Priority: 0, Epic: -1, Labels: []string{"feature", "needs-triage"}},

	// ---- Canceled ----
	{Ref: "chatwidget", Title: "In-app chat widget", State: "Canceled", Priority: 4, Epic: 0, Labels: []string{"feature", "wontfix"},
		Desc:     "Dropped: customers prefer email, and the widget slows page load.",
		Comments: []comment{{"human", "Not worth the page weight. Canceling."}}},
}

// Result counts what Seed created.
type Result struct {
	WorkspaceSlug string
	Epics         int
	Issues        int
	InReview      int
	Documents     int
}

// Seed creates the demo workspace for ownerID. It returns ErrExists, changing
// nothing, when a workspace with that slug is already there.
func Seed(ctx context.Context, svc *service.Service, ownerID, slug string) (Result, error) {
	if slug == "" {
		slug = Slug
	}
	st := svc.Store
	if _, err := st.ResolveWorkspace(ctx, slug); err == nil {
		return Result{}, ErrExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return Result{}, err
	}
	// ResolveWorkspace also matches by prefix, so a clash on the prefix shows up
	// as a conflict from CreateWorkspace below.
	ws, err := st.CreateWorkspace(ctx, Name, slug, prefix, ownerID)
	if err != nil {
		return Result{}, fmt.Errorf("create demo workspace: %w", err)
	}
	res := Result{WorkspaceSlug: ws.Slug}

	for _, l := range domainLabels {
		if _, err := st.CreateLabel(ctx, ws.ID, l, "#7C8698", "domain"); err != nil {
			return res, err
		}
	}
	ini, err := st.SaveInitiative(ctx, ws.ID, models.Initiative{Name: initiativeName, DescriptionMD: initiativeDesc})
	if err != nil {
		return res, err
	}
	var epicIDs []string
	for i, e := range epics {
		p, err := st.SaveProject(ctx, ws.ID, models.Project{InitiativeID: &ini.ID, Name: e.Name, DescriptionMD: e.Desc, Position: i})
		if err != nil {
			return res, err
		}
		epicIDs = append(epicIDs, p.ID)
		res.Epics++
	}

	keys := map[string]string{} // ref -> issue key
	ids := map[string]string{}  // ref -> issue id
	for _, it := range issues {
		in := store.IssueInput{
			Title: it.Title, DescriptionMD: it.Desc, Priority: it.Priority,
			LabelNames: it.Labels,
		}
		if it.Epic >= 0 {
			id := epicIDs[it.Epic]
			in.ProjectID = &id
		}
		gated := store.IsGatedState(it.State)
		// A gated issue is created In Progress, gets its checklist, then moves, so
		// the gate sees the ticked criteria.
		if gated {
			in.StateName = "In Progress"
		} else {
			in.StateName = it.State
		}
		created, err := svc.CreateIssue(ctx, ws.ID, in, "human")
		if err != nil {
			return res, fmt.Errorf("issue %q: %w", it.Title, err)
		}
		keys[it.Ref], ids[it.Ref] = created.Key, created.ID
		res.Issues++

		for _, c := range it.Criteria {
			cr, err := st.AddCriterion(ctx, ws.ID, created.ID, c.Text, "", nil)
			if err != nil {
				return res, err
			}
			if c.Done {
				done := true
				if _, err := st.UpdateCriterion(ctx, ws.ID, cr.ID, nil, &done, nil, nil, nil); err != nil {
					return res, err
				}
			}
		}
		for _, msg := range it.Commits {
			sha := fmt.Sprintf("%x", sha1.Sum([]byte(created.Key+msg)))
			if _, err := st.AddCommit(ctx, ws.ID, created.ID, sha, msg, nil); err != nil {
				return res, err
			}
		}
		if gated {
			// The AI moves work to In Review. The human approves it to Done.
			actor := "ai"
			if _, err := svc.UpdateIssue(ctx, ws.ID, created.ID, store.IssuePatch{StateName: strp("In Review")}, actor); err != nil {
				return res, fmt.Errorf("move %s to In Review: %w", created.Key, err)
			}
			if it.State == "Done" {
				if _, err := svc.UpdateIssue(ctx, ws.ID, created.ID, store.IssuePatch{StateName: strp("Done")}, "human"); err != nil {
					return res, fmt.Errorf("move %s to Done: %w", created.Key, err)
				}
			} else {
				res.InReview++
			}
		}
		for _, c := range it.Comments {
			if _, err := svc.AddComment(ctx, ws.ID, created.ID, c.Body, c.Actor); err != nil {
				return res, err
			}
		}
		if it.Doc != nil {
			id := created.ID
			if _, err := st.SaveDocument(ctx, ws.ID, models.Document{Title: it.Doc.Title, Type: it.Doc.Type, BodyMD: it.Doc.Body, Author: "ai", IssueID: &id}); err != nil {
				return res, err
			}
			res.Documents++
		}
	}

	// Blockers last, once every key exists.
	for _, it := range issues {
		if len(it.Blocked) == 0 {
			continue
		}
		var refs []string
		for _, b := range it.Blocked {
			refs = append(refs, keys[b])
		}
		if _, err := st.SetBlockers(ctx, ws.ID, ids[it.Ref], refs); err != nil {
			return res, fmt.Errorf("blockers for %s: %w", keys[it.Ref], err)
		}
	}
	return res, nil
}

func strp(s string) *string { return &s }
