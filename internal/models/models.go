// Package models holds the core domain types shared across the app.
package models

import "time"

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
	ID            string     `json:"id"`
	InitiativeID  *string    `json:"initiativeId"`
	Name          string     `json:"name"`
	DescriptionMD string     `json:"descriptionMd"`
	Status        string     `json:"status"`
	Position      int        `json:"position"`
	RepoURL       *string    `json:"repoUrl"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
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
	Number        int       `json:"number"`
	Key           string    `json:"key"` // e.g. K-42
	Title         string    `json:"title"`
	DescriptionMD string    `json:"descriptionMd"`
	StateID       string    `json:"stateId"`
	ProjectID     *string   `json:"projectId"`
	AssigneeID    *string   `json:"assigneeId"`
	Priority      int       `json:"priority"` // 0 none,1 urgent,2 high,3 medium,4 low
	Position      float64   `json:"position"` // ordering within a state column
	Labels        []Label   `json:"labels"`
	DocCount      int       `json:"docCount"`     // attached documents (implementation coverage)
	ParentKey     *string   `json:"parentKey"`    // epic this issue belongs to (nil = top-level)
	ChildCount    int       `json:"childCount"`   // sub-issues (>0 ⇒ this is an epic)
	GitBranch     *string   `json:"gitBranch"`    // the branch that implemented this
	PRURL         *string   `json:"prUrl"`        // the pull request
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
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

// Criterion is one done-when acceptance item.
type Criterion struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issueId"`
	Body      string    `json:"body"`
	Done      bool      `json:"done"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

type Comment struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issueId"`
	BodyMD    string    `json:"bodyMd"`
	Actor     string    `json:"actor"` // human | ai
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
