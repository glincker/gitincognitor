package cli

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	cmd := hookCommand(bin, os.Getenv("GITINCOGNITO_HOME"))
	var done []string
	if *claude {
		msg, err := installClaudeHook(*settings, cmd, *dry)
		if err != nil {
			return err
		}
		done = append(done, msg)
	}
	if *launchd {
		body := plistBody(bin, os.Getenv("GITINCOGNITO_HOME"))
		if *dry {
			done = append(done, "would write "+*plist)
		} else {
			if err := os.MkdirAll(filepath.Dir(*plist), 0o755); err != nil {
				return err
			}
			if err := writeAtomic(*plist, []byte(body), 0o644); err != nil {
				return err
			}
			done = append(done, "wrote "+*plist+", load it with: launchctl load "+*plist)
		}
	}
	c.Emit(map[string]any{"ok": true, "actions": done}, fmt.Sprintf("%v", done))
	return nil
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// hookCommand quotes the binary and carries GITINCOGNITO_HOME so the hook
// syncs the same store the user installed it from.
func hookCommand(bin, home string) string {
	prefix := ""
	if home != "" {
		prefix = "GITINCOGNITO_HOME=" + shQuote(home) + " "
	}
	return prefix + shQuote(bin) + " sync --quiet --all"
}

// writeAtomic writes via a temp file in the same dir so a crash never leaves a
// half-written settings file.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gitincognito-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func installClaudeHook(path, cmd string, dry bool) (string, error) {
	m := map[string]any{}
	raw, err := os.ReadFile(path)
	if err == nil {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return "", fmt.Errorf("%s is not a JSON object, not touching it", path)
		}
		m = obj
	}
	hooks := map[string]any{}
	if h, ok := m["hooks"]; ok {
		if hooks, ok = h.(map[string]any); !ok {
			return "", fmt.Errorf("%s: \"hooks\" is not an object, fix it by hand", path)
		}
	}
	var stop []any
	if st, ok := hooks["Stop"]; ok {
		if stop, ok = st.([]any); !ok {
			return "", fmt.Errorf("%s: \"hooks.Stop\" is not a list, fix it by hand", path)
		}
	}
	enc, _ := json.Marshal(cmd)
	needle := strings.Trim(string(enc), `"`)
	for _, e := range stop {
		if b, _ := json.Marshal(e); strings.Contains(string(b), needle) {
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
	return "added Stop hook to " + path, writeAtomic(path, append(out, '\n'), 0o644)
}

func xmlEsc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func plistBody(bin, home string) string {
	env := ""
	if home != "" {
		env = "<key>EnvironmentVariables</key><dict><key>GITINCOGNITO_HOME</key><string>" + xmlEsc(home) + "</string></dict>\n"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>dev.glincker.gitincognito</string>
<key>ProgramArguments</key><array><string>%s</string><string>sync</string><string>--quiet</string><string>--all</string></array>
%s<key>StartInterval</key><integer>%d</integer>
<key>RunAtLoad</key><true/>
</dict></plist>
`, xmlEsc(bin), env, timerSecs())
}
