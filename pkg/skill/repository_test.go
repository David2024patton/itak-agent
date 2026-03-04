package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillFileWithFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "test-skill")
	os.MkdirAll(skillDir, 0755)

	skillFile := filepath.Join(skillDir, "SKILL.md")
	content := `---
name: web-scraper
description: Scrapes web pages for data
tags: [web, scraping, automation]
---
# Web Scraper Skill

Use this skill to scrape web pages.
1. Navigate to URL
2. Extract data
`
	os.WriteFile(skillFile, []byte(content), 0644)

	skill, err := ParseSkillFile(skillFile)
	if err != nil {
		t.Fatalf("ParseSkillFile error: %v", err)
	}

	if skill.Name != "web-scraper" {
		t.Errorf("expected name 'web-scraper', got %q", skill.Name)
	}
	if skill.Description != "Scrapes web pages for data" {
		t.Errorf("expected description mismatch, got %q", skill.Description)
	}
	if len(skill.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(skill.Tags))
	}
	if skill.Body == "" {
		t.Error("body should not be empty")
	}
}

func TestParseSkillFileNoFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "my-skill")
	os.MkdirAll(skillDir, 0755)

	skillFile := filepath.Join(skillDir, "SKILL.md")
	content := `# My Skill
Instructions for the skill.
`
	os.WriteFile(skillFile, []byte(content), 0644)

	skill, err := ParseSkillFile(skillFile)
	if err != nil {
		t.Fatalf("ParseSkillFile error: %v", err)
	}

	// Name should be derived from directory name.
	if skill.Name != "my-skill" {
		t.Errorf("expected name 'my-skill' from directory, got %q", skill.Name)
	}
	if skill.Body == "" {
		t.Error("body should contain the file content")
	}
}

func TestParseSkillFileEmpty(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "empty-skill")
	os.MkdirAll(skillDir, 0755)

	skillFile := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(skillFile, []byte(""), 0644)

	_, err := ParseSkillFile(skillFile)
	if err == nil {
		t.Error("expected error for empty skill file")
	}
}

func TestRepositoryDiscovery(t *testing.T) {
	tmp := t.TempDir()

	// Create 3 skill directories with SKILL.md files.
	for _, name := range []string{"coding", "research", "web"} {
		dir := filepath.Join(tmp, name)
		os.MkdirAll(dir, 0755)
		content := "---\nname: " + name + "\ndescription: " + name + " skill\n---\n# " + name + "\nInstructions.\n"
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644)
	}

	// Create a directory without SKILL.md (should be ignored).
	os.MkdirAll(filepath.Join(tmp, "empty-dir"), 0755)

	repo, err := NewRepository(tmp)
	if err != nil {
		t.Fatalf("NewRepository error: %v", err)
	}

	if repo.Count() != 3 {
		t.Errorf("expected 3 skills, got %d", repo.Count())
	}
}

func TestRepositoryGet(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "my-tool")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: my-tool\ndescription: test\n---\nBody"), 0644)

	repo, _ := NewRepository(tmp)

	skill, ok := repo.Get("my-tool")
	if !ok {
		t.Fatal("should find skill by name")
	}
	if skill.Description != "test" {
		t.Errorf("expected 'test' description, got %q", skill.Description)
	}
}

func TestRepositoryNames(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(tmp, name)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\nBody"), 0644)
	}

	repo, _ := NewRepository(tmp)
	names := repo.Names()

	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}

func TestRepositoryRefresh(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "skill1")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: skill1\ndescription: test\n---\nBody"), 0644)

	repo, _ := NewRepository(tmp)
	if repo.Count() != 1 {
		t.Fatalf("expected 1 skill initially, got %d", repo.Count())
	}

	// Add a new skill and refresh.
	dir2 := filepath.Join(tmp, "skill2")
	os.MkdirAll(dir2, 0755)
	os.WriteFile(filepath.Join(dir2, "SKILL.md"), []byte("---\nname: skill2\ndescription: test\n---\nBody"), 0644)

	repo.Refresh()
	if repo.Count() != 2 {
		t.Errorf("after refresh, expected 2 skills, got %d", repo.Count())
	}
}

func TestRepositoryNonexistentDir(t *testing.T) {
	repo, err := NewRepository("/nonexistent/dir/skills")
	if err != nil {
		t.Fatalf("should not error on nonexistent dir, got: %v", err)
	}
	if repo.Count() != 0 {
		t.Errorf("expected 0 skills for nonexistent dir, got %d", repo.Count())
	}
}
