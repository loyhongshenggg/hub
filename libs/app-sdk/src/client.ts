import {
  envelope,
  parseShellMessage,
  type AppMessage,
  type HubContext,
  type Theme,
} from './protocol.js';

export interface HubClient {
  /** True when running inside the hub shell's iframe. */
  readonly inShell: boolean;
  readonly shellOrigin: string;
  /** Set once the shell has answered `ready`. */
  readonly context: HubContext | null;
  /** The hub's theme in the shell; the OS preference outside it. */
  readonly theme: Theme;
  /** Calls fn with the context now (if known) and whenever it changes. */
  onContext(fn: (c: HubContext) => void): () => void;
  /** Calls fn with the theme now and whenever it changes. */
  onTheme(fn: (t: Theme) => void): () => void;
  /** Badge on the app's tab and dock icon; 0 clears it. */
  setBadge(count: number): void;
  /** Short message shown by the shell. */
  notify(title: string, body?: string): void;
  /** Open another app's tab; path is used if the tab isn't open yet. */
  openApp(appId: string, path?: string): void;
  /** Tab title; null restores the app's name. */
  setTitle(title: string | null): void;
  disconnect(): void;
}

export interface ConnectOptions {
  /** Shell origin. Default: this app's origin minus its first label (notes.hub.localhost → hub.localhost). */
  shellOrigin?: string;
  /** The app's window (tests). */
  window?: Window;
}

/** http://notes.hub.localhost:7000 → http://hub.localhost:7000 */
export function shellOriginFor(loc: {
  protocol: string;
  host: string;
}): string {
  const labels = loc.host.split('.');
  return `${loc.protocol}//${labels.length > 1 ? labels.slice(1).join('.') : loc.host}`;
}

/**
 * Connects to the hub shell. Safe outside the shell too: then messages are
 * dropped and the theme follows the OS. Call once per page.
 */
export function connect(opts: ConnectOptions = {}): HubClient {
  const win = opts.window ?? window;
  const parent = win.parent;
  const inShell = parent !== win;
  const shellOrigin = opts.shellOrigin ?? shellOriginFor(win.location);
  const mq =
    typeof win.matchMedia === 'function'
      ? win.matchMedia('(prefers-color-scheme: dark)')
      : undefined;

  let context: HubContext | null = null;
  let theme: Theme = mq?.matches ? 'dark' : 'light';
  const contextFns = new Set<(c: HubContext) => void>();
  const themeFns = new Set<(t: Theme) => void>();

  const setTheme = (t: Theme) => {
    if (t === theme) return;
    theme = t;
    themeFns.forEach((fn) => fn(t));
  };
  const send = (msg: AppMessage) => {
    if (inShell) parent.postMessage(envelope(msg), shellOrigin);
  };

  const onMessage = (e: MessageEvent) => {
    if (e.source !== parent || e.origin !== shellOrigin) return;
    const msg = parseShellMessage(e.data);
    if (!msg) return;
    if (msg.type === 'context') {
      context = msg.context;
      contextFns.forEach((fn) => fn(msg.context));
      setTheme(msg.context.theme);
    } else {
      if (context) context = { ...context, theme: msg.theme };
      setTheme(msg.theme);
    }
  };
  const onSystemTheme = () => setTheme(mq?.matches ? 'dark' : 'light');

  win.addEventListener('message', onMessage as EventListener);
  if (!inShell) mq?.addEventListener('change', onSystemTheme);
  send({ type: 'ready' });

  return {
    inShell,
    shellOrigin,
    get context() {
      return context;
    },
    get theme() {
      return theme;
    },
    onContext(fn) {
      contextFns.add(fn);
      if (context) fn(context);
      return () => void contextFns.delete(fn);
    },
    onTheme(fn) {
      themeFns.add(fn);
      fn(theme);
      return () => void themeFns.delete(fn);
    },
    setBadge: (count) =>
      send({ type: 'setBadge', count: Math.max(0, Math.floor(count)) }),
    notify: (title, body) => send({ type: 'notify', title, body }),
    openApp: (appId, path) => send({ type: 'openApp', appId, path }),
    setTitle: (title) => send({ type: 'setTitle', title }),
    disconnect() {
      win.removeEventListener('message', onMessage as EventListener);
      mq?.removeEventListener('change', onSystemTheme);
      contextFns.clear();
      themeFns.clear();
    },
  };
}

/**
 * Sets `class="dark"` and `color-scheme` on an element (default <html>).
 * Colour transitions are paused for the switch itself: otherwise every
 * button and link animates its new colours at once, which is slow on large
 * high-DPI screens. Does nothing when the theme is already applied.
 */
export function applyThemeClass(
  t: Theme,
  el: HTMLElement = document.documentElement,
): void {
  const dark = t === 'dark';
  if (el.classList.contains('dark') === dark && el.style.colorScheme === t) {
    return;
  }
  const doc = el.ownerDocument;
  const pause = doc.createElement('style');
  pause.textContent =
    '*,*::before,*::after{transition:none!important}';
  doc.head.appendChild(pause);
  el.classList.toggle('dark', dark);
  el.style.colorScheme = t;
  // Apply the new colours now, while transitions are off.
  void doc.defaultView?.getComputedStyle(doc.body).color;
  setTimeout(() => pause.remove(), 0);
}

/**
 * Keeps `class="dark"` and `color-scheme` on an element (default <html>) in
 * sync with the hub theme — what @hub/ui-kit's tokens expect.
 */
export function syncThemeClass(
  hub: HubClient,
  el: HTMLElement = document.documentElement,
): () => void {
  return hub.onTheme((t) => applyThemeClass(t, el));
}
