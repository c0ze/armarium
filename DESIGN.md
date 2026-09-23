---
name: Armarium
description: A self-hosted comics and ebook library shown as a catalog wall of real covers.
colors:
  amber: "#ffc857"
  amber-ink: "#1a1405"
  ground: "#0b0e13"
  surface: "#151922"
  raised: "#1f2430"
  line: "#262c38"
  outline: "#3a4150"
  text: "#e6e6e8"
  text-2: "#a4a9b3"
  text-3: "#858b96"
  danger: "#ff7a6b"
  reader-paper: "#fbf8f1"
  reader-ink: "#1f1d1a"
  reader-link-paper: "#7a4b12"
typography:
  display:
    fontFamily: "'Bebas Neue', 'Oswald', 'Arial Narrow', sans-serif"
    fontSize: "clamp(3.4rem, 6.2vw, 6rem)"
    fontWeight: 400
    lineHeight: 0.95
    letterSpacing: "0.02em"
  headline:
    fontFamily: "'Bebas Neue', 'Oswald', 'Arial Narrow', sans-serif"
    fontSize: "clamp(3rem, 5vw, 4.6rem)"
    fontWeight: 400
    lineHeight: 0.95
    letterSpacing: "0.02em"
  title:
    fontFamily: "'Bebas Neue', 'Oswald', 'Arial Narrow', sans-serif"
    fontSize: "2rem"
    fontWeight: 400
    lineHeight: 0.95
    letterSpacing: "0.02em"
  body:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Noto Sans', 'Noto Sans JP', sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.5
    fontFeature: "tnum"
  meta:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Noto Sans', 'Noto Sans JP', sans-serif"
    fontSize: "12px"
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "0.04em"
  label:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Noto Sans', 'Noto Sans JP', sans-serif"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: 1
    letterSpacing: "0.14em"
  caption:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Noto Sans', 'Noto Sans JP', sans-serif"
    fontSize: "11px"
    fontWeight: 500
    lineHeight: 1.2
    letterSpacing: "0.16em"
  reader-body:
    fontFamily: "Georgia, 'Iowan Old Style', 'Noto Serif', serif"
    fontSize: "1.1rem"
    fontWeight: 400
    lineHeight: 1.6
rounded:
  sm: "3px"
  md: "4px"
  full: "50%"
spacing:
  gutter: "clamp(1.2rem, 3vw, 3rem)"
  tile-gap: "0.7rem"
  grid-row: "1.6rem"
  grid-col: "1.1rem"
  stack: "1.1rem"
  rail-gap: "2.6rem"
  page-top: "2.8rem"
  rail-w: "15rem"
  tile-w: "9.5rem"
  tile-open: "26rem"
components:
  button-outline:
    backgroundColor: "transparent"
    textColor: "{colors.text}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0.85rem 1.2rem"
  button-reading:
    backgroundColor: "transparent"
    textColor: "{colors.amber}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0.85rem 1.2rem"
  button-reading-hover:
    backgroundColor: "{colors.amber}"
    textColor: "{colors.amber-ink}"
  button-quiet:
    textColor: "{colors.text-2}"
    typography: "{typography.label}"
    padding: "0.85rem 0.6rem"
  button-round:
    textColor: "{colors.text}"
    rounded: "{rounded.full}"
    size: "2.2rem"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text}"
    rounded: "{rounded.sm}"
    padding: "0.7rem 0.85rem"
  select:
    backgroundColor: "{colors.ground}"
    textColor: "{colors.text-2}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0.72rem 2.4rem 0.72rem 1rem"
  nav-item:
    textColor: "{colors.text-2}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0.5rem 0.6rem"
  nav-item-current:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.amber}"
  tile:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
    width: "{spacing.tile-w}"
  tile-open:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
    width: "{spacing.tile-open}"
    padding: "1.1rem 1.1rem 1rem"
  card:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
  title-plate:
    backgroundColor: "{colors.raised}"
    textColor: "{colors.text}"
    padding: "0.9rem 0.8rem"
  tag:
    textColor: "{colors.text-2}"
    rounded: "{rounded.sm}"
    padding: "0.3rem 0.65rem"
