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
	b.WriteString("Tool selection policy (STRICT):\n")
	b.WriteString("1) First evaluate whether any installed skill can handle the task.\n")
	b.WriteString("2) If yes, you MUST use `run_skill_bash` and skill workflow first.\n")
	b.WriteString("3) Do NOT start with `python_*` or `code_*` containers when a matching skill exists.\n")
	b.WriteString("4) `python_*` / `code_*` are fallback only when no skill fits, or the skill workflow clearly fails.\n")
	b.WriteString("5) If using fallback containers, explicitly state why skill path is not used.\n")
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
	b.WriteString("For skill execution, use tool `run_skill_bash` with skill_name and command.\n")
	b.WriteString("ALL intermediate/temporary files (generated scripts, API responses, JSON data, scratch files) MUST be written under /tmp/ - NEVER in the project directory or under the skill directory.\n")
	b.WriteString("Only the final deliverable file should be written to FRONTEND_UPLOAD_DIR.\n")
	b.WriteString("When a file is generated, return the absolute local file path first.\n")
	b.WriteString("Do NOT guess or construct download URL in natural language.\n")
	b.WriteString("If you need a downloadable URL, call MCP tool `convert_local_path_to_url` with that file path, then use the tool output URL.\n")
	b.WriteString("Never invent host/port/path and never rewrite path segments.\n")
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
