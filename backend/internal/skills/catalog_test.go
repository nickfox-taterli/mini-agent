package skills

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_IncludesStrictFileTypeRouting(t *testing.T) {
	in := []Skill{
		{Name: "minimax-xlsx", Description: "xlsx skill", Path: "skills/minimax-xlsx"},
		{Name: "minimax-docx", Description: "docx skill", Path: "skills/minimax-docx"},
		{Name: "minimax-pdf", Description: "pdf skill", Path: "skills/minimax-pdf"},
		{Name: "pptx-generator", Description: "pptx skill", Path: "skills/pptx-generator"},
	}

	got := BuildSystemPrompt(in)

	required := []string{
		"File-type routing policy (STRICT):",
		"For `.xlsx` / `.xls` / `.xlsm` spreadsheet tasks, MUST try skill `minimax-xlsx` first.",
		"For `.docx` document tasks, MUST try skill `minimax-docx` first.",
		"For `.pdf` generation/edit/fill/reformat tasks, MUST try skill `minimax-pdf` first.",
		"For `.pptx` presentation read/edit/create tasks, MUST try skill `pptx-generator` first.",
		"If a matching file-type skill is installed, do NOT start with `python_*` or LibreOffice tools.",
		"When fallback happens, explicitly report: attempted skill + failure reason + chosen fallback tool.",
	}

	for _, token := range required {
		if !strings.Contains(got, token) {
			t.Fatalf("expected prompt to include %q", token)
		}
	}
}

func TestBuildSystemPrompt_OnlyMentionsInstalledFileTypeSkills(t *testing.T) {
	in := []Skill{
		{Name: "minimax-xlsx", Description: "xlsx skill", Path: "skills/minimax-xlsx"},
	}

	got := BuildSystemPrompt(in)

	if !strings.Contains(got, "minimax-xlsx") {
		t.Fatalf("expected xlsx routing rule to exist")
	}
	if strings.Contains(got, "minimax-docx") {
		t.Fatalf("did not expect docx routing rule when minimax-docx is not installed")
	}
	if strings.Contains(got, "minimax-pdf") {
		t.Fatalf("did not expect pdf routing rule when minimax-pdf is not installed")
	}
	if strings.Contains(got, "pptx-generator") {
		t.Fatalf("did not expect pptx routing rule when pptx-generator is not installed")
	}
}

