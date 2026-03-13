---
name: frontend-design
description: Create distinctive, production-grade web interfaces with high design quality. Use when building web components, pages, dashboards, or applications. Enforces mandatory competitor research before coding, design token systems, and a structured 5-phase workflow. Adapted from Agent Zero's frontend-design skill.
---

# Frontend Design Skill

Five-phase process: **Research -> Design -> Implement -> Verify -> User Review.**

## Project Type Awareness

Not every project has a UI. Adapt the process based on the type:

| Project Type | Research Phase | Design Phase | Screenshot Review |
|-------------|---------------|--------------|-------------------|
| Website / web app | Full competitor research + screenshots | Visual design | Screenshot to user |
| Dashboard / admin panel | Research similar tools | Visual design | Screenshot to user |
| Discord / Telegram bot | Research similar bots | No visual design | Test commands instead |
| API / backend service | Research similar APIs | No visual design | Test endpoints |
| CLI tool / script | Research similar tools | No visual design | Show terminal output |

For non-visual projects, skip Phases 0-1 visual steps and jump to implementation.

---

## Phase 0 - Competitor Research (MANDATORY for UI projects)

Complete this phase before writing any code for websites and web apps.

### Step 1: Identify the Category
Determine the business category: "pest control", "dance studio", "law firm", "SaaS dashboard", etc.

### Step 2: Search for Top Competitors
Use `search_web` to find real competitor websites:
- "best [category] websites 2026"
- "top [category] website design inspiration"
- "[category] near me" (for local businesses)

### Step 3: Screenshot Top 10 Competitors
Pick the 10 most relevant competitor URLs. Use `browser_subagent` to visit each and take screenshots.

Save observations about each site's design: layout, colors, fonts, hero section, CTA placement, navigation style.

### Step 4: Write a Research Brief
Create a research brief summarizing:
- **Common patterns**: What layout do 80% of top sites use?
- **Color trends**: What colors dominate in this industry? Why?
- **Typography**: What fonts do the best sites use?
- **Hero section**: What's above the fold?
- **Call-to-action**: Where are CTAs placed? What language do they use?
- **Standout designs**: Which 2-3 sites are clearly BEST? What makes them exceptional?
- **Differentiation opportunities**: What could we do BETTER than all of them?

### Step 5: Create Design Tokens
Before coding, create a design tokens file:

```css
:root {
    /* Colors - DERIVED FROM RESEARCH */
    --color-primary: #2d5a3d;      /* Why: green works for pest/outdoor brands */
    --color-secondary: #f5a623;    /* Why: orange CTAs stood out on competitor sites */
    --color-bg: #fafaf8;
    --color-text: #1a1a1a;
    --color-surface: #ffffff;
    --color-border: #e5e5e5;
    --color-muted: #6b7280;

    /* Typography - DISTINCTIVE CHOICES */
    --font-display: 'Playfair Display', serif;
    --font-body: 'Source Sans 3', sans-serif;

    /* Spacing */
    --space-xs: 0.25rem;
    --space-sm: 0.5rem;
    --space-md: 1rem;
    --space-lg: 2rem;
    --space-xl: 4rem;
    --space-2xl: 8rem;

    /* Animation */
    --duration-micro: 150ms;
    --duration-reveal: 400ms;
    --ease-standard: cubic-bezier(0.4, 0, 0.2, 1);
    --ease-bounce: cubic-bezier(0.34, 1.56, 0.64, 1);
}

/* Dark Mode Tokens */
@media (prefers-color-scheme: dark) {
    :root {
        --color-bg: #0f0f0f;
        --color-text: #e5e5e5;
        --color-surface: #1a1a1a;
        --color-border: #2e2e2e;
        --color-muted: #9ca3af;
        /* Primary/secondary stay the same or adjust for dark contrast */
    }
}

/* Manual toggle override */
[data-theme="dark"] {
    --color-bg: #0f0f0f;
    --color-text: #e5e5e5;
    --color-surface: #1a1a1a;
    --color-border: #2e2e2e;
    --color-muted: #9ca3af;
}
```

Always define both light and dark token variants. Use `prefers-color-scheme` as the default, with a `[data-theme]` attribute override for manual toggle switches.

---

## Phase 1 - Design Thinking

Before coding, commit to a BOLD aesthetic direction informed by research:

- **Purpose**: What problem does this interface solve? Who uses it?
- **Tone**: Based on competitor analysis, pick a direction: brutally minimal, maximalist, retro-futuristic, organic/natural, luxury/refined, playful, editorial, brutalist, art deco, soft/pastel, industrial.
- **Differentiation**: What will make THIS site better than the top 3?

### Aesthetics Guidelines

