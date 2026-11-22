import { describe, expect, it, vi } from 'vitest';
import {
  connect,
  envelope,
  parseAppMessage,
  parseShellMessage,
  PROTOCOL_VERSION,
  shellOriginFor,
  syncThemeClass,
  applyThemeClass,
} from './index.js';

describe('shellOriginFor', () => {
  it('drops the first host label', () => {
    expect(
      shellOriginFor({ protocol: 'http:', host: 'notes.hub.localhost:7000' }),
    ).toBe('http://hub.localhost:7000');
    expect(shellOriginFor({ protocol: 'http:', host: 'localhost:5173' })).toBe(
      'http://localhost:5173',
    );
  });
});

describe('parseAppMessage', () => {
  it('accepts valid messages and rejects malformed ones', () => {
    expect(parseAppMessage({ hub: 1, type: 'setBadge', count: 3.7 })).toEqual({
      type: 'setBadge',
      count: 3,
    });
    expect(
      parseAppMessage({ hub: 1, type: 'openApp', appId: 'notes', path: '/x' }),
    ).toEqual({ type: 'openApp', appId: 'notes', path: '/x' });
    expect(parseAppMessage({ hub: 1, type: 'setTitle', title: null })).toEqual({
      type: 'setTitle',
      title: null,
    });
    for (const bad of [
      null,
      'ready',
      { type: 'ready' },
      { hub: 2, type: 'ready' },
      { hub: 1, type: 'setBadge', count: -1 },
      { hub: 1, type: 'setBadge', count: Infinity },
      { hub: 1, type: 'notify', title: 'x'.repeat(201) },
      { hub: 1, type: 'openApp', appId: 'notes', path: 'javascript:alert(1)' },
      { hub: 1, type: 'unknown' },
    ]) {
      expect(parseAppMessage(bad), JSON.stringify(bad)).toBeNull();
    }
  });
});

describe('parseShellMessage', () => {
  it('validates context and theme', () => {
    const ctx = { appId: 'notes', theme: 'dark', locale: 'en' };
    expect(
      parseShellMessage(envelope({ type: 'context', context: ctx as never })),
    ).toEqual({ type: 'context', context: ctx });
    expect(
      parseShellMessage({ hub: 1, type: 'theme', theme: 'blue' }),
    ).toBeNull();
  });
});

/** A fake app window inside a shell iframe. */
function fakeFrame() {
  const target = new EventTarget();
  const parent = { postMessage: vi.fn() };
  const win = Object.assign(target, {
    parent,
    location: { protocol: 'http:', host: 'notes.hub.localhost:7000' },
  }) as unknown as Window;
  const deliver = (
    data: unknown,
    origin = 'http://hub.localhost:7000',
    source: unknown = parent,
  ) =>
    target.dispatchEvent(
      Object.assign(new Event('message'), { data, origin, source }),
    );
  return { win, parent, deliver };
}

describe('connect', () => {
  it('outside the shell: not in shell, sends nothing', () => {
    const post = vi.spyOn(window, 'postMessage');
    const hub = connect();
    expect(hub.inShell).toBe(false);
    hub.setBadge(2);
    expect(post).not.toHaveBeenCalled();
    hub.disconnect();
  });

  it('in the shell: says ready, receives context and theme, sends to the shell origin only', () => {
    const { win, parent, deliver } = fakeFrame();
    const hub = connect({ window: win });
    expect(hub.inShell).toBe(true);
    expect(parent.postMessage).toHaveBeenCalledWith(
      { hub: PROTOCOL_VERSION, type: 'ready' },
      'http://hub.localhost:7000',
    );

    const themes: string[] = [];
    hub.onTheme((t) => themes.push(t));
    deliver(
      envelope({
        type: 'context',
        context: { appId: 'notes', theme: 'dark', locale: 'en' },
      }),
    );
    expect(hub.context?.appId).toBe('notes');
    deliver(envelope({ type: 'theme', theme: 'light' }));
    expect(themes).toEqual(['light', 'dark', 'light']);

    // Ignored: wrong origin, wrong source, garbage.
    deliver(
      envelope({ type: 'theme', theme: 'dark' }),
      'http://evil.hub.localhost:7000',
    );
    deliver(
      envelope({ type: 'theme', theme: 'dark' }),
      'http://hub.localhost:7000',
      {},
    );
    deliver({ type: 'theme', theme: 'dark' });
    expect(hub.theme).toBe('light');

    hub.notify('Saved', 'All good');
    hub.openApp('budget', '/month');
    expect(parent.postMessage).toHaveBeenLastCalledWith(
      { hub: 1, type: 'openApp', appId: 'budget', path: '/month' },
      'http://hub.localhost:7000',
    );
    hub.disconnect();
  });

  it('syncThemeClass toggles the dark class', () => {
    const { win, deliver } = fakeFrame();
    const hub = connect({ window: win });
    const el = document.createElement('html');
    syncThemeClass(hub, el);
    deliver(envelope({ type: 'theme', theme: 'dark' }));
    expect(el.classList.contains('dark')).toBe(true);
    expect(el.style.colorScheme).toBe('dark');
  });
});

describe('applyThemeClass', () => {
  it('switches with colour transitions paused, then restores them', () => {
    vi.useFakeTimers();
    const el = document.documentElement;
    el.classList.remove('dark');
    el.style.colorScheme = 'light';
    const styles = () => document.head.querySelectorAll('style').length;
    const before = styles();
    applyThemeClass('dark');
    expect(el.classList.contains('dark')).toBe(true);
    expect(el.style.colorScheme).toBe('dark');
    expect(styles()).toBe(before + 1); // transitions paused for the switch
    vi.runAllTimers();
    expect(styles()).toBe(before);
    applyThemeClass('dark'); // already dark: no work, nothing paused
    expect(styles()).toBe(before);
    vi.useRealTimers();
  });
});
