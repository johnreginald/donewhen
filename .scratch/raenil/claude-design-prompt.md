# Claude Design prompt — Raenil

Paste everything below the line into Claude Design.

---

## Design a UI for **Raenil** — a self-hosted, single-user issue tracker (a personal Linear)

**Raenil** ("Linear" reversed) is a self-hosted project tracker for **one solo developer who works alongside an AI agent (Claude)**. It already exists and works; I want a **fresh, high-fidelity visual design + component system** for it — Linear-caliber craft, but its own identity. Design for **dark theme first**, with a light theme as a second pass. Every screen must work on **desktop and on a phone (installed PWA)**.

### Who it's for & the feel
- A single power user. No teams, no permissions, no onboarding funnels.
- **Keyboard-first, dense, fast, calm.** Muscle-memory over discoverability. Think Linear/Height: quiet surfaces, one strong accent, crisp type, generous-but-tight spacing, motion that's subtle and instant.
- The AI is a first-class actor: when Claude creates or moves an issue it appears live and is attributed to "AI". Design should make human-vs-AI activity legible without being noisy.

### Product model (design must reflect this exact structure)
- **Hierarchy:** Initiatives → Projects (epics) → Issues. Issues have comments. There are also standalone **Documents** (markdown).
- **Workflow states (Kanban columns, in order):** `Triage · Backlog · Aligning · Ready · In Progress · In Review · Done · Canceled`. Each has a color + category (Aligning/Ready = unstarted, In Review = started, etc.). This is continuous-flow Kanban — **no sprints, estimates, or story points anywhere.**
- **Issue fields:** key (e.g. `R-42`), title, markdown description (may contain **```mermaid diagrams that render inline**), state, project, priority (No priority/Urgent/High/Medium/Low), and **labels grouped into exclusive groups** (`repo`, `platform`, `type`, `domain`, `triage` — one label max per group).
- **Comments** carry an actor: `human` or `ai`.

### Screens to design (desktop + mobile for each)
1. **Login / first-run setup** — single account. Minimal card. (Setup variant = "Create your account".)
2. **Board (primary)** — horizontal Kanban, one column per workflow state, draggable issue cards. Card shows key, title, label pills, priority indicator. Column header shows name + count + state color dot. Left **sidebar**: brand, ⌘K search entry, nav (Board / List / Documents / Settings), and a **Projects tree grouped under Initiatives** + an "All issues" item.
3. **List view** — dense table of issues (key, priority, title, status pill, project, labels), grouped/sorted by state.
4. **Issue detail — a right-side drawer** over the board: editable title; Status / Priority / Project selectors; label chips with an add-picker; markdown description with an **edit ⇄ preview toggle** and **rendered mermaid** (with a per-diagram code/diagram toggle); a comments thread (human/ai attributed) + composer; delete.
5. **Documents** — list + markdown editor + rendered view (mermaid supported), like a lightweight wiki.
6. **Command palette (⌘K)** — modal: search issues, quick "create issue: …", and navigate. Keyboard-driven (↑/↓/↵), result rows tagged by kind (issue / nav / action).
7. **Settings** — three cards: **Notifications** (toggle push-to-this-device), **MCP endpoint** (shows the URL + how to auth), **API tokens** (create → reveal-once secret, list, revoke).
8. **System bits** — toast/snackbar (e.g. "R-42 → In Review (by AI)"), empty states, loading states, hover/focus states, the PWA install affordance, and the **notification** look (a status-move push: title `R-42 → In Review`, body `<title> (moved by AI)`).

### Interactions to account for in the visuals
- Drag-and-drop cards between columns (define the drag/placeholder/drop-target styling).
- **Live updates** (SSE): cards can appear/move/update in real time — design a gentle "just changed" affordance.
- Deep-linking to an issue opens the drawer over the board.
- Responsive: sidebar collapses to a hamburger drawer on phones; board columns become swipeable/near-full-width; the issue drawer becomes a full-screen sheet.

### Brand direction (refine, don't just accept)
- Name **Raenil**; wordmark + a compact **"R" app-icon mark** (used as favicon + PWA icon, must read at 48px and as a maskable icon).
- Current accent is indigo `#6e79f1` on a near-black UI (`#0d0e12` bg, elevated `#16171d`/`#1c1e26`, borders `#262832`). Treat these as a starting point — **propose a refined palette** (accent + neutrals + the 8 state colors + priority colors + label colors) that's cohesive and accessible in **both dark and light**. Provide the type scale (system/Inter-style), spacing, radius, and elevation tokens as **CSS custom properties** (the app themes via CSS variables).

### What already exists (so this is a redesign, not a blank slate)
- Working app: **SvelteKit (Svelte 5) SPA** talking to a Go REST API; dark UI, indigo accent, board + list + issue drawer + ⌘K + docs + settings all functional, PWA-installable, live SSE, mermaid rendering in the drawer. It looks clean but generic — I want a **more distinctive, polished visual identity and a tighter component system** while keeping the same information architecture and data model above.
- Please deliver: the **component library** (buttons, inputs/selects, label & status pills, priority icon, cards, drawer, modal, table rows, toast, nav items, tokens/palette) and the **8 screens** in dark + light, desktop + mobile, including empty/loading/hover/focus states. Keep it buildable in Svelte + CSS variables.

**Constraints:** dark-first; WCAG-AA contrast; no stock illustrations; no marketing/landing page — this is the app itself. Optimize for information density and speed, the way Linear does.
