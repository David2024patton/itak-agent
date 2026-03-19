package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// WorkflowTemplate defines a predefined multi-agent pipeline for common
// request types. This lets tiny models (2B-4B params) route requests
// correctly without needing to produce structured JSON delegation decisions.
// The orchestrator checks these templates BEFORE calling the LLM.
type WorkflowTemplate struct {
	Name     string         // human-readable name
	Keywords []string       // trigger keywords (any match fires)
	Phrases  []string       // trigger phrases (substring match)
	Steps    []WorkflowStep // ordered agent pipeline
}

// WorkflowStep is a single step in a workflow pipeline.
type WorkflowStep struct {
	Agent     string // agent to delegate to
	Task      string // task template (%s = original user message)
	Context   string // context template
	SkillFile string // optional: path to SKILL.md to inject as context
	Swarm     bool   // if true, spawn parallel workers for sub-tasks
	// SwarmPages lists pages to generate in parallel (used with Swarm=true).
	// Each entry becomes a separate worker task. The shared CSS/layout from
	// the previous pipeline step is injected into each worker's context.
	SwarmPages []SwarmPage
}

// SwarmPage defines a single page to generate in a swarm step.
type SwarmPage struct {
	Filename    string // output filename (e.g., "index.html")
	Title       string // page title for the model
	Description string // what this page should contain
}

