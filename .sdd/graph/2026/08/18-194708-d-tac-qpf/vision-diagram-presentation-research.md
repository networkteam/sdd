# Vision diagram presentation research

## Observation

The four Vision diagrams are inline SVGs with 920-unit-wide view boxes. Each is wrapped in a locally styled `figure.card` with 18px horizontal padding. Nova sets `--sl-content-width: 50rem`, leaving the diagram around 764px wide in the normal content column. The smallest 9.5px SVG labels therefore render at roughly 8px before other viewport constraints.

The diagrams themselves are semantically structured and accessible through `<title>` and `<desc>`; the problem is their presentation scale.

## Layout options

### Widen the whole Starlight content column

Starlight supports overriding `--sl-content-width` through custom CSS. This would also widen prose on every affected page and cannot safely consume the space reserved for Nova's navigation and page table of contents. Rejected as too broad.

References:

- https://starlight.astro.build/guides/css-and-tailwind/
- https://starlight-theme-nova.pages.dev/guide/css-and-styling/

### Diagram-only breakout

Keep prose at Nova's 50rem width. Center each diagram beyond the inner `.sl-container`, calculate the safe width from the viewport and active Starlight sidebars, and cap it at a deliberate maximum. Nova's `.main-pane` has `overflow-x: hidden`, so the breakout must stay within the main pane; it must not overlap navigation or the table of contents. Narrow viewports retain horizontal scrolling.

This uses available whitespace on wider displays without sacrificing the page TOC or prose readability.

## Lightbox options

### starlight-image-zoom

Current version inspected: 0.15.0.

- Peer dependency: `@astrojs/starlight >=0.41.0`
- Node requirement: `>=22.12.0`
- The project uses Starlight 0.41.3 and Node 24.
- Uses a native `<dialog>`, portals the dialog to the document body, restores focus, supports Escape/backdrop closure, and respects reduced-motion preferences.
- Has no third-party client-side runtime dependency.
- Inline SVG is supported through the explicit `<Zoom>` component.
- Automatic image handling does not reach the current SVGs because they are nested inside Astro components and figure/scroll wrappers. `<Zoom>` must directly wrap each SVG.

References:

- https://starlight-image-zoom.vercel.app/getting-started/
- https://starlight-image-zoom.vercel.app/components/zoom/
- https://github.com/HiDeoo/starlight-image-zoom/blob/main/packages/starlight-image-zoom/components/ImageZoom.astro
- https://github.com/HiDeoo/starlight-image-zoom/blob/main/packages/starlight-image-zoom/package.json

### Local custom lightbox

The researched sample relies on `document.currentScript` inside an Astro script, injects elements that may not receive scoped-style attributes, keeps the overlay inside an isolated/clipped page subtree, and does not fully handle focus restoration or modal semantics. Owning a correct dialog would add ongoing accessibility, navigation, animation, scroll-lock, and browser-behavior maintenance.

Rejected in favor of the focused community plugin.

## Implementation shape

Introduce one local `DiagramFrame.astro` to own:

- borderless figure presentation;
- responsive breakout constrained to the safe main pane and a tunable maximum width;
- narrow-screen horizontal scrolling;
- caption styling;
- the explicit `<Zoom>` wrapper and accessible label.

Migrate the four Vision SVGs to that frame. Keep the page table of contents and normal prose width unchanged. The inline view uses the available layout width; the lightbox supplies the full-viewport focused view.
