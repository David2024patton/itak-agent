package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ══════════════════════════════════════════════════════════════════
// medge_skill  -  load a MedgeClaw / K-Dense scientific skill procedure
// ══════════════════════════════════════════════════════════════════
//
// What: Reads a specific biomedical skill's SKILL.md from the MedgeClaw catalog.
// Why:  182 skills are too large to inject into prompt. The agent loads
//       a specific skill on demand when it needs the procedure.
// How:  Reads data/medgeclaw/skills/{skill_name}/SKILL.md and returns it.

type MedgeSkillTool struct{}

func (t *MedgeSkillTool) Name() string { return "medge_skill" }
func (t *MedgeSkillTool) Description() string {
	return `Load a MedgeClaw / K-Dense biomedical scientific skill procedure (182 skills).
Use "list" to see all available skills, or provide a skill name to load its full procedure.

Categories: Bioinformatics (RNA-seq, scRNA-seq, DESeq2, Scanpy, AnnData, scvi-tools),
Drug Discovery (RDKit, TorchDrug, ZINC, AlphaFold, molecular docking, virtual screening),
Clinical (survival analysis, treatment plans, DICOM processing, clinical trials),
Multi-Omics (proteomics, metabolomics, pathway enrichment),
Scientific Communication (writing, literature review, PubMed, bioRxiv, figure generation),
Databases (UniProt, STRING, ChEMBL, USPTO, PDB), ML/Stats (scikit-learn, statsmodels, UMAP).

Examples: medge_skill("deseq2"), medge_skill("alphafold-database"), medge_skill("list")`
}

func (t *MedgeSkillTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"skill": map[string]interface{}{
				"type":        "string",
				"description": "Skill name (e.g. 'deseq2', 'alphafold-database', 'scientific-writing') or 'list' to see all available skills",
			},
		},
		"required": []string{"skill"},
	}
}

func (t *MedgeSkillTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	skill := argStr(args, "skill")
	if skill == "" {
		return "", fmt.Errorf("missing required argument: skill")
	}

	// Resolve skills directory relative to working directory.
	skillsDir := filepath.Join("data", "medgeclaw", "skills")

	// List mode.
	if strings.EqualFold(skill, "list") {
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			return "", fmt.Errorf("cannot read skills directory: %w", err)
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# MedgeClaw Scientific Skills (%d available)\n\n", len(names)))
		sb.WriteString("## Categories\n")
		sb.WriteString("Bioinformatics | Drug Discovery | Genomics | Clinical Research |\n")
		sb.WriteString("Multi-Omics | Medical Imaging | Scientific Writing | Databases |\n")
		sb.WriteString("ML/Statistics | Visualization | Time Series | Simulation\n\n")
		sb.WriteString("## All Skills\n")
		for i, n := range names {
			sb.WriteString(fmt.Sprintf("%3d. %s\n", i+1, n))
		}
		sb.WriteString("\n---\nUse medge_skill(\"skill-name\") to load any skill's full procedure.\n")
		return sb.String(), nil
	}

	// Load specific skill.
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
		return "", fmt.Errorf("skill %q not found in MedgeClaw catalog", skill)
	}

	return fmt.Sprintf("# Skill: %s\n\n%s", skill, string(data)), nil
}
