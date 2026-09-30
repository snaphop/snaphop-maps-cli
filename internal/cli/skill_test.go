package cli

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/snaphop/snaphop-maps-cli/skills"
)

func skillText(t *testing.T) string {
	t.Helper()
	data, err := skills.FS.ReadFile(skills.Name + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// frontmatter returns the skill's top-level frontmatter fields and its body.
func frontmatter(t *testing.T, text string) (map[string]string, string) {
	t.Helper()
	if !strings.HasPrefix(text, "---\n") {
		t.Fatal("SKILL.md does not open with frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		t.Fatal("SKILL.md's frontmatter is not closed")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(text[4:4+end], "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok && !strings.HasPrefix(line, " ") {
			fields[key] = strings.TrimSpace(value)
		}
	}
	return fields, text[4+end+5:]
}

// TestTheSkillFollowsTheAgentSkillsSpecification holds SKILL.md to https://agentskills.io/specification,
// which every client that reads it follows.
func TestTheSkillFollowsTheAgentSkillsSpecification(t *testing.T) {
	t.Parallel()
	fields, body := frontmatter(t, skillText(t))
	name := fields["name"]
	if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(name) || len(name) > 64 || name != skills.Name {
		t.Errorf("name %q is not a valid skill name matching its directory %q", name, skills.Name)
	}
	if description := fields["description"]; description == "" || len(description) > 1024 || !strings.Contains(description, "Use when") {
		t.Errorf("description (%d characters) must say what the skill does and when to use it, in at most 1024", len(description))
	}
	if compatibility := fields["compatibility"]; len(compatibility) > 500 {
		t.Errorf("compatibility is %d characters", len(compatibility))
	}
	if fields["license"] != "MIT" {
		t.Errorf("license = %q", fields["license"])
	}
	for field := range fields {
		switch field {
		case "name", "description", "license", "compatibility", "metadata", "allowed-tools":
		default:
			t.Errorf("frontmatter field %q is not in the specification", field)
		}
	}
	if lines := strings.Count(body, "\n"); lines >= 500 {
		t.Errorf("the body is %d lines; the specification asks for under 500", lines)
	}
	files := skillFiles()
	if len(files) == 0 || files[0] != "SKILL.md" {
		t.Errorf("skill files = %v", files)
	}
}

func TestTheSkillNamesEveryCommand(t *testing.T) {
	t.Parallel()
	text := skillText(t)
	for _, cmd := range commands() {
		if !strings.Contains(text, "snaphop-maps "+cmd.Name) {
			t.Errorf("SKILL.md never shows `snaphop-maps %s`", cmd.Name)
		}
	}
}

// TestEveryExampleInTheSkillRuns runs each command in SKILL.md's code blocks, as a shell would split
// it, against a fake service: an example an agent copies must work.
func TestEveryExampleInTheSkillRuns(t *testing.T) {
	t.Parallel()
	examples := shellCommands(t, codeBlocks(skillText(t)))
	if len(examples) < 8 {
		t.Fatalf("found only %d examples: %q", len(examples), examples)
	}
	for _, example := range examples {
		t.Run(strings.Join(example, " "), func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(r request) answer {
				if r.Tool == "register_agent" {
					return tool(registered("shk_new"))
				}
				return tool(map[string]any{"id": "m1", "published": map[string]any{"page": "https://maps.example/m1/"}})
			})
			h := newHarness(t, s)
			if example[1] != "register-agent" {
				h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
			}
			if o := h.run(example[1:]...); o.code != exitOK {
				t.Fatalf("exit %d\nstdout: %s\nstderr: %s", o.code, o.stdout, o.stderr)
			}
		})
	}
}

