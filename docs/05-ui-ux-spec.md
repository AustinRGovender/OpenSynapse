# OpenSynapse — UI and UX Specification

**Version:** 1.0

This document is prescriptive about visual design. Claude Code should follow it closely to keep the product visually consistent. Design references: Linear, Vercel, Grafana (modern), Raycast. Things to avoid: saturated reds and blues, decorative gradients, emoji in the product UI, dated skeuomorphism, JMeter's grey toolbars.

---

## 1. Design principles

Quiet by default. The UI stays out of the user's way. Charts and data take visual priority; chrome is neutral.

Information density, not decoration. Every pixel should serve the user. No whitespace for its own sake, no icons for decoration, no tooltips that restate the obvious label.

Honest feedback. Loading states are explicit. Errors are clear and actionable. Progress bars move for a reason.

Keyboard first. Every common action has a shortcut. The product is usable without a mouse.

Consistency over novelty. Similar actions look similar everywhere. Once a user learns one view, the others feel familiar.

---

## 2. Colour system

The palette is built on slate neutrals and a single accent colour.

### 2.1 Neutrals (slate scale)

```
slate-50   #f8fafc     page background (light mode)
slate-100  #f1f5f9     subtle backgrounds, hover states
slate-200  #e2e8f0     borders, dividers
slate-300  #cbd5e1     muted borders, disabled text
slate-400  #94a3b8     placeholder text, subtle labels
slate-500  #64748b     secondary text
slate-600  #475569     secondary text (darker)
slate-700  #334155     primary text in light mode
slate-800  #1e293b     headings in light mode
slate-900  #0f172a     text (highest contrast)
slate-950  #020617     page background (dark mode)
```

### 2.2 Accent

```
teal-500   #0d9488     primary interactive (buttons, links, selected state)
teal-600   #0f766e     hover
teal-700   #115e59     active / pressed
teal-50    #f0fdfa     subtle accent backgrounds
```

Teal is chosen because it reads as technical and calm, avoids the over-used blue, and has good contrast in both light and dark themes.

### 2.3 Semantic

```
success   #10b981 (emerald-500)
warning   #f59e0b (amber-500)
error     #ef4444 (red-500)
info      #3b82f6 (blue-500)
```

Each semantic colour has a 50-level tint for backgrounds. Semantic colours appear only on semantic elements (error messages, warning banners, success confirmations). They do not appear in charts or decoration.

### 2.4 Chart palette

Charts use a separate qualitative palette chosen for accessibility (distinguishable by colour-blind users and at 200 percent zoom):

```
chart-1  #0d9488  (teal, matches accent)
chart-2  #8b5cf6  (violet)
chart-3  #f59e0b  (amber)
chart-4  #14b8a6  (lighter teal)
chart-5  #ec4899  (pink)
chart-6  #6366f1  (indigo)
chart-7  #84cc16  (lime)
chart-8  #06b6d4  (cyan)
```

For overlays where semantic meaning matters (for example, a comparison where run A vs run B is compared to a baseline), use teal for the baseline and violet for the comparator. Red is reserved for error states only.

### 2.5 Dark mode

Every colour above has a dark mode pair. The slate scale inverts (slate-950 becomes background, slate-100 becomes primary text). Teal shifts slightly brighter (teal-400 for interactive, teal-300 for hover). Semantic colours shift slightly desaturated to avoid glare.

---

## 3. Typography

One typeface for UI, one for code.

```
UI:    Inter       (system fallback: -apple-system, system-ui)
Code:  JetBrains Mono (fallback: Menlo, Consolas, monospace)
```

Scale:

```
xs    11px   small labels, metadata
sm    13px   secondary text, metrics on cards
base  14px   default body
lg    16px   emphasised body
xl    20px   section headings
2xl   24px   page headings
3xl   30px   hero numbers (e.g., large metric displays)
```

Line height: 1.5 for body, 1.2 for headings. Font weights: 400 regular, 500 medium for labels and nav, 600 semibold for headings, 700 used sparingly for hero numbers.

Inter is bundled; no webfont loads from CDN. JetBrains Mono likewise bundled.

---

## 4. Spacing and layout

The layout grid uses 4px increments.

```
space-0   0px
space-1   4px
space-2   8px
space-3   12px
space-4   16px   <- default component padding
space-5   20px
space-6   24px
space-8   32px
space-10  40px
space-12  48px
space-16  64px
```

Standard layout widths. The app fills the viewport. The main content area has a max content width of 1440px on very wide screens, centred, with the rest as neutral background.

Page layout: a 56px top bar, a 220px left sidebar (collapsible to 56px), the main content area. The top bar carries: logo, global search, environment selector, run button shortcut, profile menu. The sidebar carries: primary navigation (Home, Plans, Runs, Playground, Crawler, Library, Settings).

---

## 5. Component style

### 5.1 Buttons

Primary button. Teal-500 background, white text, 14px font, 500 weight, 8px vertical padding, 16px horizontal, 6px border radius, no shadow. Hover raises to teal-600. Active drops to teal-700. Disabled is slate-200 background, slate-400 text.

Secondary button. Transparent background, slate-300 border, slate-700 text. Hover adds slate-100 background. Used for secondary actions next to a primary.

Ghost button. No border, no background, slate-600 text. Hover adds slate-100 background. Used for tertiary actions in toolbars.

Destructive button. Red-500 background, white text, used only for delete and stop-run actions. Confirmation dialog required before any destructive action.

