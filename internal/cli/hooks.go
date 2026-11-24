package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func init() { commands["hooks"] = cmdHooks }

// timerSecs is the launchd interval, overridable for people who want it tighter.
func timerSecs() int {
	if n, err := strconv.Atoi(os.Getenv("GITINCOGNITO_TIMER_SECS")); err == nil && n > 0 {
		return n
	}
	return 900
}

func cmdHooks(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return errors.New("usage: git incognito hooks install [--claude] [--launchd] [--print]")
	}
	fs := c.flags("hooks install")
	claude := fs.Bool("claude", false, "add a Claude Code Stop hook")
	launchd := fs.Bool("launchd", false, "write a launchd timer plist (macOS)")
	dry := fs.Bool("print", false, "show what would change, write nothing")
	home, _ := os.UserHomeDir()
	settings := fs.String("settings", filepath.Join(home, ".claude", "settings.json"), "Claude settings path")
	plist := fs.String("plist", filepath.Join(home, "Library", "LaunchAgents", "dev.glincker.gitincognito.plist"), "plist path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !*claude && !*launchd {
		*claude, *launchd = true, true
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := bin + " sync --quiet --all"
	var done []string
	if *claude {
		msg, err := installClaudeHook(*settings, cmd, *dry)
		if err != nil {
			return err
		}
		done = append(done, msg)
	}
	if *launchd {
		body := plistBody(bin)
		if *dry {
			done = append(done, "would write "+*plist)
		} else {
			if err := os.MkdirAll(filepath.Dir(*plist), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(*plist, []byte(body), 0o644); err != nil {
				return err
			}
			done = append(done, "wrote "+*plist+", load it with: launchctl load "+*plist)
		}
	}
	c.Emit(map[string]any{"ok": true, "actions": done}, fmt.Sprintf("%v", done))
	return nil
}

func installClaudeHook(path, cmd string, dry bool) (string, error) {
	m := map[string]any{}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &m); err != nil {
			return "", fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	stop, _ := hooks["Stop"].([]any)
	for _, e := range stop {
		if b, _ := json.Marshal(e); string(b) != "" && contains(string(b), cmd) {
			return "Stop hook already present in " + path, nil
		}
	}
	stop = append(stop, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": cmd}}})
	hooks["Stop"] = stop
	m["hooks"] = hooks
	if dry {
		return "would add Stop hook to " + path, nil
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if raw != nil {
		if err := os.WriteFile(path+".bak", raw, 0o600); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return "added Stop hook to " + path, os.WriteFile(path, append(out, '\n'), 0o644)
}

func plistBody(bin string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>dev.glincker.gitincognito</string>
<key>ProgramArguments</key><array><string>%s</string><string>sync</string><string>--quiet</string><string>--all</string></array>
<key>StartInterval</key><integer>%d</integer>
<key>RunAtLoad</key><true/>
</dict></plist>
`, bin, timerSecs())
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
