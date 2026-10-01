# Getting started

This is the short path for a developer who already uses Claude Code. You run DoneWhen, connect Claude to it, give Claude the rules, and work through tickets. Each step links to the full page.

## 1. Run it

You need Docker with Compose, and Git.

```bash
git clone https://github.com/johnreginald/donewhen.git && cd donewhen
cp .env.example .env
```

Edit `.env`:
- Put `openssl rand -hex 32` in `DONEWHEN_SESSION_SECRET`.
- Change `POSTGRES_PASSWORD`.
- For a local test, set `DONEWHEN_BASE_URL=http://localhost:8090` and keep `DONEWHEN_ENV=dev`.

```bash
docker compose up -d --build
docker compose exec donewhen /app/donewhen user you@example.com 'a-strong-password'
```

Open <http://localhost:8090>, sign in and create a workspace.

For a real server with HTTPS, read [SELF-HOSTING.md](SELF-HOSTING.md). For Podman, read [PODMAN.md](PODMAN.md).

## 2. Connect Claude Code

Make a token. The server shows the token one time.

```bash
docker compose exec donewhen /app/donewhen token claude
```

Register the MCP server. Use the address of your own server.

```bash
claude mcp add --transport http donewhen http://localhost:8090/mcp \
  --header "Authorization: Bearer <token>"
```

The tools now show as `mcp__donewhen__*`. To limit an agent to one workspace, make a pinned token: `donewhen token <name> <workspace-slug>`.

Add the skill. It teaches Claude how to write tickets and done-when lists.

```bash
ln -s "$(pwd)/skills/donewhen" ~/.claude/skills/donewhen
```

Optional: add the plugin for `/donewhen:tasks`. It is a menu to browse tickets from Claude Code. The setup is in the [README](../README.md#the-plugin-donewhentasks).

## 3. Give Claude the rules

Copy [templates/CLAUDE.md](templates/CLAUDE.md) into the `CLAUDE.md` of your project. For other agents (Codex and similar), copy [templates/AGENTS.md](templates/AGENTS.md) into `AGENTS.md`. The text is the same. Fill in the three placeholder lines at the end.

These rules do three things:
- Claude asks you before it creates a ticket.
- Claude writes a done-when checklist on every ticket.
- Claude stops at In Review. You merge and move the ticket to Done.

The server also enforces the gate. A ticket cannot move to In Review or Done while an item is not ticked. This applies to every agent.

## 4. Work with it

1. **Plan.** Talk with Claude about the feature. Ask Claude to write the spec.
2. **Tickets.** Ask Claude to turn the spec into tickets. Claude proposes titles and epics. You confirm. Claude creates the tickets, each with a done-when list.
3. **Build.** Point an agent at a ticket: "Do PP-12." The agent moves the ticket to In Progress and builds. It ticks each item when the item is met. For a long run, use `/goal`. The agent then works through the list.
4. **Review.** Finished tickets wait in **In Review**, and in the **Inbox** in the web app. Read the code. Ask for changes, or merge.
5. **Close.** Move the ticket to Done. The record stays: the commit, the branch and a short engineering document.

If an agent cannot continue without you, the ticket moves to **Blocked** with the reason. Answer in a comment and move the ticket back.

[AI-WORKFLOW.md](AI-WORKFLOW.md) has the details of one ticket. [CONCEPTS.md](CONCEPTS.md) has the vocabulary.

## Keys in the web app

| Key | Action |
|---|---|
| `N` | New issue (quick capture) |
| `⌘K` | Search and command palette |
| `G` then `B` / `I` / `L` | Go to Board / Inbox / List |
| `J` / `K` | Next and previous issue |
| `S` `P` `L` `E` | Change status, priority, labels, epic of the focused issue |
| `?` | Show all keys |

## If something does not work

- **The tools do not appear.** Run `claude mcp list`. The server must say connected. Check the token and the `/mcp` address.
- **The server is unreachable.** Check `docker compose ps`. Check `DONEWHEN_BASE_URL`.
- **Login loops in `prod`.** Cookies need HTTPS. Use a proxy with a certificate. For a local test, set `DONEWHEN_ENV=dev`.