---

# Design System: Armarium

## Overview

**Creative North Star: "The Catalog Wall"**

Armarium is a dark wall of real covers. The ground is near-black and cool, the chrome is two lifted grey-blue surfaces and hairline rules, and the covers supply every colour on screen. Hover or focus a cover and it widens in place into a card that says what it is; the neighbours slide aside. Headings are tall condensed capitals (Bebas Neue); everything that is data (authors, counts, formats, chapter positions) is set in the system UI face with tabular figures.

One warm colour exists: amber. It marks where you are and what reads: the focused edge, the current nav item, reading progress, and the Resume / Open / Read action. Everything else is a neutral outline. The same dark ground carries into the EPUB reader and comic viewer, with a paper mode for books.

The world rejects the admin-table-with-thumbnails and the pastel card grid. Density is medium: rails of 9.5rem covers, generous row gaps, no boxes around sections, just hairline rules and whitespace.

**Key Characteristics:**
- Covers are the only colour; chrome stays within the ground/surface/raised/text greys.
- Amber is reserved and appears at rest only on current location, reading progress and reading actions.
- Condensed caps for every heading, system UI for every datum.
- Flat chrome; soft shadows belong only to covers lifted off the wall.
- Gradients exist only as scrims over cover imagery.

## Colors

A cool near-black monochrome with a single amber signal; the cover art is the palette.

### Primary
- **Reading Amber** (amber): the 1px focus outline, the widened tile's edge, the current nav item, progress bars and the in-tile "Reading · 32%" status, the Resume / Open / Read / Continue buttons, the text caret and selection. Hover on small text links (View all, tags, series link, reader back arrow) also tints amber, as the pointer's stand-in for focus.
- **Amber Ink** (amber-ink): text on a filled amber surface only (reading-button hover, text selection).

### Neutral
- **Night Ground** (ground): page background, sidebar, reader and viewer chrome, and the dark reader page.
- **Shelf Surface** (surface): tiles and cards under their covers, inputs, hovered and current nav rows, the reader contents drawer.
- **Raised Slate** (raised): progress tracks, the title plate behind coverless items, the selected segment in segmented controls, scrollbar thumbs, missing-image fill.
- **Hairline** (line): every divider: sidebar edge, hero base, page-head rules, admin rows, segmented-control and tag borders, round arrow buttons.
- **Outline Grey** (outline): the resting border of neutral buttons.
- **Bone** (text): titles and primary copy.
- **Ash** (text-2): bylines, nav items at rest, secondary copy.
- **Smoke** (text-3): meta lines, counts, placeholders, group labels.
- **Ember Red** (danger): error text and an armed destructive confirmation (token revoke). Never a fill.
- **Paper / Ink / Paper Link** (reader-paper, reader-ink, reader-link-paper): the light EPUB page. In dark mode the reader uses ground, text and amber instead.

### Named Rules
**The Covers-Are-The-Colour Rule.** No hue enters the chrome. If a surface needs colour, it gets it from a cover image, blurred or whole.

**The Amber Reservation Rule.** Amber at rest means "you are here" or "this reads": current nav, progress, reading action. Every other button is a neutral outline, including Details, Mark read, Download, Scan and Create token. A second amber button on one screen is a bug.

## Typography

**Display Font:** Bebas Neue (with Oswald, Arial Narrow), self-hosted via @fontsource/bebas-neue
**Body Font:** system-ui stack (with Segoe UI, Roboto, Noto Sans, Noto Sans JP)
**Reader Font:** Georgia (with Iowan Old Style, Noto Serif), EPUB body only

**Character:** Poster-tall capitals over quiet native text. The display face names things; the system face counts them.

