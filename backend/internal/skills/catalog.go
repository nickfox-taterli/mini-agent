package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	Path        string
}

func Load(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	out := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		mdPath := filepath.Join(skillDir, "SKILL.md")
		b, err := os.ReadFile(mdPath)
		if err != nil {
			continue
		}
		name, desc := parseSkillMeta(string(b), entry.Name())
		out = append(out, Skill{
			Name:        name,
			Description: desc,
			Path:        skillDir,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func BuildSystemPrompt(sk []Skill) string {
	if len(sk) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("You can use local skills installed on the server.\n")
	b.WriteString("When a request matches a skill, follow the corresponding SKILL.md.\n")
	b.WriteString("Installed skills:\n")
	for _, item := range sk {
		b.WriteString("- ")
		b.WriteString(item.Name)
		if item.Description != "" {
			b.WriteString(": ")
			b.WriteString(item.Description)
		}
		b.WriteString(". path=")
		b.WriteString(item.Path)
		b.WriteString("\n")
	}
	b.WriteString("For skill execution, prefer tool `run_skill_bash` with skill_name and command.\n")
	b.WriteString("For generated files, prefer `write_frontend_temp_file` or direct output under FRONTEND_TMP_DIR.\n")
	return b.String()
}

func parseSkillMeta(content string, fallbackName string) (string, string) {
	lines := strings.Split(content, "\n")
	name := fallbackName
	desc := ""
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "name:") {
			v := strings.TrimSpace(strings.TrimPrefix(trim, "name:"))
			v = strings.Trim(v, "\"'")
			if v != "" {
				name = v
			}
		}
		if strings.HasPrefix(trim, "description:") {
			v := strings.TrimSpace(strings.TrimPrefix(trim, "description:"))
			v = strings.Trim(v, "\"'")
			desc = v
		}
		if trim == "---" && desc != "" {
			break
		}
	}
	return name, desc
}