// workflowTemplates are the built-in workflow patterns.
// When a user request matches keywords/phrases, the orchestrator skips the
// LLM routing call and uses this predefined pipeline instead.
//
// Website Build uses a multi-step pipeline with swarm parallelism:
//   Step 1: Researcher - competitive analysis of top 10 sites
//   Step 2: Coder - generate shared CSS/layout/nav template
//   Step 3: Coder (swarm) - generate all pages in parallel using shared CSS
//   Step 4: Reviewer - lint, check SEO, mobile, accessibility
var workflowTemplates = []WorkflowTemplate{
	{
		Name:     "Website Build",
		Keywords: []string{"website", "web site", "landing page", "web app", "webapp"},
		Phrases:  []string{"build me", "create a", "make me", "build a"},
		Steps: []WorkflowStep{
			// Step 1: Research - find top competitors and extract design patterns.
			{
				Agent: "researcher",
				Task:  "Research the top 10 businesses for: %s",
				Context: `RESEARCH TASK: Find the top 10 competitor websites in this business category.

For each competitor, note:
- Company name and URL
- Color scheme (primary, secondary, accent colors)
- Typography choices (fonts used)
- Layout style (hero section, navigation, CTA placement)
- Unique features or design elements that stand out
- What pages they have (services, about, FAQ, testimonials, etc.)

After reviewing all 10, write a RESEARCH BRIEF with:
1. COMMON PATTERNS: What layout/colors do 80% of top sites use?
2. COLOR TRENDS: What colors dominate? What hex codes?
3. TYPOGRAPHY: What fonts do the best sites use?
4. HERO SECTION: What content is above the fold?
5. CALL TO ACTION: Where are CTAs? What text do they use?
6. TOP 3 BEST SITES: Which are clearly best and why?
7. DIFFERENTIATION: What could we do BETTER than all of them?
8. RECOMMENDED PAGES: What pages should this site have?

Output a structured text brief. This will be passed to the coder.`,
			},
			// Step 2: Generate shared CSS design system + layout template.
			// This runs sequentially BEFORE the page swarm so all pages share
			// the same design tokens, nav, and footer.
			{
				Agent: "coder",
				Task:  "Generate the CSS design system and shared layout for: %s",
				Context: `OUTPUT RULES: Output ONLY code. No descriptions, no explanations.

You MUST generate these files:
1. css/tokens.css - Design tokens (colors, fonts, spacing) derived from the research brief
2. css/style.css - Full design system (navbar, footer, grid, cards, forms, buttons, hero)
3. css/responsive.css - Mobile breakpoints (375px, 768px, 1024px)
4. js/nav.js - Hamburger menu toggle (closable nav for mobile)
5. js/help.js - Help icon tooltip system (show/hide on click)

Use the research brief to pick colors, fonts, and layout patterns that BEAT the competition.

MANDATORY DESIGN RULES:
- Use Google Fonts (link in HTML). Pick distinctive fonts, NOT system defaults.
- Dark mode support via CSS custom properties and prefers-color-scheme
- Hamburger nav must be closable (click X or outside to close)
- Help icons: <span class="help-icon" data-tooltip="Help text here">?</span>
- All animations wrapped in prefers-reduced-motion media query
- Skip-to-content link as first focusable element
- CSS Grid for layouts, Flexbox for components
- No AI slop: no generic purple gradients, no boring card grids

Each file MUST start with a comment marker. Use these EXACT formats:
<!-- css/tokens.css -->
<!-- css/style.css -->
<!-- css/responsive.css -->
<!-- js/nav.js -->
<!-- js/help.js -->

START NOW with css/tokens.css.`,
				SkillFile: "/app/data/skills/frontend-website/SKILL.md",
			},
			// Step 3: SWARM - generate all pages in parallel.
			// Each page gets the shared CSS from Step 2 injected into its context.
			// Workers run concurrently, one per page.
			{
				Agent: "coder",
				Task:  "Generate page: %s",
				Swarm: true,
				Context: `OUTPUT RULES: Output ONLY the HTML code for this ONE page. No descriptions.

The CSS files (tokens.css, style.css, responsive.css) and JS files (nav.js, help.js) 
already exist from the previous step. Link to them in your HTML head:
<link rel="stylesheet" href="css/tokens.css">
<link rel="stylesheet" href="css/style.css">
<link rel="stylesheet" href="css/responsive.css">

Every page MUST include:
- <!DOCTYPE html> with lang attribute
- <meta charset="UTF-8"> and <meta name="viewport">
- <meta name="description"> with SEO-optimized content
- <title> with "Page Name | Business Name"
- Open Graph meta tags (og:title, og:description, og:type)
- Schema.org JSON-LD structured data block
- Skip-to-content link
- Shared navigation bar (same on every page)
- Help icons on interactive elements: <span class="help-icon" data-tooltip="...">?</span>
- Footer with business info, social links, quick nav
- Semantic HTML5 (header, main, section, article, footer, nav)

Start the file with the filename comment marker: <!-- FILENAME -->
Then output the complete HTML.`,
				SwarmPages: []SwarmPage{
					{Filename: "index.html", Title: "Home Page", Description: "Hero section with main CTA, brief overview of services, testimonials preview, and call to action"},
					{Filename: "about.html", Title: "About Us", Description: "Company story, team section, mission/values, years of experience, certifications"},
					{Filename: "services.html", Title: "Services", Description: "Service cards in a grid, each with icon, title, description, and 'Learn More' CTA"},
					{Filename: "contact.html", Title: "Contact", Description: "Contact form (name, email, phone, message), business address, phone, email, map placeholder, hours of operation"},
					{Filename: "faq.html", Title: "FAQ", Description: "Accordion-style FAQ with 8-10 common questions, expandable answers with smooth transitions"},
					{Filename: "sitemap.html", Title: "Sitemap", Description: "Clean sitemap page listing all pages with links, organized by section"},
				},
			},
			// Step 4: Review - check quality, SEO, accessibility, mobile.
			{
				Agent: "researcher",
				Task:  "Review the generated website code for: %s",
				Context: `CODE REVIEW TASK: Check the generated website files for quality issues.

Check for:
1. SEO: Does every page have <title>, <meta description>, Open Graph tags, schema.org?
2. MOBILE: Is there a responsive CSS file? Does the nav have a hamburger menu?
3. ACCESSIBILITY: Skip-to-content link? ARIA labels? Focus indicators? Alt text?
4. HELP ICONS: Are there help-icon tooltips on interactive elements?
5. CONSISTENCY: Does every page use the same nav, footer, and CSS?
6. PERFORMANCE: Are fonts loaded with font-display:swap? Lazy loading on images?
7. PAGES: Are all mandatory pages present (home, about, services, contact, FAQ, sitemap)?
8. DARK MODE: Does CSS support prefers-color-scheme?

Output a REVIEW REPORT with:
- PASS items (things that look good)
- FAIL items (things that need fixing)
- SUGGESTIONS (improvements, not blockers)

Be specific. Quote the actual code that has issues.`,
			},
		},
	},
	{
		Name:     "Code Project",
		Keywords: []string{"application", "program", "script", "tool", "cli", "api"},
		Phrases:  []string{"build me", "create a", "make me", "write a", "code a"},
		Steps: []WorkflowStep{
			{
				Agent:   "coder",
				Task:    "Build this project: %s",
				Context: "Write the complete code. Output each file with a comment marker on the line BEFORE the code. Use <!-- filename.ext --> for all file types. Example:\n<!-- main.go -->\npackage main\n...\n\n<!-- README.md -->\n# Project Title\n...",
			},
		},
	},
	{
		Name:     "Research Report",
		Keywords: []string{"research", "report", "analysis", "investigate", "compare"},
		Phrases:  []string{"find out", "look into", "tell me about", "what is", "how does"},
		Steps: []WorkflowStep{
			{
				Agent:   "researcher",
				Task:    "Research thoroughly: %s",
				Context: "Gather information from multiple sources. Include facts, statistics, and citations.",
			},
		},
	},
	{
		Name:     "File Operations",
		Keywords: []string{"file", "folder", "directory", "list", "find", "show me"},
		Phrases:  []string{"what files", "list my", "show me the", "check my", "look at"},
		Steps: []WorkflowStep{
			{
				Agent:   "scout",
				Task:    "%s",
				Context: "Explore the filesystem and report what you find.",
			},
		},
	},
}

