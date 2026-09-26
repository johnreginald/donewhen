package main

import (
	"strings"
	"testing"
)

func TestServicePlist(t *testing.T) {
	path := servicePATH("/Users/me/.local/bin:/var/folders/w5/x/T/shims:/usr/bin::/private/var/folders/y")
	if path != "/Users/me/.local/bin:/usr/bin" {
		t.Errorf("PATH = %q: per-session temp dirs must go", path)
	}
	b, err := servicePlist("/Users/me/.local/bin/orchestrator", "/Users/me", "/Users/me/Library/Logs/raenil-host.log", path, []string{"--poll", "5s"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"<string>dev.raenil.orchestrator-host</string>",
		"<string>/Users/me/.local/bin/orchestrator</string>\n\t\t<string>host</string>\n\t\t<string>--poll</string>",
		"<key>KeepAlive</key>\n\t<true/>", "<key>RunAtLoad</key>\n\t<true/>",
		// launchd waits for a stopping host to finish its job (Drain is 15m).
		"<key>ExitTimeOut</key>\n\t<integer>960</integer>",
		"<string>/Users/me/.local/bin:/usr/bin</string>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("plist lacks %q:\n%s", want, s)
		}
	}
}
