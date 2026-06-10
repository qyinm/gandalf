# Hem Design Theme

> Design tokens and visual identity for Hem — landing page, docs, and web dashboard.
> Last updated: 2026-06-10

---

## Color Palette

```yaml
design:
  mode: light
  colors:
    # ── Surfaces (mono ramp) ──
    background: "#ffffff"          # Page background
    surface: "#fafafa"             # Card/section surface (near-white)
    surface-hover: "#f4f4f5"       # Surface hover (zinc 100)
    foreground: "#0a0a0a"          # Primary text (near-black)
    foreground-soft: "#18181b"     # Soft black (zinc 900)

    # ── Brand (appears in TWO places only) ──
    brand: "#FF5F6A"              # Nav logo dot + terminal `$` sigil. Nothing else.

    # ── Neutrals ──
    muted: "#f4f4f5"              # Muted background (zinc 100)
    muted-foreground: "#71717a"   # Muted text (zinc 500)
    border: "#e4e4e7"             # Borders, dividers (zinc 200)
    border-strong: "#d4d4d8"      # Strong borders (zinc 300)
    ring: "#0a0a0a"               # Focus rings (black, not brand)

    # ── Semantic (used in docs, not landing) ──
    success: "#16a34a"            # Success / green 600
    warning: "#ca8a04"            # Warning / amber 600
    destructive: "#dc2626"        # Error / red 600
```

### Usage rules

| Token | Where |
|-------|-------|
| `brand` | **Two surfaces only** — nav logo dot, terminal `$` sigil. Nothing else. |
| `foreground` | All body text, primary button backgrounds, focus rings |
| `muted-foreground` | Secondary text, meta info, timestamps, captions, nav links |
| `border` | Card borders, dividers, input borders, section hairlines |
| `surface` | Subtle card backgrounds, code block backgrounds |

The page is editorial — black on white — not branded. Primary buttons are `bg: var(--foreground)` (black on white). The brand color is a stamp, not a system.

---

## Typography

```yaml
design:
  typography:
    font-family: "'Inter', system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    heading: "'Playfair Display', Georgia, 'Times New Roman', serif"
    mono: "'DM Mono', 'Fira Code', 'JetBrains Mono', ui-monospace, monospace"
```

### Scale

| Token | Size | Weight | Line-Height | Font |
|-------|------|--------|-------------|------|
| `h1` | 3rem (48px) | 700 | 1.1 | Playfair Display |
| `h2` | 2rem (32px) | 700 | 1.2 | Playfair Display |
| `h3` | 1.5rem (24px) | 600 | 1.3 | Playfair Display |
| `h4` | 1.25rem (20px) | 600 | 1.4 | Inter |
| `body` | 1rem (16px) | 400 | 1.65 | Inter |
| `body-sm` | 0.875rem (14px) | 400 | 1.5 | Inter |
| `caption` | 0.75rem (12px) | 500 | 1.4 | Inter |
| `code` | 0.875rem (14px) | 400 | 1.6 | DM Mono |

### Usage rules

