package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
)

// SlideDeck represents the structure of a presentation.
type SlideDeck struct {
	Title  string  `json:"title"`
	Theme  string  `json:"theme"` // e.g. "black", "white", "league", "dracula"
	Slides []Slide `json:"slides"`
}

// Slide represents a single slide.
type Slide struct {
	Title   string `json:"title"`
	Content string `json:"content"` // HTML content
	Notes   string `json:"notes,omitempty"` // Speaker notes
}

// SlideGenerateTool creates HTML presentations using Reveal.js.
type SlideGenerateTool struct {
	DataDir string
	Bus     *eventbus.EventBus // Optional: publishes artifact.created events for Canvas auto-reload.
}

func (s *SlideGenerateTool) Name() string { return "slide_generate" }

func (s *SlideGenerateTool) Description() string {
	return "Generate an interactive HTML slide deck (using Reveal.js) from a structured JSON input."
}

func (s *SlideGenerateTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "Name of the output file (without .html extension)",
			},
			"deck_json": map[string]interface{}{
				"type":        "string",
				"description": "JSON string representing the SlideDeck structure: {\"title\": \"...\", \"theme\": \"dracula\", \"slides\": [{\"title\": \"...\", \"content\": \"...\"}]}",
			},
		},
		"required": []string{"filename", "deck_json"},
	}
}

func (s *SlideGenerateTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	filename, ok := args["filename"].(string)
	if !ok || filename == "" {
		return "", fmt.Errorf("missing or invalid filename")
	}

	deckJSONStr, ok := args["deck_json"].(string)
	if !ok || deckJSONStr == "" {
		return "", fmt.Errorf("missing or invalid deck_json")
	}

	var deck SlideDeck
	if err := json.Unmarshal([]byte(deckJSONStr), &deck); err != nil {
		return "", fmt.Errorf("failed to parse deck_json: %v", err)
	}

	if deck.Theme == "" {
		deck.Theme = "dracula" // default theme
	}

	// Generate HTML content.
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">

	<title>%s</title>

	<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/reveal.js/4.5.0/reset.css">
	<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/reveal.js/4.5.0/reveal.css">
	<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/reveal.js/4.5.0/theme/%s.css">
	<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/reveal.js/4.5.0/plugin/highlight/monokai.css">

	<!-- iTaK Eco Branding injected -->
	<style>
		.reveal { font-family: 'Inter', sans-serif; }
		.reveal h1, .reveal h2, .reveal h3, .reveal h4, .reveal h5, .reveal h6 {
			font-family: 'Inter', sans-serif;
			font-weight: 700;
			letter-spacing: -0.02em;
		}
		/* Custom iTaK accent */
		.reveal a { color: #00d2ff; }
	</style>
</head>
<body>
	<div class="reveal">
		<div class="slides">
			<section>
				<h1>%s</h1>
				<h3>iTaK Eco Generated Presentation</h3>
			</section>`, deck.Title, deck.Theme, deck.Title)

	for _, slide := range deck.Slides {
		html += fmt.Sprintf("\n\t\t\t<section>\n\t\t\t\t<h2>%s</h2>\n\t\t\t\t%s", slide.Title, slide.Content)
		if slide.Notes != "" {
			html += fmt.Sprintf("\n\t\t\t\t<aside class=\"notes\">%s</aside>", slide.Notes)
		}
		html += "\n\t\t\t</section>"
	}

	html += `
		</div>
	</div>

	<script src="https://cdnjs.cloudflare.com/ajax/libs/reveal.js/4.5.0/reveal.js"></script>
	<script src="https://cdnjs.cloudflare.com/ajax/libs/reveal.js/4.5.0/plugin/notes/notes.js"></script>
	<script src="https://cdnjs.cloudflare.com/ajax/libs/reveal.js/4.5.0/plugin/markdown/markdown.js"></script>
	<script src="https://cdnjs.cloudflare.com/ajax/libs/reveal.js/4.5.0/plugin/highlight/highlight.js"></script>
	<script>
		Reveal.initialize({
			hash: true,
			plugins: [ RevealMarkdown, RevealHighlight, RevealNotes ]
		});
	</script>
</body>
</html>`

	// Ensure output directory exists.
	outDir := filepath.Join(s.DataDir, "slides")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %v", outDir, err)
	}

	outPath := filepath.Join(outDir, filename+".html")
	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %v", outPath, err)
	}

	// Publish artifact.created event for Canvas auto-reload.
	if s.Bus != nil {
		s.Bus.Publish(eventbus.Event{
			Topic:   eventbus.TopicArtifactCreated,
			Tool:    "slide_generate",
			Message: deck.Title,
			Data: map[string]interface{}{
				"type":     "slide_deck",
				"filename": filename + ".html",
				"url":      "/slides/" + filename + ".html",
				"title":    deck.Title,
			},
		})
	}

	return fmt.Sprintf("Successfully generated slide deck: %s", outPath), nil
}
