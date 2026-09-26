package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A terminal run works a ticket in a live terminal the user can watch and
// answer, instead of headless. The session lives in tmux, so closing the
// window does not stop the work and `tmux attach` opens it again; a cmux
// workspace or a Terminal window attaches to it for the user to see.
//
// The harness reports each turn's end by appending a JSON line to a file —
// Claude through a Stop hook, Codex through notify — carrying the agent's last
// message. The turn loop (runTerminalTurns) decides from it what happens next:
// wait for the user, type the check failures back in, or close.

// TerminalRounds is how many times failing checks are typed back into one
// terminal session before the attempt ends as a failure.
const TerminalRounds = 3

// TerminalTimeout bounds a terminal attempt. Waiting for the user counts, so
// it is long: the session is there to be answered.
const TerminalTimeout = 12 * time.Hour

// errSessionEnded means the terminal session is gone — the agent exited, or
// someone closed it.
var errSessionEnded = errors.New("the terminal session ended")

// terminalSession is one live terminal a worker runs in.
type terminalSession struct {
	// Name is the tmux session.
	Name string
	// Title names the window the user sees: the ticket key.
	Title string
	// Dir is the worktree.
	Dir string
	// RunDir holds the launch script and the turns file.
	RunDir string
	// Log receives what the host reports about the session.
	Log func(format string, args ...any)
	// Expect is how the worker's first message starts. A harness that runs
	// side threads of its own — Codex titles each session in one — reports
	// their turns too; only the thread whose first message is this counts.
	Expect string
	// Dismiss answers screens the harness shows at start that are not the
	// user's business, such as Codex asking to trust the user's own hooks.
	Dismiss []screenAnswer
	// WaitFn and SendFn replace the turns file and the keyboard for a harness
	// driven through its own server, where the terminal only shows the
	// session: OpenCode.
	WaitFn func(ctx context.Context) (Turn, error)
	SendFn func(ctx context.Context, text string) error

	seen    int    // turn lines already consumed
	main    string // the worker's thread, once known
	cmuxRef string // the cmux workspace attached, when there is one
}

// screenAnswer is keys to send when a start-up screen shows Match.
type screenAnswer struct {
	Match string
	Keys  []string
}

// sessionName turns a ticket key into a tmux session name.
func sessionName(key string) string {
	s := regexp.MustCompile(`[^A-Za-z0-9_-]+`).ReplaceAllString(key, "-")
	return "raenil-" + s
}

func (t *terminalSession) logf(format string, args ...any) {
	if t.Log != nil {
		t.Log(format, args...)
	}
}

// TurnsPath is where the harness appends one JSON line per finished turn.
func (t *terminalSession) TurnsPath() string { return filepath.Join(t.RunDir, "turns.jsonl") }

// Start writes the launch script, starts it in a detached tmux session, and
// opens a window on it.
func (t *terminalSession) Start(ctx context.Context, script string) error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("terminal runs need tmux on this Mac (brew install tmux): %w", err)
	}
	path := filepath.Join(t.RunDir, "terminal.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return err
	}
	_ = os.Remove(t.TurnsPath())
	_ = exec.CommandContext(ctx, "tmux", "kill-session", "-t", t.Name).Run()
	out, err := exec.CommandContext(ctx, "tmux", "new-session", "-d", "-s", t.Name,
		"-x", "220", "-y", "60", "-c", t.Dir, "sh "+shellQuote(path)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("start tmux session: %v: %s", err, strings.TrimSpace(string(out)))
	}
	t.open(ctx)
	if len(t.Dismiss) > 0 {
		go t.dismiss(ctx)
	}
	return nil
}

// dismiss watches the first half minute for start-up screens to answer.
func (t *terminalSession) dismiss(ctx context.Context) {
	done := map[int]bool{}
	for i := 0; i < 30 && len(done) < len(t.Dismiss); i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
		out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-p", "-t", t.Name).Output()
		if err != nil {
			return
		}
		for j, d := range t.Dismiss {
			if !done[j] && strings.Contains(string(out), d.Match) {
				done[j] = true
				_ = exec.CommandContext(ctx, "tmux", append([]string{"send-keys", "-t", t.Name}, d.Keys...)...).Run()
				t.logf("terminal: answered %q", d.Match)
			}
		}
	}
}