// codeBlocks is the text of every fenced code block.
func codeBlocks(markdown string) string {
	var b strings.Builder
	inside := false
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inside = !inside
			b.WriteString("\n")
			continue
		}
		if inside {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// shellCommands splits script text into the snaphop-maps commands a POSIX shell would run: quotes
// may span lines, a backslash before a newline continues the line, and # starts a comment.
func shellCommands(t *testing.T, script string) [][]string {
	t.Helper()
	var commands [][]string
	var words []string
	var word strings.Builder
	inWord, quote := false, byte(0)
	endWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}
	endCommand := func() {
		endWord()
		if len(words) > 0 && words[0] == "snaphop-maps" {
			commands = append(commands, words)
		}
		words = nil
	}
	for i := 0; i < len(script); i++ {
		c := script[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote != 0:
			word.WriteByte(c)
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == '\\' && i+1 < len(script) && script[i+1] == '\n':
			i++
		case c == '#' && !inWord:
			for i < len(script) && script[i] != '\n' {
				i++
			}
			endCommand()
		case c == '\n' || c == '|' || c == ';':
			endCommand()
		case c == ' ' || c == '\t':
			endWord()
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	if quote != 0 {
		t.Fatal("a code block leaves a quote open")
	}
	endCommand()
	return commands
}

func TestSkillPrintsTheSkill(t *testing.T) {
	t.Parallel()
	o := newHarness(t, nil).run("skill")
	if o.code != exitOK || o.stdout != skillText(t) {
		t.Fatalf("exit %d, stdout %q", o.code, o.stdout)
	}
}

func TestSkillInstallPutsItWhereEachClientLooks(t *testing.T) {
	t.Parallel()
	for _, client := range skillClients {
		t.Run(client.Name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			home, _ := h.homeDir()
			want := filepath.Join(home, filepath.FromSlash(client.Directory), "snaphop-maps")
			out := h.run("skill", "install", "--client", strings.ToUpper(client.Name)).success(t)
			if out["path"] != want || out["client"] != client.Name || out["replaced"] != false {
				t.Fatalf("install = %v, want path %s", out, want)
			}
			if data, err := os.ReadFile(filepath.Join(want, "SKILL.md")); err != nil || string(data) != skillText(t) {
				t.Fatalf("installed SKILL.md: %v", err)
			}
			if again := h.run("skill", "install", "--client", client.Name).success(t); again["replaced"] != true {
				t.Fatalf("second install = %v", again)
			}
		})
	}
}

func TestSkillInstallIntoAProject(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	project := filepath.Join(h.dir, "project")
	out := h.run("skill", "install", "--client", "gemini", "--project", project).success(t)
	if out["path"] != filepath.Join(project, ".gemini", "skills", "snaphop-maps") {
		t.Fatalf("install = %v", out)
	}
}

func TestSkillRefusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(h *harness) []string
		exit  int
		code  string
	}{
		{"unknown action", func(*harness) []string { return []string{"skill", "publish"} }, exitUsage, "UNKNOWN_ACTION"},
		{"no client", func(*harness) []string { return []string{"skill", "install"} }, exitUsage, "INVALID_FLAG"},
		{"unknown client", func(*harness) []string { return []string{"skill", "install", "--client", "clippy"} }, exitUsage, "INVALID_FLAG"},
		{"no home", func(h *harness) []string {
			h.homeDir = func() (string, error) { return "", errors.New("$HOME is not defined") }
			return []string{"skill", "install", "--client", "claude"}
		}, exitFailure, "NO_HOME_DIRECTORY"},
		{"project is a file", func(h *harness) []string {
			return []string{"skill", "install", "--client", "claude", "--project", h.file("project", "")}
		}, exitFailure, "SKILL_NOT_INSTALLED"},
		{"something in the way", func(h *harness) []string {
			if err := os.MkdirAll(filepath.Join(h.dir, ".claude", "skills", "snaphop-maps", "SKILL.md"), 0o755); err != nil {
				t.Fatal(err)
			}
			return []string{"skill", "install", "--client", "claude", "--project", h.dir}
		}, exitFailure, "SKILL_NOT_INSTALLED"},
		{"pack nowhere", func(h *harness) []string {
			return []string{"skill", "pack", "--output", filepath.Join(h.dir, "missing", "skill.zip")}
		}, exitFailure, "SKILL_NOT_PACKED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			if p := h.run(tc.setup(h)...).failure(t, tc.exit, tc.code); p.Hint == "" {
				t.Fatalf("no hint: %+v", p)
			}
		})
	}
}

func TestSkillPackMakesAnUploadableZip(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	out := h.run("skill", "pack").success(t)
	path := filepath.Join(h.dir, defaultSkillZip)
	data, err := os.ReadFile(path)
	if err != nil || out["path"] != path || out["bytes"] != float64(len(data)) {
		t.Fatalf("pack = %v, %v", out, err)
	}
	sum := sha256.Sum256(data)
	if out["sha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 = %v", out["sha256"])
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 1 || archive.File[0].Name != "snaphop-maps/SKILL.md" {
		t.Fatalf("zip holds %v", archive.File)
	}
	entry, _ := archive.File[0].Open()
	var content bytes.Buffer
	_, _ = content.ReadFrom(entry)
	if content.String() != skillText(t) {
		t.Fatal("the zip's SKILL.md differs from the skill")
	}
	elsewhere := filepath.Join(t.TempDir(), "again.zip")
	again := h.run("skill", "pack", "--output", elsewhere).success(t)
	if again["sha256"] != out["sha256"] || again["path"] != elsewhere {
		t.Fatalf("packing twice gave %v and %v", out, again)
	}
}
