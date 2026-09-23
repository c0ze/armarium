# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

The owner of a home server, at a desktop browser, managing their own collection of
comics and ebooks: checking what is there, scanning new files in, finding a title,
marking progress, issuing API tokens for reading apps. Open-source self-hosters are a
secondary audience who meet the same UI on their own servers.

## Product Purpose

Armarium serves folders of comics and Calibre libraries to reading apps (OPDS,
PanelFlow, KOReader) and gives the owner one place to see and manage the whole
collection. Success: the owner can see the state of tens of thousands of items at a
glance, find anything in seconds, and trust what they see (what is new, missing,
unreadable, in progress).

## Positioning

A small, single-binary server that reads Calibre's own metadata and plain comic
folders side by side, with no metadata management of its own. It stands in for
Kavita and Calibre-Web without their weight.

## Operating Context

Desktop browser on the home network (sometimes over Tailscale). Libraries: one local
comics folder of ~30 series (mostly CBR), comics from a download client over NFS, and
21 Calibre libraries (~13,000 books, many MOBI-only). Reading mostly happens in other
apps; the web UI's EPUB reader and comic viewer are fallbacks.

## Capabilities and Constraints

- Views: login, library (per-library tabs, search, filters by status/source/tag,
  sort, cover grid), series, item detail (formats, progress, mark read/unread,
  open in PanelFlow), EPUB reader, comic quick viewer, admin (scan, tokens).
- Covers are lazily generated thumbnails; many items have none (PDFs, MOBI without
  a Calibre cover) and need a designed fallback.
- Strict CSP: no inline styles or scripts, fonts must be self-hosted or system.
- Svelte 5 SPA embedded in a Go binary; keep the bundle small.

## Brand Commitments

Name: Armarium (a medieval monastery's book cupboard). The previous look borrowed
PanelFlow's pastel gradients, pill tabs and soft cards; the owner rejected it as
generic. No visual direction is pinned yet.

## Product Principles

- The collection is the interface: covers and titles carry the page, chrome recedes.
- State must be legible: new, missing, unreadable, reading, read.
- Fast at scale: tens of thousands of items without the page feeling heavy.
- Nothing invented: show what the library actually holds.
