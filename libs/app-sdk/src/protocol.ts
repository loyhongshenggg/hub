/**
 * The shell ⇄ app postMessage protocol. Every message is an envelope
 * `{ hub: PROTOCOL_VERSION, type, ...fields }`. Additive changes keep the
 * version; breaking ones bump it (and the shell keeps speaking old versions).
 */
export const PROTOCOL_VERSION = 1;

export type Theme = 'light' | 'dark';

export interface HubContext {
  appId: string;
  theme: Theme;
  /** BCP 47 tag, e.g. "en-US". */
  locale: string;
}

/** Shell → app. */
export type ShellMessage =
  | { type: 'context'; context: HubContext }
  | { type: 'theme'; theme: Theme };

/** App → shell. */
export type AppMessage =
  | { type: 'ready' }
  /** 0 clears the badge. */
  | { type: 'setBadge'; count: number }
  | { type: 'notify'; title: string; body?: string }
  | { type: 'openApp'; appId: string; path?: string }
  /** null restores the app's name. */
  | { type: 'setTitle'; title: string | null };

export type Envelope<M> = M & { hub: typeof PROTOCOL_VERSION };

export function envelope<M extends ShellMessage | AppMessage>(
  msg: M,
): Envelope<M> {
  return { hub: PROTOCOL_VERSION, ...msg };
}

const isObj = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null;
const str = (v: unknown, max: number): v is string =>
  typeof v === 'string' && v.length <= max;
const isTheme = (v: unknown): v is Theme => v === 'light' || v === 'dark';

/** Validates an untrusted message from an app frame; null if malformed. */
export function parseAppMessage(data: unknown): AppMessage | null {
  if (!isObj(data) || data['hub'] !== PROTOCOL_VERSION) return null;
  switch (data['type']) {
    case 'ready':
      return { type: 'ready' };
    case 'setBadge': {
      const n = data['count'];
      return typeof n === 'number' && Number.isFinite(n) && n >= 0
        ? { type: 'setBadge', count: Math.floor(n) }
        : null;
    }
    case 'notify':
      if (
        !str(data['title'], 200) ||
        (data['body'] !== undefined && !str(data['body'], 2000))
      )
        return null;
      return {
        type: 'notify',
        title: data['title'],
        body: data['body'] as string | undefined,
      };
    case 'openApp':
      if (
        !str(data['appId'], 40) ||
        (data['path'] !== undefined &&
          !(str(data['path'], 2000) && data['path'].startsWith('/')))
      )
        return null;
      return {
        type: 'openApp',
        appId: data['appId'],
        path: data['path'] as string | undefined,
      };
    case 'setTitle':
      if (data['title'] !== null && !str(data['title'], 200)) return null;
      return { type: 'setTitle', title: data['title'] as string | null };
  }
  return null;
}

/** Validates an untrusted message from the shell; null if malformed. */
export function parseShellMessage(data: unknown): ShellMessage | null {
  if (!isObj(data) || data['hub'] !== PROTOCOL_VERSION) return null;
  if (data['type'] === 'theme' && isTheme(data['theme']))
    return { type: 'theme', theme: data['theme'] };
  if (data['type'] === 'context' && isObj(data['context'])) {
    const c = data['context'];
    if (
      typeof c['appId'] === 'string' &&
      isTheme(c['theme']) &&
      typeof c['locale'] === 'string'
    ) {
      return {
        type: 'context',
        context: { appId: c['appId'], theme: c['theme'], locale: c['locale'] },
      };
    }
  }
  return null;
}