### Hierarchy
- **Display** (400, clamp 3.4 to 6rem, 0.95): the wall hero title, max 14ch. Login wordmark scales this up.
- **Headline** (400, clamp 3 to 4.6rem, 0.95): page titles on library, series and admin; the item page runs slightly larger (to 5.2rem, max 18ch).
- **Title** (400, 2rem, 0.95): rail and admin section headings (1.7rem under 700px); open-tile titles at 1.9rem, clamped to three lines; reader and viewer bar titles at 1.5rem.
- **Body** (400, 15px, 1.5, tabular figures): all running copy and data.
- **Meta** (400, 12 to 13px, 0.04em): format, page counts, series position, in Smoke.
- **Label** (500, 12px, 0.14em, uppercase): buttons, selects, sidebar nav. Library names drop to sentence case 13.5px.
- **Caption** (500, 11px, 0.16em, uppercase, Ash): status and count lines ("Chapter 20 of 62", "1,128 items"), sidebar group labels, form labels.
- **Reader body** (1.1rem, 1.6, 34em measure): EPUB text; the reader's size and measure override the book's.

### Named Rules
**The Caps-Name, System-Count Rule.** Every h1 to h3 is Bebas Neue caps with balanced wrapping; no heading in the system face, no data in Bebas.

**The Caption-Follows Rule.** Caption caps sit after or beside the thing they describe (under a title, beside a progress bar, as a group label in the nav). They never stand above a heading as a label for it.

## Layout

A fixed left rail (15rem) beside a fluid main column with a side gutter of clamp(1.2rem, 3vw, 3rem). The wall stacks full-width rows 2.6rem apart: a full-bleed resume band (it cancels the gutter and restores it inside), then Continue reading, Recently added, then one rail per library.

- **Rails** scroll horizontally with snap, hidden scrollbars and round arrow buttons at the header's right; tiles 9.5rem wide at 2:3, 0.7rem apart. Rails lazy-load 600px before entering view and draw surface-coloured ghosts while loading.
- **Grids** (library, series) use auto-fill columns of min 9.5rem, 1.6rem row and 1.1rem column gaps.
- **Page heads** open 2.8rem from the top and close with a hairline before the content.
- **Item page** is two columns: cover clamp(12rem, 24vw, 20rem), body flexible, gap up to 4rem.
- **Responsive:** under 900px the sidebar becomes a fixed bottom bar (search plus icon-only nav, safe-area padded). Under 760px the hero drops its cover. Under 700px rail arrows go (also on touch devices) and the item page stacks. Under 600px grids tighten to 7rem minimum.

**The Full-Bleed-Once Rule.** Only the resume band breaks the gutter. Everything else sits in the column.

## Elevation & Depth

Chrome is flat and tonal: ground, surface and raised greys, separated by hairlines. Shadows are soft, black and ambient, and only covers get them: the large art on hero and item pages, and a tile or card lifted by hover or focus. The reader's contents drawer carries one side shadow. The wall also has depth by dimming: while one rail is hovered, the others fade to 40% opacity and half saturation.

### Shadow Vocabulary
- **Cover on display** (`0 24px 60px rgba(0,0,0,.6), 0 2px 6px rgba(0,0,0,.5)`): hero and item-page covers.
- **Tile open** (`0 12px 32px rgba(0,0,0,.55)`): widened wall tile.
- **Card lift** (`0 14px 30px rgba(0,0,0,.5)`): grid card on hover or focus, with a 3px rise.
- **Drawer** (`12px 0 40px rgba(0,0,0,.45)`): reader contents panel.

