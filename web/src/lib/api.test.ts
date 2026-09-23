import { describe, expect, it } from 'vitest';
import { fileUrl, formatSize, percent, query, readable, seriesOf, type Item } from './api';

const item = (page: number, pages: number, status: Item['progress']['status'] = 'reading') =>
  ({ pages, progress: { page, status, locator: '', updatedAt: 0 } }) as Item;

describe('api helpers', () => {
  it('builds query strings without empty values', () => {
    expect(query({ q: 'saga', library: 0, status: '', sort: 'added' })).toBe('?q=saga&sort=added');
    expect(query({})).toBe('');
  });
  it('computes percent read', () => {
    expect(percent(item(5, 10))).toBe(50);
    expect(percent(item(10, 10))).toBe(99);
    expect(percent(item(3, 10, 'read'))).toBe(100);
    expect(percent(item(0, 0))).toBe(0);
  });
  it('builds file URLs and knows readable formats', () => {
    expect(fileUrl(3)).toBe('/api/items/3/file');
    expect(fileUrl(3, true)).toBe('/api/items/3/file?inline=1');
    expect(fileUrl(3, false, 'mobi')).toBe('/api/items/3/file?format=mobi');
    expect(readable('epub')).toBe(true);
    expect(readable('mobi')).toBe(false);
  });
  it('names a series only when it is not the author grouping', () => {
    expect(seriesOf({ author: 'Frank Herbert', seriesName: 'Dune' })).toBe('Dune');
    expect(seriesOf({ author: 'John Jackson Miller', seriesName: 'John Jackson Miller ' })).toBe('');
    expect(seriesOf({ author: 'S. D. Perry, Weddle', seriesName: 's. d. perry' })).toBe('');
    expect(seriesOf({ author: '', seriesName: 'Saga' })).toBe('Saga');
  });
  it('formats sizes', () => {
    expect(formatSize(2048)).toBe('2 KB');
    expect(formatSize(5 * (1 << 20))).toBe('5.0 MB');
  });
});