- **Typography**: Beautiful and distinctive. NEVER use generic fonts (Arial, system fonts). Pair a display font with a body font.
- **Color**: Dominant colors with sharp accents outperform timid, evenly-distributed palettes.
- **Motion**: CSS animations for reveal effects and micro-interactions. Staggered entry animations. Scroll-triggered reveals. Hover states that surprise.
  - **Duration standards**: 150-200ms for micro-interactions (hover, toggle), 300-500ms for reveals and transitions
  - **Easing**: Use `var(--ease-standard)` for most transitions, `var(--ease-bounce)` for playful elements
  - **Reduced motion**: Always wrap non-essential animations in `@media (prefers-reduced-motion: no-preference)`
  - **Reusable classes**: Create `.fade-in`, `.slide-up`, `.stagger-children` utilities instead of ad-hoc keyframes
- **Spatial**: Unexpected layouts. Asymmetry. Overlap. Grid-breaking elements. Generous negative space OR controlled density.
- **Backgrounds**: Gradient meshes, noise textures, geometric patterns, layered transparencies, dramatic shadows. Create atmosphere, not flat white pages.

### SEO Requirements (every page)
- Proper `<title>` tag with business name + location + service
- `<meta name="description">` with compelling summary
- Single `<h1>` per page with keyword-rich heading
- Semantic HTML5 elements
- `alt` text on all images
- Open Graph meta tags
- Favicon

### Accessibility Standards (every page)
- **WCAG 2.1 AA minimum** - this is the bar, not optional
- Color contrast ratio: 4.5:1 for normal text, 3:1 for large text (18px+ bold or 24px+)
- All interactive elements reachable via keyboard (Tab/Shift+Tab/Enter/Space/Escape)
- Visible focus indicators on every focusable element - style them, don't hide them
- ARIA labels on icons, image buttons, and any element whose purpose isn't obvious from text
- Skip-to-content link as the first focusable element
- `prefers-reduced-motion` media query wrapping all non-essential animations
- Form inputs have associated `<label>` elements (not just placeholder text)
- Error messages linked to inputs via `aria-describedby`

### Performance Standards
- **Core Web Vitals targets**: LCP < 2.5s, CLS < 0.1, INP < 200ms
- `font-display: swap` on all `@font-face` declarations
- Lazy load images below the fold: `loading="lazy"`
- Preload critical assets: `<link rel="preload">` for hero images and display fonts
- Prefer WebP/AVIF over PNG/JPEG. Use `<picture>` with fallbacks when needed
- Responsive images with `srcset` and `sizes` attributes
- Minimize render-blocking CSS - inline critical CSS for above-the-fold content
- SVG for icons and logos (not icon fonts)

---

## Phase 2 - Implementation

Build order:
1. **Design tokens** - CSS variables file (from Phase 0)
2. **Layout shell** - Header, nav, footer, page structure
3. **Hero section** - Most impactful section, informed by research
4. **Core sections** - Services, about, testimonials, contact
5. **Responsive design** - Mobile-first: 375px, 768px, 1024px, 1440px
6. **Animations** - Scroll reveals, hover effects, transitions (use token durations/easings)
7. **Forms** - Validation UX, error/success states, accessible labels
8. **Images** - Use `generate_image` for hero images, icons, backgrounds
9. **Dark mode** - Verify all sections in both light/dark token sets
10. **Polish** - Spacing, alignment, consistency pass

### Form Design Patterns
- Inline validation on blur, not on every keystroke
- Error states: red border + icon + descriptive message below the input
- Success states: green check on validated fields
- Use appropriate mobile input types: `type="tel"`, `type="email"`, `type="url"`, `inputmode="numeric"`
- Auto-focus first input on page load or modal open
- Group related fields with `<fieldset>` and `<legend>`
- Submit buttons show loading state during async operations
- Never rely solely on color to communicate state (use icons + text too)

### Anti-Patterns (NEVER DO)
- Generic AI aesthetics: purple gradients on white, Inter font, rounded cards in a grid
- Cookie-cutter layouts with no context-specific character
- Placeholder text without real copy
- Missing mobile responsiveness
- Missing favicons, meta tags, or semantic HTML
- Same design every time - VARY themes, fonts, colors between projects

---

## Phase 3 - Quality Verification

After building:
1. **Compare against research**: Does your site look BETTER than the top 3 competitors?
2. **Mobile check**: Resize to 375px. Does it work?
3. **Dark mode check**: Toggle to dark - are all elements visible and properly styled?
4. **First impression test**: Would someone remember this site 5 minutes later?
5. **CTA clarity**: Is it obvious what the user should DO?
6. **Accessibility spot-check**: Tab through the page. Can you reach everything? Are focus rings visible?
7. **Browser compatibility**: Test in Chrome + Safari at minimum. Watch for:
   - Flexbox gap support differences
   - Safari scrollbar styling quirks
   - `backdrop-filter` vendor prefix needs (`-webkit-backdrop-filter`)
   - Font rendering differences between platforms
8. If any answer is "no" - go back and improve before delivering.

---

## Phase 4 - User Review Loop

After building a visual project, send a screenshot back to the user:
1. Use `browser_subagent` to screenshot the built site
2. Present the screenshot and ask for feedback
3. User can describe changes verbally or annotate the screenshot
4. Implement changes, screenshot again, repeat until approved