### Named Rules
**The Scrim-Only Gradient Rule.** Gradients exist only to lay the ground over cover imagery (the hero's left-to-right and bottom scrims over its blurred backdrop). No gradient fills on chrome.

**The Known Limit.** The hero backdrop is the lead's own cover, blurred (14px), brightened and at 60% opacity. On mostly-black covers it reads as a dim streak rather than a colour field. This was accepted at review; don't boost opacity or add a synthetic tint to compensate.

## Shapes

Nearly square. Controls, inputs, tags and nav rows use a 3px corner; covers, tiles, cards and ghosts a 4px corner. Circles appear in three places only: rail arrow buttons, the white "read" dot on a cover's corner, and the admin scan pulse. Borders are 1px hairlines; progress is a 3px bar, flush along a cover's bottom edge or free-standing on a raised track. Icons are hand-drawn SVG line glyphs on a 24px grid at 1.5 stroke, in currentColor.

## Components

### Buttons
- **Shape:** squared outline (3px), label type in caps, icon then text at 0.6rem gap.
- **Neutral outline (default):** transparent, Bone text, Outline Grey border; hover lifts the border to Ash.
- **Reading action:** amber border and text; hover fills amber with Amber Ink text. Only Resume, Open, Read, Continue/Start and Open PDF / Open in PanelFlow get it.
- **Quiet:** borderless Ash text, tighter padding; reader toolbar and per-row admin actions.
- **Round:** 2.2rem circle with a hairline border for rail scrolling.
- **Focus:** border and text go amber, no extra ring. Disabled drops to 45% opacity.

### Segmented control
Hairline-bordered 3px group of borderless buttons divided by hairlines; the selected segment gets Raised Slate and Bone text (not amber, since a filter is not a location).

### Inputs / Fields
- **Style:** Shelf Surface fill, hairline border, 3px corner, 14px text, Smoke placeholder. Selects use the label type in caps on the ground with a drawn chevron.
- **Focus:** border goes amber, no outline. The sidebar search tints its icon amber too.

### Wall tile (signature)
A 9.5rem, 2:3 cover on a surface-coloured tile. On hover or keyboard focus (hover-capable devices only) it widens to 26rem over 0.5s, gains the amber hairline edge and the open shadow, and after 0.15s fades in title, byline, meta and an amber status line pinned to the bottom. Reduced motion removes the transitions.

### Grid card
In grids the layout must not reflow, so the card lifts 3px instead of widening, with the amber edge; its title turns amber on hover. Title 13.5px medium, two lines; subline in Smoke.

### Title plate
Stands in for any missing cover at the same proportions: Raised Slate with a 3px top rule, title in display caps up to five lines, byline, and format in caption type at the foot. The wall never shows a hole.

### Resume band
Full-bleed hero: blurred own-cover backdrop under ground scrims, display title, byline, amber progress with a caption position, the amber Resume beside an outline Details, and the sharp cover on the right. With nothing in progress it shows the newest arrival and "Open".

### Navigation
Sidebar: wordmark in display caps (2.3rem, 0.08em), bordered search, then label-type rows with icons. Rest Ash; hover Bone on Shelf Surface; current amber on Shelf Surface. Status rows carry right-aligned Smoke counts. On mobile it becomes the bottom bar described in Layout.

### Tags
Small 3px outline chips (12px, Ash); hover turns border and text amber.
### Readers
Book reader and comic viewer are full-screen with ground-coloured bars and hairline edges, display-caps titles and quiet toolbar buttons. The comic stage sits on a darker black; the page slider uses amber accent colour.

## Do's and Don'ts

### Do:
- **Do** let covers carry all colour; add chrome only in the ground, surface, raised and text greys.
- **Do** mark focus with a 1px amber outline at 2px offset; tiles and cards use their amber edge instead.
- **Do** give coverless items a title plate, never an empty box or broken image.
- **Do** keep reading actions amber and every other action a neutral outline.
- **Do** style through classes and Svelte `style:` directives (e.g. progress widths). The CSP forbids inline `style` attributes.
- **Do** self-host fonts (Bebas Neue via @fontsource); nothing loads from a font CDN.

### Don't:
- **Don't** make a second amber button on a screen, or colour Details, Mark read, Download or admin actions amber.
- **Don't** put gradients on chrome; the only gradients are scrims over cover art.
- **Don't** set headings in the system face or data in Bebas Neue.
- **Don't** set a caps caption above a heading as its label.
- **Don't** add shadows to flat chrome (sidebar, headers, rows, inputs); shadows belong to lifted covers and the reader drawer.
- **Don't** use hard offset shadows or tinted shadows; depth is soft black only.
- **Don't** let a book's stylesheet win on measure, margins or body size in the reader.
