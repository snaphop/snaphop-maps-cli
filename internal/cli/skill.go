package cli

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/snaphop/snaphop-maps-cli/internal/atomicfile"
	"github.com/snaphop/snaphop-maps-cli/skills"
)

// skillClient is where one client looks for skills, under the user's home directory or a project.
type skillClient struct {
	Name      string `json:"name"`
	Directory string `json:"directory"`
	Reads     string `json:"reads"`
}

// skillClients are the clients `skill install` knows. Each reads the same SKILL.md; they differ only
// in where they look for it.
var skillClients = []skillClient{
	{"claude", ".claude/skills", "Claude Code"},
	{"codex", ".agents/skills", "OpenAI Codex"},
	{"gemini", ".gemini/skills", "Gemini CLI"},
	{"grok", ".grok/skills", "Grok Build"},
	{"cursor", ".cursor/skills", "Cursor"},
	{"agents", ".agents/skills", "every client that reads the shared .agents/skills directory, such as Codex, Gemini CLI and Grok Build"},
}

// defaultSkillZip is where `skill pack` writes unless told otherwise.
const defaultSkillZip = "snaphop-maps-skill.zip"

// skillEpoch is every packed file's time, so the same skill always packs to the same bytes.
var skillEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func runSkill(inv *invocation) int {
	action := "show"
	if len(inv.positional) == 1 {
		action = inv.positional[0]
	}
	switch action {
	case "show":
		data, _ := skills.FS.ReadFile(skills.Name + "/SKILL.md")
		return inv.text(string(data))
	case "install":
		return installSkill(inv)
	case "pack":
		return packSkill(inv)
	}
	return inv.usage("UNKNOWN_ACTION", fmt.Sprintf("skill has no action %q.", action), "Use `skill`, `skill install --client NAME` or `skill pack`.")
}

// skillFiles are the skill's files, as paths inside its directory.
func skillFiles() []string {
	var files []string
	_ = fs.WalkDir(skills.FS, skills.Name, func(name string, entry fs.DirEntry, _ error) error {
		if !entry.IsDir() {
			files = append(files, strings.TrimPrefix(name, skills.Name+"/"))
		}
		return nil
	})
	return files
}

func skillFile(name string) []byte {
	data, _ := skills.FS.ReadFile(path.Join(skills.Name, name))
	return data
}

// installSkill copies the skill into a client's skills directory: the user's, or with --project a
// project's. A copy already there is replaced.
func installSkill(inv *invocation) int {
	requested := strings.ToLower(*inv.strings["client"])
	var client *skillClient
	var names []string
	for i := range skillClients {
		names = append(names, skillClients[i].Name)
		if skillClients[i].Name == requested {
			client = &skillClients[i]
		}
	}
	if client == nil {
		return inv.usage("INVALID_FLAG", fmt.Sprintf("--client %q is not a client skill install knows.", requested),
			"Give --client as one of "+strings.Join(names, ", ")+". For claude.ai or ChatGPT, upload the zip from `snaphop-maps skill pack`.")
	}
	clientDirectory := filepath.FromSlash(client.Directory)
	// A project may come from anyone, so nothing is written outside it. The user's own skills directory
	// is theirs, and may be a link, as a dotfile manager makes one: only the skill's directory in it is
	// confined.
	base, directory := inv.path(*inv.strings["project"]), filepath.Join(clientDirectory, skills.Name)
	if base == "" {
		home, err := inv.env.HomeDir()
		if err != nil {
			return inv.fail("NO_HOME_DIRECTORY", "Cannot find the home directory: "+err.Error()+".", "Give a directory with --project.")
		}
		base, directory = filepath.Join(home, clientDirectory), skills.Name
	}
	target := filepath.Join(base, directory)
	files := skillFiles()
	replaced, err := writeSkill(base, directory, files)
	if err != nil {
		return inv.fail("SKILL_NOT_INSTALLED", err.Error()+".",
			"Check that "+base+" is a directory you can write to, and that "+target+" is a directory of its own inside it, not a link, holding nothing but the skill.")
	}
	document, _ := json.Marshal(map[string]any{
		"client": client.Name, "reads": client.Reads, "path": target, "files": files, "replaced": replaced,
	})
	return inv.answer(document)
}

// writeSkill writes the skill's files into directory under base, and nowhere else: a link that leads
// out of base is refused, the skill's directory may not be a link, and a link where a file goes is
// replaced rather than followed. A project may come from anyone, and a link it holds must not turn an
// install into a write over the user's own files, such as the credentials file. base itself is made
// when missing, through any link on the way to it. It says whether a copy was there before.
func writeSkill(base, directory string, files []string) (bool, error) {
	err := os.MkdirAll(base, 0o755)
	var root *os.Root
	if err == nil {
		root, err = os.OpenRoot(base)
	}
	if err != nil {
		return false, err
	}
	defer root.Close()
	info, err := root.Lstat(directory)
	replaced := err == nil
	if replaced && !info.IsDir() {
		return false, fmt.Errorf("%s is a link or a file, not the skill's own directory", filepath.Join(base, directory))
	}
	for _, name := range files {
		file := filepath.Join(directory, filepath.FromSlash(name))
		err := root.MkdirAll(filepath.Dir(file), 0o755)
		if err == nil {
			err = atomicfile.WriteIn(root, file, skillFile(name), 0o644)
		}
		if err != nil {
			return false, err
		}
	}
	return replaced, nil
}

// packSkill writes the skill as a zip holding its one directory, the shape claude.ai, ChatGPT and
// the model APIs take as an upload. The same skill always packs to the same bytes.
func packSkill(inv *invocation) int {
	output := inv.path(inv.first(*inv.strings["output"], defaultSkillZip))
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range skillFiles() {
		// Writing to memory cannot fail.
		entry, _ := writer.CreateHeader(&zip.FileHeader{Name: skills.Name + "/" + name, Method: zip.Deflate, Modified: skillEpoch})
		_, _ = entry.Write(skillFile(name))
	}
	_ = writer.Close()
	if err := atomicfile.Write(output, archive.Bytes(), 0o644); err != nil {
		return inv.fail("SKILL_NOT_PACKED", err.Error()+".", "Give --output a file in a directory you can write to.")
	}
	sum := sha256.Sum256(archive.Bytes())
	document, _ := json.Marshal(map[string]any{"path": output, "bytes": archive.Len(), "sha256": hex.EncodeToString(sum[:])})
	return inv.answer(document)
}
