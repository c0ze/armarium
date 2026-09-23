<script lang="ts">
  // One stroke weight, 24px grid, drawn for Armarium.
  const paths: Record<string, string> = {
    wall: 'M3 5h18M3 12h18M3 19h18M7 5v14M13 5v14',
    search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM20 20l-4.2-4.2',
    comic: 'M4 4h16v16H4zM4 11h9M13 4v16M13 14h7',
    book: 'M5 4h11a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3zM5 17a3 3 0 0 1 3-3h11',
    admin: 'M4 7h10M18 7h2M4 17h2M10 17h10M14 5v4M6 15v4',
    logout: 'M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10',
    left: 'M15 5l-7 7 7 7',
    right: 'M9 5l7 7-7 7',
    play: 'M8 5v14l11-7z',
    check: 'M5 12.5l4.5 4.5L19 7',
    clock: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 7v5l3 2',
    circle: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z',
    download: 'M12 4v11M7 10l5 5 5-5M5 20h14',
    back: 'M19 12H5M11 6l-6 6 6 6',
    sun: 'M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5L19 19M5 19l1.5-1.5M17.5 6.5L19 5',
    moon: 'M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5z',
    list: 'M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01',
  };
  let { name, size = 18 }: { name: string; size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
  stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d={paths[name] ?? ''} fill={name === 'play' ? 'currentColor' : 'none'} />
</svg>