// open shows the session to the user: a cmux workspace when cmux lets this
// process in, else a Terminal window. Either failing leaves the session
// running; `tmux attach -t <name>` still reaches it.
func (t *terminalSession) open(ctx context.Context) {
	attach := "tmux attach -t " + t.Name
	if bin := cmuxBin(); bin != "" {
		cmd := exec.CommandContext(ctx, bin, "workspace", "create", "--name", t.Title,
			"--cwd", t.Dir, "--command", attach, "--focus", "false")
		cmd.Env = append(os.Environ(), "CMUX_QUIET=1")
		out, err := cmd.CombinedOutput()
		if ref := regexp.MustCompile(`workspace:\d+`).FindString(string(out)); err == nil && ref != "" {
			t.cmuxRef = ref
			t.logf("terminal: cmux workspace %s (%s)", t.Title, ref)
			return
		}
		t.logf("terminal: cmux would not open a workspace (%s) — using Terminal", strings.TrimSpace(string(out)))
	}
	script := fmt.Sprintf(`tell application "Terminal" to do script %q`, attach)
	if out, err := exec.CommandContext(ctx, "osascript", "-e", script).CombinedOutput(); err != nil {
		t.logf("terminal: could not open a window (%v: %s) — attach with: %s", err, strings.TrimSpace(string(out)), attach)
		return
	}
	t.logf("terminal: opened in Terminal — or attach with: %s", attach)
}

// cmuxBin finds the cmux CLI, on PATH or inside the app.
func cmuxBin() string {
	if p, err := exec.LookPath("cmux"); err == nil {
		return p
	}
	const app = "/Applications/cmux.app/Contents/Resources/bin/cmux"
	if _, err := os.Stat(app); err == nil {
		return app
	}
	return ""
}

// alive reports whether the tmux session still exists.
func (t *terminalSession) alive(ctx context.Context) bool {
	return exec.CommandContext(ctx, "tmux", "has-session", "-t", t.Name).Run() == nil
}

// Turn is one finished turn as the harness reported it.
type Turn struct {
	SessionID string
	Message   string
	// Input is the turn's first user message, when the harness reports it.
	Input string
}

// WaitTurn blocks until the harness reports the next finished turn, the
// session ends, or ctx is done.
func (t *terminalSession) WaitTurn(ctx context.Context) (Turn, error) {
	if t.WaitFn != nil {
		return t.WaitFn(ctx)
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if turn, ok := t.nextTurn(); ok {
			return turn, nil
		}
		if !t.alive(ctx) {
			// A turn may have landed as the session closed.
			if turn, ok := t.nextTurn(); ok {
				return turn, nil
			}
			return Turn{}, errSessionEnded
		}
		select {
		case <-ctx.Done():
			return Turn{}, ctx.Err()
		case <-tick.C:
		}
	}
}

