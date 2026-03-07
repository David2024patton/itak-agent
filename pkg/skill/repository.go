package skill

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// Skill represents a loaded skill from a SKILL.md file.
type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Path        string   `json:"path"`
	Body        string   `json:"body"` // markdown instructions
}

// Repository manages skill discovery and loading.
type Repository struct {
	mu     sync.RWMutex
	dir    string
	skills map[string]*Skill
}

// NewRepository creates a skill repository from a directory.
// Auto-discovers all SKILL.md files in subdirectories.
func NewRepository(dir string) (*Repository, error) {
	repo := &Repository{
		dir:    dir,
		skills: make(map[string]*Skill),
	}

	if err := repo.discover(); err != nil {
		return nil, err
	}

	return repo, nil
}

// discover walks the skills directory and loads all SKILL.md files.
func (r *Repository) discover() error {
	if _, err := os.Stat(r.dir); os.IsNotExist(err) {
		debug.Debug("skills", "Skills directory does not exist: %s", r.dir)
		return nil
	}

	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return fmt.Errorf("read skills dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(r.dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			continue
		}

		skill, err := ParseSkillFile(skillPath)
		if err != nil {
			debug.Warn("skills", "Failed to parse %s: %v", skillPath, err)
			continue
		}

		r.mu.Lock()
		r.skills[skill.Name] = skill
		r.mu.Unlock()

		debug.Info("skills", "Discovered skill: %s (%s)", skill.Name, skill.Description)
	}

	debug.Info("skills", "Loaded %d skills from %s", len(r.skills), r.dir)
	return nil
}

// Get returns a skill by name.
func (r *Repository) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[strings.ToLower(name)]
	return s, ok
}

// List returns all available skills.
func (r *Repository) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	return result
}

// Names returns all skill names.
func (r *Repository) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}

// Count returns the number of loaded skills.
func (r *Repository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}

// Refresh reloads all skills from disk.
func (r *Repository) Refresh() error {
	r.mu.Lock()
	r.skills = make(map[string]*Skill)
	r.mu.Unlock()
	return r.discover()
}

// ParseSkillFile parses a SKILL.md file with YAML frontmatter.
// Format:
//
//	---
//	name: my-skill
//	description: Does something useful
//	tags: [coding, analysis]
//	---
//	# Skill Instructions
//	Your detailed instructions here...
func ParseSkillFile(path string) (*Skill, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open skill file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// Parse YAML frontmatter.
	skill := &Skill{
		Path: path,
	}

	// Check for frontmatter delimiter.
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty skill file")
	}
	firstLine := strings.TrimSpace(scanner.Text())
	if firstLine != "---" {
		// No frontmatter  -  entire file is the body, use directory name as skill name.
		skill.Name = strings.ToLower(filepath.Base(filepath.Dir(path)))
		skill.Description = "Skill: " + skill.Name
		skill.Body = firstLine + "\n"

		for scanner.Scan() {
			skill.Body += scanner.Text() + "\n"
		}
		return skill, nil
	}

	// Parse frontmatter key-value pairs.
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			break // end of frontmatter
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			skill.Name = strings.ToLower(value)
		case "description":
			skill.Description = value
		case "tags":
			// Parse [tag1, tag2] or tag1, tag2
			value = strings.Trim(value, "[]")
			for _, t := range strings.Split(value, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					skill.Tags = append(skill.Tags, t)
				}
			}
		}
	}

	// Read body.
	var body strings.Builder
	for scanner.Scan() {
		body.WriteString(scanner.Text())
		body.WriteString("\n")
	}
	skill.Body = body.String()

	// Default name from directory if not set in frontmatter.
	if skill.Name == "" {
		skill.Name = strings.ToLower(filepath.Base(filepath.Dir(path)))
	}
	if skill.Description == "" {
		skill.Description = "Skill: " + skill.Name
	}

	return skill, nil
}
