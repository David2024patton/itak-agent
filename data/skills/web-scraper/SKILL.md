---
name: web-scraper
description: Extract structured data from web pages using browser tools
tags: [web, scraping, research, browser]
---

# Web Scraper Skill

Use this skill to extract data from web pages.

## Tools Available

- **web_navigate** — Browse to any URL and get the page title + text content
- **web_screenshot** — Capture a page as a PNG screenshot (saved to `data/screenshots/`)
- **web_extract** — Extract specific elements using CSS selectors

## Workflow

1. **Navigate first**: Use `web_navigate` with the target URL to see the full page content
2. **Extract if needed**: If you need specific elements, use `web_extract` with a CSS selector
3. **Screenshot if needed**: Use `web_screenshot` to capture a visual snapshot

## Example CSS Selectors

| Selector | What it matches |
|----------|----------------|
| `h1` | Main headings |
| `p` | All paragraphs |
| `.title` | Elements with class "title" |
| `#content` | Element with id "content" |
| `a[href]` | All links |
| `table tr td` | Table cells |
| `article h2` | h2 inside article tags |

## Tips

- `web_navigate` returns truncated text (~4000 chars) to fit LLM context
- `web_extract` returns up to 20 matching elements
- Pages are loaded in headless Chromium (auto-downloaded by rod)
- Works with JavaScript-rendered pages (SPA, React, etc.)
