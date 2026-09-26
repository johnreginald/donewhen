package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The runner host as a macOS login service: launchd starts it at login,
// restarts it if it exits, and keeps its log — so agents run from the
// dashboard without a terminal left open.
const serviceLabel = "dev.raenil.orchestrator-host"

func servicePaths() (plist, logFile string) {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist"),
		filepath.Join(home, "Library", "Logs", "raenil-host.log")
}

func cmdService(ctx context.Context, args []string) error {
	what := ""
	if len(args) > 0 {
		what = args[0]
	}
	plist, logFile := servicePaths()
	target := fmt.Sprintf("gui/%d", os.Getuid())
	switch what {
	case "install":
		bin, err := os.Executable()
		if err != nil {
			return err
		}
		if bin, err = filepath.EvalSymlinks(bin); err != nil {
			return err
		}
		home, _ := os.UserHomeDir()
		b, err := servicePlist(bin, home, logFile, servicePATH(os.Getenv("PATH")), args[1:])
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
			return err
		}
		// Replace a running one: bootout first, ignoring "not loaded".
		_ = exec.CommandContext(ctx, "launchctl", "bootout", target+"/"+serviceLabel).Run()
		if err := os.WriteFile(plist, b, 0o644); err != nil {
			return err
		}
		// bootout returns before the old job is gone; bootstrap fails until
		// it is, so try again for a while.
		var out []byte
		for i := 0; i < 40; i++ {
			if out, err = exec.CommandContext(ctx, "launchctl", "bootstrap", target, plist).CombinedOutput(); err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil {
			return fmt.Errorf("launchctl bootstrap: %v: %s", err, bytes.TrimSpace(out))
		}
		fmt.Printf("Installed. The runner host now starts at login and restarts if it stops.\nLog: %s\n", logFile)
		return nil
	case "uninstall":
		_ = exec.CommandContext(ctx, "launchctl", "bootout", target+"/"+serviceLabel).Run()
		if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Println("Uninstalled. The runner host no longer runs in the background.")
		return nil
	case "restart":
		// Ask the host to stop rather than killing it: it finishes the job in
		// hand first, and launchd (KeepAlive) starts the new binary after.
		old := servicePID(ctx, target)
		if old > 0 {
			if out, err := exec.CommandContext(ctx, "launchctl", "kill", "SIGTERM", target+"/"+serviceLabel).CombinedOutput(); err != nil {
				return fmt.Errorf("launchctl kill: %v: %s", err, bytes.TrimSpace(out))
			}
			fmt.Println("Stopping — the host finishes any job it is working on first…")
			deadline := time.Now().Add(17 * time.Minute)
			for servicePID(ctx, target) == old && time.Now().Before(deadline) {
				time.Sleep(time.Second)
			}
		}
		// Start it now rather than waiting out launchd's throttle.
		if servicePID(ctx, target) <= 0 || servicePID(ctx, target) == old {
			if out, err := exec.CommandContext(ctx, "launchctl", "kickstart", "-k", target+"/"+serviceLabel).CombinedOutput(); err != nil {
				return fmt.Errorf("launchctl kickstart: %v: %s (installed? run: orchestrator service install)", err, bytes.TrimSpace(out))
			}
		}
		fmt.Println("Restarted.")
		return nil
	case "status", "":
		out, err := exec.CommandContext(ctx, "launchctl", "print", target+"/"+serviceLabel).CombinedOutput()
		if err != nil {
			fmt.Println("not installed — run: orchestrator service install")
			return nil
		}
		for _, l := range strings.Split(string(out), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "state =") || strings.HasPrefix(l, "pid =") || strings.HasPrefix(l, "last exit code =") {
				fmt.Println(l)
			}
		}
		fmt.Println("log:", logFile)
		return nil
	}
	return fmt.Errorf("service what? install, uninstall, restart or status")
}

// servicePATH is the PATH the service runs with: the installing shell's, so
// it finds claude, codex and opencode where you do — minus per-session
// directories under the temp dir, which will be gone by the next login.
func servicePATH(path string) string {
	var keep []string
	for _, p := range strings.Split(path, ":") {
		if p == "" || strings.HasPrefix(p, "/var/folders/") || strings.HasPrefix(p, "/private/var/folders/") {
			continue
		}
		keep = append(keep, p)
	}
	return strings.Join(keep, ":")
}

// servicePlist renders the launchd job. Extra host flags pass through.
func servicePlist(bin, home, logFile, path string, hostArgs []string) ([]byte, error) {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var argv strings.Builder
	for _, a := range append([]string{bin, "host"}, hostArgs...) {
		argv.WriteString("\t\t<string>" + esc(a) + "</string>\n")
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + serviceLabel + `</string>
	<key>ProgramArguments</key>
	<array>
` + argv.String() + `	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>` + esc(path) + `</string>
		<key>HOME</key>
		<string>` + esc(home) + `</string>
	</dict>
	<key>WorkingDirectory</key>
	<string>` + esc(home) + `</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>30</integer>
	<key>ExitTimeOut</key>
	<integer>960</integer>
	<key>StandardOutPath</key>
	<string>` + esc(logFile) + `</string>
	<key>StandardErrorPath</key>
	<string>` + esc(logFile) + `</string>
</dict>
</plist>
`), nil
}

// servicePID is the running host's process id, or 0 when it is not running.
func servicePID(ctx context.Context, target string) int {
	out, err := exec.CommandContext(ctx, "launchctl", "print", target+"/"+serviceLabel).CombinedOutput()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "pid = ") {
			n, _ := strconv.Atoi(strings.TrimPrefix(l, "pid = "))
			return n
		}
	}
	return 0
}
