-- Core schema for DoneWhen.

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id         text PRIMARY KEY,               -- opaque random token
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions(user_id);

CREATE TABLE api_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,
    hash         text NOT NULL UNIQUE,          -- sha256 of the bearer token
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

CREATE TABLE initiatives (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name           text NOT NULL,
    description_md text NOT NULL DEFAULT '',
    status         text NOT NULL DEFAULT 'active',
    position       int  NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    initiative_id  uuid REFERENCES initiatives(id) ON DELETE SET NULL,
    name           text NOT NULL,
    description_md text NOT NULL DEFAULT '',
    status         text NOT NULL DEFAULT 'active',
    position       int  NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX projects_initiative_idx ON projects(initiative_id);

CREATE TABLE workflow_states (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name     text NOT NULL UNIQUE,
    category text NOT NULL,
    position int  NOT NULL DEFAULT 0,
    color    text NOT NULL DEFAULT '#94a3b8'
);

CREATE TABLE label_groups (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name      text NOT NULL UNIQUE,
    exclusive boolean NOT NULL DEFAULT true
);

CREATE TABLE labels (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid REFERENCES label_groups(id) ON DELETE SET NULL,
    name     text NOT NULL,
    color    text NOT NULL DEFAULT '#94a3b8',
    UNIQUE (name)
);
CREATE INDEX labels_group_idx ON labels(group_id);

CREATE SEQUENCE issue_number_seq START 1;

CREATE TABLE issues (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number         int  NOT NULL UNIQUE,
    key            text NOT NULL UNIQUE,
    title          text NOT NULL,
    description_md text NOT NULL DEFAULT '',
    state_id       uuid NOT NULL REFERENCES workflow_states(id),
    project_id     uuid REFERENCES projects(id) ON DELETE SET NULL,
    assignee_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    priority       int  NOT NULL DEFAULT 0,
    position       double precision NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX issues_state_idx ON issues(state_id);
CREATE INDEX issues_project_idx ON issues(project_id);

CREATE TABLE issue_labels (
    issue_id uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    label_id uuid NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (issue_id, label_id)
);

CREATE TABLE comments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id   uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    body_md    text NOT NULL,
    actor      text NOT NULL DEFAULT 'human',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX comments_issue_idx ON comments(issue_id);

CREATE TABLE documents (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title      text NOT NULL,
    body_md    text NOT NULL DEFAULT '',
    project_id uuid REFERENCES projects(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE push_subscriptions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   text NOT NULL UNIQUE,
    p256dh     text NOT NULL,
    auth       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX push_subscriptions_user_idx ON push_subscriptions(user_id);
