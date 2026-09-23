// Pure helpers for the readers, kept out of components so they can be tested.

export interface Locator {
  chapter: number;
  scroll: number;
}

export function parseLocator(raw: string): Locator | null {
  try {
    const v = JSON.parse(raw) as Partial<Locator>;
    if (Number.isInteger(v.chapter) && (v.chapter as number) >= 0) {
      return { chapter: v.chapter as number, scroll: Math.min(Math.max(Number(v.scroll) || 0, 0), 1) };
    }
  } catch {
    /* empty or foreign locator */
  }
  return null;
}

/** The chapter an iframe landed on after following an in-book link. */
export function chapterFromPath(pathname: string, id: number): number | null {
  const m = pathname.match(/^\/read\/(\d+)\/chapter\/(\d+)$/);
  return m && Number(m[1]) === id ? Number(m[2]) : null;
}

/**
 * Progress for an EPUB position. Page counts chapters (1-based); the book only
 * becomes "read" once the last chapter is scrolled to the end.
 */
export function progressFor(chapter: number, chapters: number, fraction: number) {
  const last = chapter >= chapters - 1;
  const page = last ? (fraction > 0.95 ? chapters : Math.max(chapters - 1, 0)) : chapter + 1;
  return {
    page,
    status: page >= chapters ? ('read' as const) : ('reading' as const),
    locator: JSON.stringify({ chapter, scroll: Math.round(fraction * 1000) / 1000 }),
  };
}
