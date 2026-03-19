package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ══════════════════════════════════════════════════════════════════
// net_skill  -  load a NetClaw network skill procedure
// ══════════════════════════════════════════════════════════════════
//
// What: Reads a specific network skill's SKILL.md from the NetClaw catalog.
// Why:  92 skills are too large to inject into prompt. The agent loads
//       a specific skill on demand when it needs the procedure.
// How:  Reads data/network/skills/{skill_name}/SKILL.md and returns it.

type NetSkillTool struct {
	DataDir string // agent data directory
}

func (t *NetSkillTool) Name() string { return "net_skill" }
func (t *NetSkillTool) Description() string {
	return `Load a network skill procedure from the NetClaw catalog (92 skills).
Use "list" to see all available skills, or provide a skill name to load its full procedure.

Categories: pyATS device/routing/security/topology, Cisco ACI/ISE/Meraki/CML/FMC/SD-WAN/NSO,
Juniper JunOS, Arista CVP, F5 BIG-IP, Palo Alto, FortiManager, EVPN/VXLAN,
AWS/GCP cloud, Grafana/Prometheus, nmap, packet analysis, BGP/OSPF protocol participation,
NetBox/Nautobot reconciliation, ServiceNow ITSM, Slack alerting, subnet calculator, RFC lookup.

Examples: net_skill("pyats-health-check"), net_skill("subnet-calculator"), net_skill("list")`
}

func (t *NetSkillTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"skill": map[string]interface{}{
				"type":        "string",
				"description": "Skill name (e.g. 'pyats-health-check', 'subnet-calculator') or 'list' to see all available skills",
			},
		},
		"required": []string{"skill"},
	}
}

func (t *NetSkillTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	skill := argStr(args, "skill")
	if skill == "" {
		return "", fmt.Errorf("missing required argument: skill")
	}

	skillsDir := filepath.Join(t.DataDir, "network", "skills")

	// List mode: show all available skills.
	if strings.ToLower(skill) == "list" {
		catalogPath := filepath.Join(t.DataDir, "network", "SKILL_CATALOG.md")
		data, err := os.ReadFile(catalogPath)
		if err != nil {
			// Fall back to listing directories.
			entries, dirErr := os.ReadDir(skillsDir)
			if dirErr != nil {
				return "", fmt.Errorf("cannot read skills directory: %w", dirErr)
			}
			var names []string
			for _, e := range entries {
				if e.IsDir() {
					names = append(names, e.Name())
				}
			}
			return fmt.Sprintf("Available network skills (%d):\n%s", len(names), strings.Join(names, "\n")), nil
		}
		return string(data), nil
	}

	// Load a specific skill.
	skillPath := filepath.Join(skillsDir, skill, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		// Try fuzzy match.
		entries, _ := os.ReadDir(skillsDir)
		var matches []string
		for _, e := range entries {
			if e.IsDir() && strings.Contains(strings.ToLower(e.Name()), strings.ToLower(skill)) {
				matches = append(matches, e.Name())
			}
		}
		if len(matches) > 0 {
			return fmt.Sprintf("Skill %q not found. Did you mean one of these?\n%s", skill, strings.Join(matches, "\n")), nil
		}
		return "", fmt.Errorf("skill %q not found in network catalog", skill)
	}

	return string(data), nil
}