// nextTurn reads the first turn line not yet consumed.
func (t *terminalSession) nextTurn() (Turn, bool) {
	f, err := os.Open(t.TurnsPath())
	if err != nil {
		return Turn{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	n := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		n++
		if n <= t.seen {
			continue
		}
		t.seen = n
		turn := parseTurn(line)
		if !t.worker(turn) {
			continue
		}
		return turn, true
	}
	return Turn{}, false
}

// worker reports whether a turn belongs to the worker's own thread.
func (t *terminalSession) worker(turn Turn) bool {
	if t.main == "" {
		if t.Expect != "" && turn.Input != "" &&
			!strings.HasPrefix(strings.TrimSpace(turn.Input), strings.TrimSpace(firstN(t.Expect, 200))) {
			return false
		}
		t.main = turn.SessionID
		return true
	}
	return turn.SessionID == "" || turn.SessionID == t.main
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// parseTurn reads what Claude's Stop hook and Codex's notify both carry.
func parseTurn(line string) Turn {
	var raw map[string]any
	if json.Unmarshal([]byte(line), &raw) != nil {
		return Turn{}
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	turn := Turn{
		SessionID: str("session_id", "thread-id", "thread_id"),
		Message:   str("last_assistant_message", "last-assistant-message"),
	}
	if in, ok := raw["input-messages"].([]any); ok && len(in) > 0 {
		turn.Input, _ = in[0].(string)
	}
	return turn
}

// Send types text into the session as one message and submits it. A
// bracketed paste keeps newlines inside the message instead of sending each
// line on its own.
func (t *terminalSession) Send(ctx context.Context, text string) error {
	if t.SendFn != nil {
		return t.SendFn(ctx, text)
	}
	f, err := os.CreateTemp(t.RunDir, "send-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	f.Close()
	buf := t.Name + "-send"
	for _, args := range [][]string{
		{"load-buffer", "-b", buf, f.Name()},
		{"paste-buffer", "-p", "-d", "-b", buf, "-t", t.Name},
	} {
		if out, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("tmux %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
		}
	}
	time.Sleep(400 * time.Millisecond)
	return exec.CommandContext(ctx, "tmux", "send-keys", "-t", t.Name, "Enter").Run()
}

// Close ends the session and its window.
func (t *terminalSession) Close(ctx context.Context) {
	_ = exec.CommandContext(ctx, "tmux", "kill-session", "-t", t.Name).Run()
	if t.cmuxRef != "" {
		if bin := cmuxBin(); bin != "" {
			cmd := exec.CommandContext(ctx, bin, "workspace", "close", "--workspace", t.cmuxRef)
			cmd.Env = append(os.Environ(), "CMUX_QUIET=1")
			_ = cmd.Run()
		}
	}
}

// Notify tells the user on this Mac that a session is waiting for them.
func notifyUser(title, message string) {
	script := fmt.Sprintf(`display notification %q with title %q sound name "Glass"`, message, title)
	_ = exec.Command("osascript", "-e", script).Run()
}

// asksUser reports whether an agent's last message is waiting on an answer.
// A turn that ends in a question is not finished work: running the checks on
// it would type failures over the question the user is reading.
func asksUser(message string) bool {
	m := strings.TrimSpace(message)
	if m == "" {
		return false
	}
	// The last non-empty line decides: a summary that asked something along
	// the way but ends on a statement is done.
	lines := strings.Split(m, "\n")
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			last = l
			break
		}
	}
	last = strings.TrimRight(last, "*_` )")
	return strings.HasSuffix(last, "?")
}

// TerminalCheck runs the ticket's checks on the worktree as it stands. It
// returns whether they pass and, when not, what to tell the agent.
type TerminalCheck func(ctx context.Context) (pass bool, feedback string)

// runTerminalTurns drives a started session: each finished turn is either a
// question for the user (wait), or work to check (pass → done; fail → type
// the failures back in, up to TerminalRounds times). It returns the last
// turn seen.
func runTerminalTurns(ctx context.Context, t *terminalSession, check TerminalCheck, onQuestion func(string)) (Turn, error) {
	var last Turn
	rounds := 0
	for {
		turn, err := t.WaitTurn(ctx)
		if err != nil {
			return last, err
		}
		if turn.SessionID == "" {
			turn.SessionID = last.SessionID
		}
		last = turn
		if asksUser(turn.Message) {
			t.logf("terminal: the agent is asking — waiting for the user in %s", t.Title)
			if onQuestion != nil {
				onQuestion(turn.Message)
			}
			continue
		}
		if check == nil {
			return last, nil
		}
		pass, feedback := check(ctx)
		if pass {
			t.logf("terminal: checks pass")
			return last, nil
		}
		rounds++
		if rounds > TerminalRounds {
			t.logf("terminal: checks still fail after %d rounds", TerminalRounds)
			return last, nil
		}
		t.logf("terminal: checks fail — round %d of %d, typing the failures in", rounds, TerminalRounds)
		if err := t.Send(ctx, feedback); err != nil {
			return last, fmt.Errorf("send check failures: %w", err)
		}
	}
}

// shellQuote quotes s for sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
