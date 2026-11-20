import { beforeEach, describe, expect, it } from 'vitest';
import { useTabs } from './tabs';

describe('tabs store', () => {
  beforeEach(() => useTabs.setState({ open: [], active: null }));
  const s = () => useTabs.getState();

  it('opens once and activates', () => {
    s().openApp('a');
    s().openApp('b');
    s().openApp('a');
    expect(s().open).toEqual(['a', 'b']);
    expect(s().active).toBe('a');
  });

  it('closing the active tab activates its right neighbour, else left, else home', () => {
    ['a', 'b', 'c'].forEach((id) => s().openApp(id));
    s().activate('b');
    s().closeApp('b');
    expect(s().active).toBe('c');
    s().closeApp('c');
    expect(s().active).toBe('a');
    s().closeApp('a');
    expect(s().active).toBeNull();
  });

  it('closing an inactive tab keeps the active one', () => {
    ['a', 'b'].forEach((id) => s().openApp(id));
    s().closeApp('a');
    expect(s().active).toBe('b');
  });

  it('prunes tabs of apps that are gone', () => {
    ['a', 'b'].forEach((id) => s().openApp(id));
    s().prune(new Set(['a']));
    expect(s().open).toEqual(['a']);
    expect(s().active).toBeNull();
  });
});
