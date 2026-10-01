package config

import (
	"log"
	"os"
	"strings"
	"sync"
)

// legacyPrefix is the env prefix used before the rename to DoneWhen. Old names
// keep working for one release; remove this file's fallback after that.
const legacyPrefix = "RAENIL_"

const newPrefix = "DONEWHEN_"

var warned sync.Map // legacy var name -> struct{}

// warnf is the logger for deprecation warnings (replaced in tests).
var warnf = log.Printf

// Getenv returns the value of a DONEWHEN_* variable. The new name wins. If
// only the old RAENIL_* name is set, its value is used and one deprecation
// warning is logged per variable. name must start with DONEWHEN_.
func Getenv(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	if !strings.HasPrefix(name, newPrefix) {
		return ""
	}
	old := legacyPrefix + strings.TrimPrefix(name, newPrefix)
	v := os.Getenv(old)
	if v == "" {
		return ""
	}
	if _, seen := warned.LoadOrStore(old, struct{}{}); !seen {
		warnf("deprecated: %s is set; rename it to %s (the old name works for one more release)", old, name)
	}
	return v
}
