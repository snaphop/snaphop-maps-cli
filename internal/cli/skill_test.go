package cli

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	schemas := toolSchemas(t)
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
			// What the example sends is what the tool takes: no misspelt field, no value of the wrong type.
			for _, r := range s.received() {
				if schema, known := schemas[r.Tool]; r.Method == "tools/call" && known {
					conforms(t, r.Tool, schema, any(r.Arguments))
				}
			}
		})
	}
}

// toolSchemas are the input schemas of the service's tools, by name.
func toolSchemas(t *testing.T) map[string]map[string]any {
	t.Helper()
	var listed struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	data, _ := testdata.ReadFile("testdata/tools.json")
	if err := json.Unmarshal(data, &listed); err != nil {
		t.Fatal(err)
	}
	schemas := map[string]map[string]any{}
	for _, tool := range listed.Tools {
		schemas[tool.Name] = tool.InputSchema
	}
	return schemas
}

// conforms checks a value against the parts of a JSON Schema the service's tools use: type,
// properties, additionalProperties, required and items.
func conforms(t *testing.T, path string, schema map[string]any, value any) {
	t.Helper()
	types := map[string]bool{}
	switch declared := schema["type"].(type) {
	case string:
		types[declared] = true
	case []any:
		for _, name := range declared {
			types[name.(string)] = true
		}
	}
	kind := map[bool]string{true: "integer", false: "number"}
	switch v := value.(type) {
	case nil:
		kind[false] = "null"
	case bool:
		kind[false] = "boolean"
	case string:
		kind[false] = "string"
	case float64:
		kind[false] = map[bool]string{true: "integer", false: "number"}[v == float64(int64(v))]
	case []any:
		kind[false] = "array"
		items, _ := schema["items"].(map[string]any)
		for i, item := range v {
			if items != nil {
				conforms(t, fmt.Sprintf("%s[%d]", path, i), items, item)
			}
		}
	case map[string]any:
		kind[false] = "object"
		properties, _ := schema["properties"].(map[string]any)
		for name, field := range v {
			property, known := properties[name].(map[string]any)
			if !known && schema["additionalProperties"] == false {
				t.Errorf("%s.%s is not a field it takes", path, name)
			} else if known {
				conforms(t, path+"."+name, property, field)
			}
		}
		required, _ := schema["required"].([]any)
		for _, name := range required {
			if _, given := v[name.(string)]; !given {
				t.Errorf("%s has no %s, which it requires", path, name)
			}
		}
	}
	got := kind[false]
	if len(types) > 0 && !types[got] && !(got == "integer" && types["number"]) {
		t.Errorf("%s is %s, not %v", path, got, schema["type"])
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

// TestSkillInstallFollowsTheUsersOwnLinks is a home whose skills directory a dotfile manager links
// elsewhere, by an absolute link: the user's own, so the install goes through it.
func TestSkillInstallFollowsTheUsersOwnLinks(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	home, _ := h.homeDir()
	dotfiles := filepath.Join(h.dir, "dotfiles", "claude")
	if err := os.MkdirAll(dotfiles, 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, dotfiles, filepath.Join(home, ".claude"))
	h.run("skill", "install", "--client", "claude").success(t)
	if data, err := os.ReadFile(filepath.Join(dotfiles, "skills", "snaphop-maps", "SKILL.md")); err != nil || string(data) != skillText(t) {
		t.Fatalf("the skill did not go through the link: %v", err)
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

// link makes a symbolic link, or skips a test on a system that will not let it.
func link(t *testing.T, target, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, name); err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}
}

// TestSkillInstallNeverWritesThroughALink installs into a project that plants links, as a cloned
// repository could, aimed at a file the user cannot get back: the credentials file.
func TestSkillInstallNeverWritesThroughALink(t *testing.T) {
	t.Parallel()
	const credentials = `{"version": 1, "accounts": {"https://maps.snaphop.ai": {"apiKey": "shk_irreplaceable"}}}`
	cases := []struct {
		name  string
		plant func(t *testing.T, project, victim string)
		// refused is what the error says, when the install is refused.
		refused string
	}{
		{"the skill file is a link", func(t *testing.T, project, victim string) {
			link(t, filepath.Join(victim, "credentials.json"), filepath.Join(project, ".claude", "skills", "snaphop-maps", "SKILL.md"))
		}, ""},
		{"the skill's directory is a link", func(t *testing.T, project, victim string) {
			link(t, victim, filepath.Join(project, ".claude", "skills", "snaphop-maps"))
		}, "is a link or a file, not the skill's own directory"},
		{"a directory above it leads out of the project", func(t *testing.T, project, victim string) {
			link(t, victim, filepath.Join(project, ".claude"))
		}, "escapes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			project, victim := filepath.Join(h.dir, "project"), filepath.Join(h.dir, "victim")
			if err := os.MkdirAll(filepath.Join(victim, "skills", "snaphop-maps"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(victim, "credentials.json"), []byte(credentials), 0o600); err != nil {
				t.Fatal(err)
			}
			tc.plant(t, project, victim)
			o := h.run("skill", "install", "--client", "claude", "--project", project)
			if tc.refused == "" {
				o.success(t)
				if data, err := os.ReadFile(filepath.Join(project, ".claude", "skills", "snaphop-maps", "SKILL.md")); err != nil || string(data) != skillText(t) {
					t.Fatalf("the skill was not installed in place of the link: %v", err)
				}
			} else if p := o.failure(t, exitFailure, "SKILL_NOT_INSTALLED"); !strings.Contains(p.Message, tc.refused) {
				t.Fatalf("message = %q, want %q", p.Message, tc.refused)
			}
			if data, _ := os.ReadFile(filepath.Join(victim, "credentials.json")); string(data) != credentials {
				t.Fatalf("the credentials file now holds %.60q", data)
			}
			if entries, _ := os.ReadDir(filepath.Join(victim, "skills", "snaphop-maps")); len(entries) != 0 {
				t.Fatalf("wrote outside the project: %v", entries)
			}
		})
	}
}

func TestSkillInstallRefusesAFileWhereItsDirectoryGoes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	if err := os.MkdirAll(filepath.Join(h.dir, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	h.file(filepath.Join(".claude", "skills", "snaphop-maps"), "mine")
	h.run("skill", "install", "--client", "claude", "--project", h.dir).failure(t, exitFailure, "SKILL_NOT_INSTALLED")
	if data, _ := os.ReadFile(filepath.Join(h.dir, ".claude", "skills", "snaphop-maps")); string(data) != "mine" {
		t.Fatalf("the file now holds %q", data)
	}
}

func TestSkillPackReplacesALinkInsteadOfFollowingIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	victim := h.file("victim", "keep me")
	link(t, victim, filepath.Join(h.dir, defaultSkillZip))
	h.run("skill", "pack").success(t)
	if data, _ := os.ReadFile(victim); string(data) != "keep me" {
		t.Fatalf("the link's target now holds %.20q", data)
	}
}
