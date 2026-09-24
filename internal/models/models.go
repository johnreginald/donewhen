// Package models holds the core domain types shared across the app.
package models

import (
	"encoding/json"
	"time"
)

// State categories drive board grouping and "what counts as done".
const (
	CatTriage    = "triage"
	CatBacklog   = "backlog"
	CatUnstarted = "unstarted"
	CatStarted   = "started"
	CatCompleted = "completed"
	CatCanceled  = "canceled"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

// Workspace is the top of the hierarchy and the tenancy boundary:
// Workspace → Project (initiative) → Epic (project) → Issue. Everything a
// request can read or write is scoped to exactly one of these.
type Workspace struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	KeyPrefix string    `json:"keyPrefix"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Workspace membership roles, most privileged first.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// CanAdmin reports whether a role may change a workspace or its membership.
func CanAdmin(role string) bool { return role == RoleOwner || role == RoleAdmin }

// Membership is a workspace as seen by one user, carrying their role in it.
type Membership struct {
	Workspace
	Role string `json:"role"`
}

// Member is one person's access to a workspace.
type Member struct {
	UserID    string    `json:"userId"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

type Initiative struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DescriptionMD string    `json:"descriptionMd"`
	Status        string    `json:"status"`
	Position      int       `json:"position"`
	RepoURL       *string   `json:"repoUrl"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Project struct {
	ID            string    `json:"id"`
	InitiativeID  *string   `json:"initiativeId"`
	Name          string    `json:"name"`
	DescriptionMD string    `json:"descriptionMd"`
	Status        string    `json:"status"`
	Position      int       `json:"position"`
	RepoURL       *string   `json:"repoUrl"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type WorkflowState struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Position int    `json:"position"`
	Color    string `json:"color"`
}

type LabelGroup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Exclusive bool   `json:"exclusive"`
}

type Label struct {
	ID      string  `json:"id"`
	GroupID *string `json:"groupId"`
	Name    string  `json:"name"`
	Color   string  `json:"color"`
}

type Issue struct {
	ID            string    `json:"id"`
	WorkspaceID   string    `json:"workspaceId"`
	Number        int       `json:"number"`
	Key           string    `json:"key"` // e.g. K-42
	Title         string    `json:"title"`
	DescriptionMD string    `json:"descriptionMd"`
	StateID       string    `json:"stateId"`
	ProjectID     *string   `json:"projectId"`
	AssigneeID    *string   `json:"assigneeId"`
	AgentID       *string   `json:"agentId"`  // the agent this ticket is for, if any
	Priority      int       `json:"priority"` // 0 none,1 urgent,2 high,3 medium,4 low
	Position      float64   `json:"position"` // ordering within a state column
	Labels        []Label   `json:"labels"`
	DocCount      int       `json:"docCount"`   // attached documents (implementation coverage)
	ParentKey     *string   `json:"parentKey"`  // epic this issue belongs to (nil = top-level)
	ChildCount    int       `json:"childCount"` // sub-issues (>0 ⇒ this is an epic)
	GitBranch     *string   `json:"gitBranch"`  // the branch that implemented this
	PRURL         *string   `json:"prUrl"`      // the pull request
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// InboxItem is a "needs review" entry — an issue the AI moved to In Review —
// with a small summary for the review card.
type InboxItem struct {
	Issue
	CommitCount     int        `json:"commitCount"`
	EnteredReviewAt *time.Time `json:"enteredReviewAt"` // when the AI moved it to In Review
}

// IssueCommit links an issue to a commit that implemented it (the record of how).
type IssueCommit struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issueId"`
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	URL       *string   `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

// Criterion kinds. A criterion says not just what "done" means but how that is
// verified, so an orchestrator can tick it from evidence instead of assertion.
const (
	// CriterionManual is ticked by a human. Legacy criteria are this.
	CriterionManual = "manual"
	// CriterionDeterministic runs a command: {"cmd":"go test ./...","expect_exit":0}
	CriterionDeterministic = "deterministic"
	// CriterionPolicy inspects the diff: {"policy":"paths_within","args":["src/**"]}
	CriterionPolicy = "policy"
	// CriterionJudgment asks a model: {"prompt":"...","model":"..."}. Advisory —
	// never the sole gate on moving an issue to Done.
	CriterionJudgment = "judgment"
)

// Criterion is one done-when acceptance item.
type Criterion struct {
	ID       string `json:"id"`
	IssueID  string `json:"issueId"`
	Body     string `json:"body"`
	Done     bool   `json:"done"`
	Position int    `json:"position"`
	// Kind is one of the Criterion* constants above.
	Kind string `json:"kind"`
	// CheckSpec describes how Kind is verified; nil for manual criteria.
	CheckSpec json.RawMessage `json:"checkSpec,omitempty"`
	// EvidenceRef points at what verified this criterion, once it has been.
	EvidenceRef *string   `json:"evidenceRef,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Comment struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issueId"`
	BodyMD    string    `json:"bodyMd"`
	Actor     string    `json:"actor"`   // human | ai
	AgentID   *string   `json:"agentId"` // the agent that wrote it, when one did
	CreatedAt time.Time `json:"createdAt"`
}

type Document struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	BodyMD       string    `json:"bodyMd"`
	Type         string    `json:"type"`   // feature | change | decision | reference | overview
	Author       string    `json:"author"` // ai | human
	ProjectID    *string   `json:"projectId"`
	InitiativeID *string   `json:"initiativeId"`
	IssueID      *string   `json:"issueId"`
	Labels       []Label   `json:"labels"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type PushSubscription struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// Activity is one persisted record in the timeline — who (human/ai) did what,
// when. issueKey/issueTitle are snapshots so the log survives issue deletion.
type Activity struct {
	ID         string    `json:"id"`
	IssueID    *string   `json:"issueId"`
	IssueKey   string    `json:"issueKey"`
	IssueTitle string    `json:"issueTitle"`
	Actor      string    `json:"actor"` // human | ai
	Kind       string    `json:"kind"`
	Field      string    `json:"field,omitempty"`
	FromVal    string    `json:"from,omitempty"`
	ToVal      string    `json:"to,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Run is one agent attempt at a ticket, recorded by whichever machine ran it.
type Run struct {
	ID          string     `json:"id"`
	IssueID     *string    `json:"issueId"`
	IssueKey    string     `json:"issueKey,omitempty"`
	IssueTitle  string     `json:"issueTitle,omitempty"`
	AgentID     *string    `json:"agentId"`
	Kind        string     `json:"kind"` // work | chat
	Runner      string     `json:"runner"`
	Model       string     `json:"model"`
	Attempt     int        `json:"attempt"`
	Status      string     `json:"status"` // queued | running | succeeded | failed | aborted
	Verdict     string     `json:"verdict,omitempty"`
	SessionID   string     `json:"sessionId,omitempty"`
	ExitCode    *int       `json:"exitCode"`
	AgentError  string     `json:"agentError,omitempty"`
	Tokens      RunTokens  `json:"tokens"`
	CostUSD     float64    `json:"costUsd"`
	NotionalUSD float64    `json:"notionalCostUsd"`
	Billing     string     `json:"billing"` // subscription | api | unknown
	DeniedTools []string   `json:"deniedTools"`
	LogTail     string     `json:"logTail,omitempty"`
	Host        string     `json:"host,omitempty"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
}

// RunTokens is a run's token usage by kind.
type RunTokens struct {
	Input         int `json:"input"`
	CacheRead     int `json:"cacheRead"`
	CacheCreation int `json:"cacheCreation"`
	Output        int `json:"output"`
	Total         int `json:"total"`
}

// Run statuses.
const (
	RunQueued    = "queued"
	RunRunning   = "running"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
	RunAborted   = "aborted"
)

// Agent is a configured worker: a harness, a model, and how it should behave.
type Agent struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Role           string    `json:"role"`
	Harness        string    `json:"harness"` // claude | codex | opencode
	Model          string    `json:"model"`
	Effort         string    `json:"effort"`
	InstructionsMD string    `json:"instructionsMd"`
	AllowedTools   []string  `json:"allowedTools"`
	MaxTurns       int       `json:"maxTurns"`
	Status         string    `json:"status"` // active | paused
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Harnesses an agent can run on.
var Harnesses = []string{"claude", "codex", "opencode"}

// RunnerHost is a machine that runs agents, as it last reported itself.
type RunnerHost struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Harnesses  []HarnessStatus `json:"harnesses"`
	Version    string          `json:"version"`
	LastSeenAt time.Time       `json:"lastSeenAt"`
}

