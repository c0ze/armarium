// A tiny history-API router. The server answers every non-API path with the
// app shell, so deep links and reloads work.

export interface Route {
  name: 'library' | 'series' | 'item' | 'reader' | 'viewer' | 'admin' | 'login' | 'notfound';
  id: number;
  params: URLSearchParams;
}

function parse(): Route {
  const params = new URLSearchParams(location.search);
  const parts = location.pathname.split('/').filter(Boolean);
  const id = Number(parts[1] ?? 0);
  const valid = Number.isSafeInteger(id) && id > 0;
  switch (parts[0] ?? '') {
    case '':
      return { name: 'library', id: 0, params };
    case 'series':
    case 'item':
    case 'reader':
    case 'viewer':
      return valid ? { name: parts[0] as Route['name'], id, params } : { name: 'notfound', id: 0, params };
    case 'admin':
    case 'login':
      return { name: parts[0] as Route['name'], id: 0, params };
  }
  return { name: 'notfound', id: 0, params };
}

export const router = $state({ route: parse() });

export function navigate(href: string, replace = false) {
  if (replace) history.replaceState(null, '', href);
  else history.pushState(null, '', href);
  router.route = parse();
  if (!replace) window.scrollTo(0, 0);
}

addEventListener('popstate', () => (router.route = parse()));

/** Intercepts same-origin <a> clicks so links don't reload the page. */
export function link(node: HTMLAnchorElement) {
  const onClick = (e: MouseEvent) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const url = new URL(node.href);
    if (url.origin !== location.origin || node.target) return;
    e.preventDefault();
    navigate(url.pathname + url.search);
  };
  node.addEventListener('click', onClick);
  return { destroy: () => node.removeEventListener('click', onClick) };
}
