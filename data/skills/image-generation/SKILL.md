---
name: image-generation
description: Generate images for projects using the built-in generate_image tool, ComfyUI, or Google Gemini. Use when assets are needed for games, websites, social media cards, or any visual content.
---

# Image Generation Pipeline

Generate production-quality images for any project. Never use placeholder images.

## Tool Selection

| Need | Tool | When |
|------|------|------|
| Quick assets, UI mockups, icons | `generate_image` (built-in) | Default choice for most needs |
| Complex scenes, style control | ComfyUI (local) | When specific art styles or workflows are needed |
| Photo-realistic, text in images | Google Gemini | When photorealism or embedded text is required |

## Built-in generate_image Tool

### Best Practices
- Use descriptive, specific prompts - not vague instructions
- Include art style keywords: "flat design", "isometric", "photorealistic", "watercolor"
- Specify background: "transparent background", "dark background", "white background"
- Include dimensions context: "square format", "wide banner", "portrait"
- For game assets: include "game art", "trading card", "pack image" in prompt
- Name files descriptively: `hero_banner.png`, not `image1.png`

### Prompt Template
```
[Subject description], [art style], [lighting/mood], [background], [format hint]
```

### Examples
```
"A fierce velociraptor pack hunting in misty jungle, realistic digital art, dramatic lighting, dark moody background, wide format"

"Modern dark-themed dashboard UI with data visualization charts, glassmorphism style, purple accent glow, screenshot format"

"Professional benchmark comparison card showing performance metrics, clean infographic design, dark background with neon accents"
```

## ComfyUI (Advanced)
- Running on local infrastructure or VPS
- Access via API at configured port
- Use for batch generation of game assets (dinosaur packs, card art)
- Workflow files stored in ComfyUI workspace

## Gemini Image Generation
- Available through Open WebUI (iTaK Chat) with Gemini integration
- Best for: images with readable text, photorealistic scenes
- Use the native Google Gemini image generation pipeline

## Asset Management
- Save generated images to the project's `/assets/` directory
- Use consistent naming: `<category>/<name>_<variant>.png`
- For game projects: `/assets/dinos/packs/<species_name>.png`
- Always verify generated images by viewing them before committing