- **Headings** (h1-h3): Playfair Display, italic optional for emphasis on hero phrases. Use `font-synthesis: none` to prevent faux italic/bold.
- **Body**: Inter, regular weight. Use 500 for bold, 600 for strong emphasis.
- **Code / Terminal**: DM Mono. Terminal prompt `$` uses brand color (#FF5F6A).

---

## Spacing

```yaml
design:
  spacing:
    px: "1px"
    0: "0"
    0.5: "0.125rem"  # 2px
    1: "0.25rem"     # 4px
    2: "0.5rem"      # 8px
    3: "0.75rem"     # 12px
    4: "1rem"        # 16px
    5: "1.25rem"     # 20px
    6: "1.5rem"      # 24px
    8: "2rem"        # 32px
    10: "2.5rem"     # 40px
    12: "3rem"       # 48px
    16: "4rem"       # 64px
    20: "5rem"       # 80px
    24: "6rem"       # 96px
```

### Section rhythm

| Section | Top/Bottom Padding |
|---------|-------------------|
| Hero | `padding: 5rem 0 4rem` |
| Section (Services, Install, etc.) | `padding: 4rem 0` |
| Philosophy grid | `padding: 3rem 0` |
| Footer | `padding: 3rem 0 1.5rem` |

---

## Border Radius

```yaml
design:
  radius:
    none: "0"
    sm: "0.375rem"   # 6px
    md: "0.5rem"     # 8px
    lg: "0.75rem"    # 12px
    xl: "1rem"       # 16px
    full: "9999px"
```

| Element | Radius |
|---------|--------|
| Cards, sections | `lg` (12px) |
| Buttons | `md` (8px) |
| Terminal windows | `lg` (12px) |
| Code blocks | `sm` (6px) |
| Badges / tags | `full` |

---

## Shadows

```yaml
design:
  shadows:
    sm: "0 1px 2px 0 rgb(0 0 0 / 0.05)"
    md: "0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)"
    lg: "0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)"
    terminal: "0 8px 30px rgb(0 0 0 / 0.12)"
```

---

## Component Tokens

### Terminal Window

```
┌─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐
│ ● ● ●  [label]       │  ← bar: bg=#f1f5f9, border=#e2e8f0, dots=red/yellow/green
│                       │
│ $ command-here        │  ← body: bg=#ffffff, font=DM Mono, $=accent
│ $ another-command     │
└─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘
```

| Part | Token |
|------|-------|
| Bar background | `muted` (#f1f5f9) |
| Body background | `background` (#ffffff) |
| Border | `border` (#e2e8f0) |
| Radius | `lg` (12px) |
| Prompt sigil (`$`) | `brand` (#FF5F6A) |
| Command text | `foreground` (#0f172a) |

### Feature / Service Cards

```
┌──────────────────────┐
│ [icon]                │
│ Title                 │
│ Description text...   │
└──────────────────────┘
```

| Part | Token |
|------|-------|
| Background | `surface` (#f8fafc) |
| Hover background | `surface-hover` (#f1f5f9) |
| Border | `border` (#e2e8f0) |
| Icon color | `brand` (#FF5F6A) |
| Radius | `lg` (12px) |

### Buttons

| State | Primary | Outline |
|-------|---------|---------|
| Default | bg=`brand`, fg=`brand-foreground`, shadow=sm | bg=transparent, border=`border`, fg=`foreground` |
| Hover | bg=`brand-hover` | bg=`surface-hover`, border=`border-light` |
| Active | bg=`brand-hover`, inset shadow | bg=`muted` |
| Focus-visible | ring=`ring` (2px offset) | ring=`ring` (2px offset) |

---

## Layout

```yaml
design:
  layout:
    max-width: "1040px"     # Content container max width
    sidebar-width: "240px"  # Doc sidebar width
    gutter: "24px"          # Container horizontal padding (mobile: 16px)
    breakpoints:
      sm: "640px"
      md: "768px"
      lg: "1024px"
      xl: "1280px"
```

---

## Icons

Use [Lucide icons](https://lucide.dev/icons/) across all surfaces (landing, desktop, web dashboard).

| Context | Icon |
|---------|------|
| Brand | `Scissors` |
| Hero CTA | `ArrowRight` |
| Trust / Pledge | `Shield` |
| Services: Save | `Camera` |
| Services: Compare | `Search` |
| Services: Notes | `Shield` |
| Services: Inventory | `FolderTree` |
| Services: Restore | `History` |
| Services: Bundles | `Package` |
| Guide: Inspect | `Search` |
| Guide: Save | `Camera` |
| Guide: Compare | `Columns2` |
| Guide: Restore | `History` |
| Guide: Move | `Package` |
| Philosophy items | `ListTree`, `History`, `Columns2`, `History`, `Package` |
| Copy button | `Copy` → `Check` (on success) |
| GitHub nav link | `GitHubLogo` (SVG) |
| External link | `ExternalLink` |

Icon size: 18-20px for inline, 24-28px for feature cards.

---

## Code Blocks & Syntax

```yaml
design:
  code:
    theme: "github-light"       # Shiki syntax highlighting theme
    font: "DM Mono, ui-monospace, monospace"
    size: "0.875rem"
    background: "#f8fafc"
    inline-bg: "#f1f5f9"
    inline-radius: "4px"
```

---

## Responsive Behavior

| Breakpoint | Layout Changes |
|------------|---------------|
| `< 640px` | Single column. Nav collapses to hamburger. Terminal blocks stack. Font sizes reduce slightly. |
| `640-1024px` | 2-column service grid. Nav links visible but compact. |
| `≥ 1024px` | Full layout. 3-column service grid. Max-width container. Doc sidebar visible. |