Icon button. 32px square, rounded, ghost style by default. Used in toolbars and inline edit actions. Accessible name required.

### 5.2 Inputs

Text input. White background, slate-300 border, slate-900 text, 6px border radius, 8px vertical padding, 12px horizontal. Focus ring is 2px teal-500 with 2px offset (no glow). Invalid state: red-500 border, red-500 helper text below.

Select. Built on a headless primitive. Visually matches text input. Dropdown panel uses a light shadow (not the neon glow that some modern UIs use).

Checkbox and radio. 16px square (checkbox) or circle (radio), slate-300 border, teal-500 checked fill with white glyph. 

Switch. Used for settings that toggle a mode. 32px x 18px. Off is slate-200, on is teal-500.

### 5.3 Cards

Default card. White background, 1px slate-200 border, 8px border radius, 16px padding, no shadow. Cards used as primary content containers.

Interactive card. Default card plus cursor-pointer and a subtle hover (slate-50 background, border shifts to slate-300). Used for template gallery, fragment library, run list items.

Metric card. Compact card with a label (xs, slate-500) and a large number (3xl, slate-900). Used on the run dashboard for summary stats.

### 5.4 Tables

Dense rows (32px tall). Header row has slate-50 background, sm text, 500 weight, slate-600. Body rows have 14px text, slate-800. Dividers between rows are 1px slate-100 (not 200; we want them subtle). Hover row background is slate-50. Sort indicators are chevron icons on the active sort column.

### 5.5 Dialogs and modals

Centred, max-width 560px for standard, 960px for large. Backdrop is slate-900 at 40 percent opacity. The dialog itself has a white background, 12px border radius, a light shadow (no neon glow). 24px padding. Close button in the top-right. Focus trapped. Escape dismisses.

### 5.6 Toasts

Top-right, stacked. 6 second auto-dismiss, or permanent for errors. Success is emerald, warning is amber, error is red, info is slate. Icon on the left, message in the middle, close button on the right. Slide in from the top.

### 5.7 Code blocks and editors

Monaco editor for any code view. Theme configured to match the slate palette; not the default VS Code theme. Line numbers in slate-400. Selection in teal-100. JavaScript syntax for generated k6 scripts, JSON for plan files and API responses.

---

## 6. Motion

Motion is functional, not decorative. Transitions are short: 150ms for small state changes (button hover, dropdown open), 250ms for layout changes (sidebar collapse, panel open), 400ms max for anything. Ease: `cubic-bezier(0.2, 0, 0, 1)` (Apple's default). No bouncing, no springs, no parallax. Respects `prefers-reduced-motion`.

The template gallery SVG animations are the exception: they loop to illustrate load curves. They pause on hover if the user prefers reduced motion.

---

## 7. Charts

Built with Recharts. Default size fills the container. Axes are slate-400 with 11px labels. Grid lines are slate-100, horizontal only by default. Series lines are 2px stroke, no fill for line charts, 0.1 alpha fill for area charts. Points hidden by default, shown on hover.

Tooltips on hover. Slate-900 background, white text, small shadow, no arrow. Multi-series tooltips show one row per series with colour dot, label, and value.

Legends on the right for space-constrained charts, bottom for full-width charts. Clicking a legend item toggles the series.

Empty states for charts show a short message and an icon, nothing more.

---

## 8. Key screens

### 8.1 Home

Top strip: recent runs (3 cards). Left: quick actions (New test, Crawl application, Open playground). Right: environment health summary if any targets are monitored. Bottom: documentation links (local markdown, not external).

### 8.2 Plans list

Search bar top. Table below. Columns: name, tags, last run, last result, actions. Click a row to open the builder.

### 8.3 Plan builder

Three panes. Left: node tree (30 percent). Centre: flow canvas for the selected branch (40 percent). Right: properties form (30 percent). Panes are resizable. The top of the builder has a toolbar: back to plans, plan name (editable inline), save status ("saved", "saving..."), code view toggle, run button.

### 8.4 Run view (live and historical)

Top strip: summary metrics (6 metric cards). Main area: default charts in a grid (3 columns on wide screens, 2 on medium, 1 on narrow). Right sidebar (live only): live control panel with sliders and pause button. Bottom: error log, expandable.

### 8.5 Comparison view

Top: selected runs as chips, remove button on each. Below: summary block with improvement/degradation statements. Main area: one chart per selected metric, with overlaid lines for each run. Right sidebar: AI analysis panel if enabled.

### 8.6 Endpoint playground

Postman-like. Left: request history. Centre: request builder and response pane, split horizontally. Right: collections.

### 8.7 Crawler

Left: crawl configuration form. Centre: progress and graph visualisation as the crawl runs. Right: captured requests list. Bottom strip: actions (cancel, generate plan).

---

## 9. Accessibility

WCAG 2.1 AA minimum. All interactive elements have visible focus indicators (2px teal outline with 2px offset). Colour contrast ratios: 4.5:1 for body text, 3:1 for large text and UI elements. No information conveyed by colour alone (status uses icon plus text plus colour). All charts have a "show as table" toggle. All forms have proper labels, error messages are associated via aria-describedby.

Screen reader testing required against NVDA on Windows and VoiceOver on Mac before each release.

---

## 10. Design token file

All tokens live in a single TypeScript file (`packages/ui/src/tokens.ts`) and are exported to Tailwind via its config. Tailwind's default palette is replaced wholesale with OpenSynapse's palette to prevent developers accidentally using colours outside the system. A CI check fails the build if hex colours appear in component source outside the tokens file.