// matchWorkflow checks if a user message matches any predefined workflow.
// Returns the matched template and true if found, nil and false otherwise.
func matchWorkflow(message string) (*WorkflowTemplate, bool) {
	lower := strings.ToLower(message)

	for i := range workflowTemplates {
		tpl := &workflowTemplates[i]

		// Check phrases first (more specific).
		hasPhrase := false
		for _, phrase := range tpl.Phrases {
			if strings.Contains(lower, phrase) {
				hasPhrase = true
				break
			}
		}

		// Check keywords.
		hasKeyword := false
		for _, kw := range tpl.Keywords {
			if strings.Contains(lower, kw) {
				hasKeyword = true
				break
			}
		}

		// Match requires BOTH a phrase AND a keyword (for build/create workflows)
		// OR just a keyword (for simpler workflows like research, files).
		if hasPhrase && hasKeyword {
			debug.Info("workflow", "Matched template %q (phrase+keyword)", tpl.Name)
			return tpl, true
		}

		// For single-step workflows (research, files), keyword alone is enough.
		if hasKeyword && len(tpl.Steps) == 1 {
			debug.Info("workflow", "Matched template %q (keyword only)", tpl.Name)
			return tpl, true
		}
	}

	return nil, false
}

// buildWorkflowDelegation creates a Delegation from a matched workflow template.
// If a step has a SkillFile, the skill's body is loaded and injected into context.
// Swarm steps are expanded into individual task payloads (one per SwarmPage).
//
// For tiny models: they get the template IN their prompt instead of needing
// to call skill_load (which they can't do reliably).
func buildWorkflowDelegation(tpl *WorkflowTemplate, userMessage string) *Delegation {
	payloads := make([]TaskPayload, 0, len(tpl.Steps)*2)
	for i, step := range tpl.Steps {
		context := step.Context

		// Load skill file and inject its body into context.
		if step.SkillFile != "" {
			body, err := loadSkillBody(step.SkillFile)
			if err != nil {
				debug.Warn("workflow", "Failed to load skill %q: %v", step.SkillFile, err)
			} else {
				context = context + "\n\n--- TEMPLATE ---\n" + body
				debug.Info("workflow", "Injected skill template %q (%d chars) into step %d context",
					step.SkillFile, len(body), i+1)
			}
		}

		if step.Swarm && len(step.SwarmPages) > 0 {
			// Swarm step: create one task per page. These will be executed
			// in parallel by the orchestrator's swarm executor.
			for _, page := range step.SwarmPages {
				pageContext := context
				// Replace FILENAME placeholder in context.
				pageContext = strings.ReplaceAll(pageContext, "FILENAME", page.Filename)

				taskDesc := fmt.Sprintf("Generate %s (%s): %s. %s",
					page.Filename, page.Title, page.Description, userMessage)

				payloads = append(payloads, TaskPayload{
					Agent:   step.Agent,
					Task:    taskDesc,
					Context: pageContext,
					Swarm:   true, // Mark for parallel execution.
				})
			}
			debug.Info("workflow", "Swarm step %d: expanded into %d parallel tasks", i+1, len(step.SwarmPages))
		} else {
			// Normal sequential step.
			payloads = append(payloads, TaskPayload{
				Agent:   step.Agent,
				Task:    fmt.Sprintf(step.Task, userMessage),
				Context: context,
			})
		}
	}
	return &Delegation{
		Reasoning:   fmt.Sprintf("Matched workflow template: %s", tpl.Name),
		Delegations: payloads,
	}
}

// loadSkillBody reads a SKILL.md file and returns the body (after frontmatter).
func loadSkillBody(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read skill file: %w", err)
	}

	content := string(data)

	// Strip YAML frontmatter (between --- delimiters).
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			content = strings.TrimSpace(parts[2])
		}
	}

	return content, nil
}

