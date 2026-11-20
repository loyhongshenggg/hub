import { envelope, parseAppMessage, type ShellMessage } from '@hub/app-sdk';
import { useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { useBridgeState } from '../state/bridge';
import { useTabs } from '../state/tabs';
import { toast } from '../state/toasts';
import { resolvedTheme, useUI } from '../state/ui';
import { appsKey } from './apps';
import type { AppsResponse } from './types';

/** App iframes carry data-app-id and data-origin (see TabPanels). */
function frames(): HTMLIFrameElement[] {
  return [
    ...document.querySelectorAll<HTMLIFrameElement>('iframe[data-app-id]'),
  ];
}

function post(frame: HTMLIFrameElement, msg: ShellMessage) {
  const origin = frame.dataset['origin'];
  if (origin) frame.contentWindow?.postMessage(envelope(msg), origin);
}

/**
 * The shell side of @hub/app-sdk. Accepts a message only if it comes from an
 * open app iframe and from that app's own origin, then validates it.
 */
export function useBridge() {
  const qc = useQueryClient();
  const theme = useUI((s) => s.theme);

  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      const frame = frames().find((f) => f.contentWindow === e.source);
      if (!frame || frame.dataset['origin'] !== e.origin) return;
      const apps = qc.getQueryData<AppsResponse>(appsKey)?.apps ?? [];
      const app = apps.find((a) => a.id === frame.dataset['appId']);
      const msg = parseAppMessage(e.data);
      if (!app || !msg) return;
      switch (msg.type) {
        case 'ready':
          post(frame, {
            type: 'context',
            context: {
              appId: app.id,
              theme: resolvedTheme(useUI.getState().theme),
              locale: navigator.language,
            },
          });
          break;
        case 'setBadge':
          useBridgeState.getState().setBadge(app.id, msg.count);
          break;
        case 'setTitle':
          useBridgeState.getState().setTitle(app.id, msg.title);
          break;
        case 'notify':
          toast(`${app.name}: ${msg.title}${msg.body ? ` — ${msg.body}` : ''}`);
          break;
        case 'openApp':
          if (apps.some((a) => a.id === msg.appId))
            useTabs.getState().openApp(msg.appId, msg.path);
          else toast(`${app.name} tried to open unknown app "${msg.appId}"`);
          break;
      }
    };
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [qc]);

  // Push theme changes (user toggle or OS change in "system" mode) to apps.
  useEffect(() => {
    const send = () =>
      frames().forEach((f) =>
        post(f, { type: 'theme', theme: resolvedTheme(theme) }),
      );
    send();
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    mq.addEventListener('change', send);
    return () => mq.removeEventListener('change', send);
  }, [theme]);
}
