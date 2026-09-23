import { describe, expect, it } from 'vitest';
import { chapterFromPath, parseLocator, progressFor } from './reading';

describe('reading helpers', () => {
  it('parses locators defensively', () => {
    expect(parseLocator('{"chapter":3,"scroll":0.4}')).toEqual({ chapter: 3, scroll: 0.4 });
    expect(parseLocator('{"chapter":3,"scroll":7}')).toEqual({ chapter: 3, scroll: 1 });
    expect(parseLocator('')).toBeNull();
    expect(parseLocator('{"chapter":-1}')).toBeNull();
  });
  it('reads the chapter from iframe paths', () => {
    expect(chapterFromPath('/read/12/chapter/4', 12)).toBe(4);
    expect(chapterFromPath('/read/13/chapter/4', 12)).toBeNull();
    expect(chapterFromPath('/read/12/res/4', 12)).toBeNull();
  });
  it('marks read only at the end of the last chapter', () => {
    expect(progressFor(0, 10, 0.5)).toMatchObject({ page: 1, status: 'reading' });
    expect(progressFor(9, 10, 0.5)).toMatchObject({ page: 9, status: 'reading' });
    expect(progressFor(9, 10, 0.99)).toMatchObject({ page: 10, status: 'read' });
    expect(progressFor(0, 1, 0.1)).toMatchObject({ page: 0, status: 'reading' });
  });
});