// HarnessStatus is what a host proved about one harness: installed, signed
// in, on which kind of plan, and what to do when it is not.
type HarnessStatus struct {
	Harness   string    `json:"harness"`
	Installed bool      `json:"installed"`
	Ready     bool      `json:"ready"`
	Auth      string    `json:"auth,omitempty"` // e.g. "claude.ai subscription", "ChatGPT", "API key"
	Detail    string    `json:"detail,omitempty"`
	Fix       string    `json:"fix,omitempty"` // the command that would make it ready
	Models    []string  `json:"models,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

// Job is work Raenil has queued for a runner host.
type Job struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"` // test_env | run_ticket | chat
	AgentID    *string         `json:"agentId"`
	Input      json.RawMessage `json:"input"`
	IssueID    *string         `json:"issueId"`
	IssueKey   string          `json:"issueKey,omitempty"`
	Status     string          `json:"status"` // queued | claimed | succeeded | failed | canceled
	Host       string          `json:"host,omitempty"`
	Result     json.RawMessage `json:"result"`
	Error      string          `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
	ClaimedAt  *time.Time      `json:"claimedAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
}

// Interaction is something an agent asked of a human on a ticket.
type Interaction struct {
	ID         string          `json:"id"`
	IssueID    string          `json:"issueId"`
	AgentID    *string         `json:"agentId"`
	Kind       string          `json:"kind"`    // questions | proposal
	Payload    json.RawMessage `json:"payload"` // Questions or Proposal
	Status     string          `json:"status"`  // open | answered | approved | rejected | canceled
	Response   json.RawMessage `json:"response"`
	CreatedAt  time.Time       `json:"createdAt"`
	ResolvedAt *time.Time      `json:"resolvedAt"`
}

// Question is one structured question an agent asks.
type Question struct {
	ID         string   `json:"id"`
	Text       string   `json:"text"`
	Options    []string `json:"options,omitempty"`
	Multi      bool     `json:"multi,omitempty"`      // choose several
	AllowOther bool     `json:"allowOther,omitempty"` // free text besides the options
}

// Answer is a human's answer to one Question.
type Answer struct {
	QuestionID string   `json:"questionId"`
	Choices    []string `json:"choices,omitempty"`
	Other      string   `json:"other,omitempty"`
}

// AgentSession is an agent's resumable session on one ticket's conversation.
type AgentSession struct {
	AgentID   string    `json:"agentId"`
	IssueID   string    `json:"issueId"`
	SessionID string    `json:"sessionId"`
	Cwd       string    `json:"cwd"`
	Turns     int       `json:"turns"`
	UpdatedAt time.Time `json:"updatedAt"`
}