// extractCodeFiles parses model output for code blocks and their filenames.
// Handles multiple filename patterns for maximum compatibility with tiny model output.
//
// Pattern 1 (fenced, before): <!-- index.html -->\n```html\n...code...\n```
// Pattern 2 (fenced, inside): ```html\n<!-- filename: index.html -->\n...code...\n```
// Pattern 3 (unfenced): <!-- index.html -->\n...raw code...\n<!-- css/style.css -->\n...
//
// Pattern 3 is the most common for tiny models. It handles ALL file types
// with the unified <!-- path/filename.ext --> marker format.
func extractCodeFiles(output string) map[string]string {
	files := make(map[string]string)

	// Pattern 1: filename comment on line BEFORE a fenced code block.
	beforeBlockRe := regexp.MustCompile(`(?s)<!--\s*([\w./-]+\.\w+)\s*-->\s*\n` + "```" + `\w*\s*\n(.*?)\n` + "```")
	for _, m := range beforeBlockRe.FindAllStringSubmatch(output, -1) {
		if len(m) >= 3 {
			filename := strings.TrimSpace(m[1])
			code := m[2]
			files[filename] = code
			debug.Debug("workflow", "Extracted file %q (%d bytes) [fenced-before]", filename, len(code))
		}
	}
	if len(files) > 0 {
		return files
	}

	// Pattern 2: filename comment INSIDE a fenced code block (first line).
	codeBlockRe := regexp.MustCompile("(?s)```\\w*\\s*\\n(.*?)\\n```")
	insidePatterns := []*regexp.Regexp{
		regexp.MustCompile(`<!--\s*filename:\s*(.+?)\s*-->`),
		regexp.MustCompile(`/\*\s*filename:\s*(.+?)\s*\*/`),
		regexp.MustCompile(`//\s*filename:\s*(.+)`),
		regexp.MustCompile(`#\s*filename:\s*(.+)`),
	}
	for _, match := range codeBlockRe.FindAllStringSubmatch(output, -1) {
		if len(match) < 2 {
			continue
		}
		code := match[1]
		firstLine := strings.SplitN(code, "\n", 2)[0]
		for _, pattern := range insidePatterns {
			if m := pattern.FindStringSubmatch(firstLine); len(m) >= 2 {
				filename := strings.TrimSpace(m[1])
				rest := ""
				if parts := strings.SplitN(code, "\n", 2); len(parts) == 2 {
					rest = parts[1]
				}
				files[filename] = rest
				debug.Debug("workflow", "Extracted file %q (%d bytes) [fenced-inside]", filename, len(rest))
				break
			}
		}
	}
	if len(files) > 0 {
		return files
	}

	// Pattern 3: Unfenced output with unified <!-- filename --> markers.
	// The model outputs: <!-- path/filename.ext --> followed by raw code
	// until the next marker. This handles ALL file types (HTML, CSS, JS, etc.)
	// with a single regex pattern.
	markerRe := regexp.MustCompile(`(?m)^<!--\s*([\w./-]+\.\w+)\s*-->`)
	locs := markerRe.FindAllStringSubmatchIndex(output, -1)

	for i, loc := range locs {
		// Extract the filename.
		filename := strings.TrimSpace(output[loc[2]:loc[3]])
		if filename == "" {
			continue
		}

		// Code starts after the marker line.
		codeStart := loc[1]
		// Code ends at the next marker or end of string.
		codeEnd := len(output)
		if i+1 < len(locs) {
			codeEnd = locs[i+1][0]
		}

		code := strings.TrimSpace(output[codeStart:codeEnd])
		if len(code) > 0 {
			files[filename] = code
			debug.Debug("workflow", "Extracted file %q (%d bytes) [unfenced]", filename, len(code))
		}
	}

	// Fallback: also check for /* path/file.ext */ markers (some models use these for CSS/JS).
	if len(files) == 0 {
		cssJsMarkerRe := regexp.MustCompile(`(?m)^/\*\s*([\w./-]+\.(?:css|js))\s*\*/`)
		cssJsLocs := cssJsMarkerRe.FindAllStringSubmatchIndex(output, -1)
		for i, loc := range cssJsLocs {
			filename := strings.TrimSpace(output[loc[2]:loc[3]])
			if filename == "" {
				continue
			}
			codeStart := loc[1]
			codeEnd := len(output)
			if i+1 < len(cssJsLocs) {
				codeEnd = cssJsLocs[i+1][0]
			}
			code := strings.TrimSpace(output[codeStart:codeEnd])
			if len(code) > 0 {
				files[filename] = code
				debug.Debug("workflow", "Extracted file %q (%d bytes) [css-js-marker]", filename, len(code))
			}
		}
	}

	return files
}

// writeCodeFiles writes extracted code files to a project directory.
func writeCodeFiles(projectDir string, files map[string]string) error {
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}

	for filename, content := range files {
		filePath := filepath.Join(projectDir, filename)

		// Create subdirectories if needed (e.g., css/style.css).
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			debug.Warn("workflow", "Failed to create dir %s: %v", dir, err)
			continue
		}

		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			debug.Warn("workflow", "Failed to write %s: %v", filePath, err)
			continue
		}

		debug.Info("workflow", "Wrote file: %s (%d bytes)", filePath, len(content))
	}

	return nil
}
