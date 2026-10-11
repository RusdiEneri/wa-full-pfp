---
version: "alpha"
name: "Neubrutalism"
description: "Neubrutalist interface. Ideal for landing pages, saas. AI-ready template."
colors:
  primary: "#FFEB3B"
  secondary: "#FF5252"
  tertiary: "#2196F3"
  neutral: "#000000"
typography:
  h1:
    fontFamily: System UI stack
    fontSize: 2.25rem
    fontWeight: 700
  body-md:
    fontFamily: System UI stack
    fontSize: 1rem
    fontWeight: 400
  label-caps:
    fontFamily: System UI stack
    fontSize: 0.75rem
    fontWeight: 500
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.neutral}"
    padding: 12px
---

## Overview

Neubrutalist interface. Ideal for landing pages, saas. AI-ready template. Brutalism on the web was always a provocation. Raw HTML, system fonts, zero polish — it said "I don't care about your expectations." Neubrutalism kept the attitude but ditched the hostility. It's brutalism that actually wants you to click the button.

The turning point was Gumroad's 2021 redesign. Sahil Lavingia stripped the product down to thick black borders, flat primary colors, and offset drop shadows that looked like someone dragged a rectangle two pixels southeast. It was loud, playful, and unmistakably intentional. Designers noticed. Within months, the Figma community had dozens of neubrutalist UI kits climbing the charts.

What separates it from classic brutalism is warmth. Where brutalism embraced ugliness as ideology, neubrutalism uses bold geometry as decoration. The borders are thick but the corners can be rounded. The shadows are hard but the palette is candy-colored. It's confrontational aesthetics made approachable — punk rock in a good mood.

- Density: 5/10 — Balanced
- Variance: 4/10 — Moderate
- Motion: 4/10 — Subtle

- **Style:** Bold, Colorful, Raw, Playful
- **Keywords:** Bold borders, black outlines, primary colors, thick shadows, no gradients, flat colors, 45° shadows, playful, Gen Z
- **Era:** 2020s Modern
- **Light/Dark:** ✓ Full / ✓ Full

## Colors

- **#FFEB3B** (#FFEB3B) — Primary surface or dominant color
- **#FF5252** (#FF5252) — Secondary surface or text color
- **#2196F3** (#2196F3) — Supporting palette color
- **#000000** (#000000) — Supporting palette color


## Typography

- **Display / Hero:** System UI stack (-apple-system, sans-serif) — Weight 700, tight tracking, used for headline impact
- **Body:** System UI stack (-apple-system, sans-serif) — Weight 400, 16px/1.6 line-height, max 72ch per line
- **UI Labels / Captions:** System UI stack (-apple-system, sans-serif) — 0.875rem, weight 500, slight letter-spacing
- **Monospace:** JetBrains Mono — Used for code, metadata, and technical values

Scale:
- Hero: clamp(2.5rem, 5vw, 4rem)
- H1: 2.25rem
- H2: 1.5rem
- Body: 1rem / 1.6
- Small: 0.875rem


## Layout

- **Grid:** CSS Grid primary. Max-width containment: 1280px centered with 1.5rem side padding.
- **Spacing rhythm:** Balanced. Base unit: 0.5rem (8px).
- **Section vertical gaps:** clamp(4rem, 8vw, 8rem).
- **Hero layout:** Split-screen (text left, visual right).
- **Feature sections:** Zig-zag alternating text+image rows. No 3-equal-columns.
- **Mobile collapse:** All multi-column layouts collapse below 768px. No horizontal overflow.
- **z-index contract:** base (0) / sticky-nav (100) / overlay (200) / modal (300) / toast (500).


## Elevation & Depth

box-shadow: 4px 4px 0 #000, border: 3px solid #000, no gradients, sharp corners (0px), bold typography

- **Physics:** Ease-out curves, 200-300ms duration. Smooth and predictable.
- **Entry animations:** Fade + translate-Y (16px → 0) over 420ms ease-out. Staggered cascades for lists: 80ms between items.
- **Hover states:** Subtle color shift + shadow adjustment over 200ms.
- **Page transitions:** Fade only (200ms).
- **Performance:** Only transform and opacity animated. No layout-triggering properties.


## Shapes

Base corner radius: 8px. See rounded tokens in front matter for the full scale.


## Components

- **Primary Button:** Subtly rounded (0.5rem) shape. Accent color fill. Hover: 8% darken + subtle lift shadow. Active: -1px translate tactile press. Font weight 600. No outer glows.
- **Secondary / Ghost Button:** Outline variant. 1.5px border in muted color. Text in primary color. Hover: subtle background fill.
- **Cards:** Subtly rounded (0.5rem) corners. Surface background. Subtle shadow (0 2px 12px rgba(0,0,0,0.06)). 1px border stroke.
- **Inputs:** Label above input. 1px border stroke. Focus ring: 2px accent color offset 2px. Error text below in semantic red. No floating labels.
- **Navigation:** Primary surface background. Active item: accent color indicator. Font weight 500 when active.
- **Skeletons:** Shimmer animation matching component dimensions. No circular spinners.
- **Empty States:** Icon-based composition with descriptive text and action button.


## Do's and Don'ts

- No emojis in UI — use icon system only (Lucide, Heroicons)
- No pure black (#000000) — use off-black or charcoal variants
- No oversaturated accent colors (saturation cap: 80%)
- No 3-column equal-width feature layouts — use zig-zag or asymmetric grid
- No `h-screen` — use `min-h-[100dvh]`
- No AI copywriting clichés: "Elevate", "Seamless", "Unleash", "Next-Gen"
- No broken external image links — use picsum.photos or inline SVG
- No generic lorem ipsum in demos

- Do Hard borders (2-4px)
- Do Hard offset shadows
- Do High saturation colors
- Do Bold typography
- Do No blurs/gradients
- Do Distinctive 'ugly-cute' look


## Use Case

Landing pages, SaaS
