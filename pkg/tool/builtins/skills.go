package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/David2024patton/GOAgent/pkg/skill"
)

// SkillListTool lists all available skills.
type SkillListTool struct {
	Repo *skill.Repository
}

func (t *SkillListTool) Name() string { return "skill_list" }
func (t *SkillListTool) Description() string {
	return "List all available skills. Returns name, description, and tags for each skill."
}
func (t *SkillListTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *SkillListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if t.Repo == nil {
		return "No skills repository configured.", nil
	}

	skills := t.Repo.List()
	if len(skills) == 0 {
		return "No skills available.", nil
	}

	type skillInfo struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags,omitempty"`
	}

	infos := make([]skillInfo, 0, len(skills))
	for _, s := range skills {
		infos = append(infos, skillInfo{
			Name:        s.Name,
			Description: s.Description,
			Tags:        s.Tags,
		})
	}

	data, _ := json.MarshalIndent(infos, "", "  ")
	return string(data), nil
}

// SkillLoadTool loads a skill's instructions by name.
type SkillLoadTool struct {
	Repo *skill.Repository
}

func (t *SkillLoadTool) Name() string { return "skill_load" }
func (t *SkillLoadTool) Description() string {
	return "Load a skill's full instructions by name. The instructions tell you how to perform a specific task. Use skill_list first to see available skills."
}
func (t *SkillLoadTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the skill to load",
			},
		},
		"required": []string{"name"},
	}
}

func (t *SkillLoadTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("missing required argument: name")
	}

	if t.Repo == nil {
		return "No skills repository configured.", nil
	}

	s, found := t.Repo.Get(strings.ToLower(name))
	if !found {
		available := t.Repo.Names()
		return fmt.Sprintf("Skill %q not found. Available skills: %s", name, strings.Join(available, ", ")), nil
	}

	return fmt.Sprintf("# Skill: %s\n\n%s\n\n---\n\n%s", s.Name, s.Description, s.Body), nil
}
