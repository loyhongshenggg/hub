import { act } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TabPanels } from '../components/tab-panels';
import { useBridgeState } from '../state/bridge';
import { useTabs } from '../state/tabs';
import { useToasts } from '../state/toasts';
import { app, renderWithApps } from '../test/fixtures';
import { useBridge } from './bridge';

function Harness() {
  useBridge();
  return <TabPanels />;
}

const notes = app('notes', { status: 'running' });
const budget = app('budget', { status: 'running' });

function setup() {
  const { container } = renderWithApps(<Harness />, [notes, budget]);
  const frame = container.querySelector<HTMLIFrameElement>(
    'iframe[data-app-id="notes"]',
  );
  if (!frame?.contentWindow) throw new Error('no notes frame');
  const send = (data: unknown, origin = notes.url) =>
    act(() =>
      window.dispatchEvent(
        new MessageEvent('message', {
          data,
          origin,
          source: frame.contentWindow,
        }),
      ),
    );
  return { frame, send };
}

describe('bridge', () => {
  beforeEach(() => {
    useTabs.setState({ open: ['notes'], active: 'notes', paths: {} });
    useBridgeState.setState({ badges: {}, titles: {} });
    useToasts.setState({ toasts: [] });
  });

  it('answers ready with the app context, sent to the app origin', () => {
    const { frame, send } = setup();
    const post = vi.spyOn(frame.contentWindow as Window, 'postMessage');
    send({ hub: 1, type: 'ready' });
    expect(post).toHaveBeenCalledWith(
      expect.objectContaining({
        hub: 1,
        type: 'context',
        context: expect.objectContaining({ appId: 'notes' }),
      }),
      'http://notes.hub.localhost:7000',
    );
  });

  it('applies badge, title, notify and openApp', () => {
    const { send } = setup();
    send({ hub: 1, type: 'setBadge', count: 4 });
    send({ hub: 1, type: 'setTitle', title: 'Inbox (4)' });
    send({ hub: 1, type: 'notify', title: 'Saved' });
    send({ hub: 1, type: 'openApp', appId: 'budget', path: '/month' });
    expect(useBridgeState.getState().badges).toEqual({ notes: 4 });
    expect(useBridgeState.getState().titles).toEqual({ notes: 'Inbox (4)' });
    expect(useToasts.getState().toasts.map((t) => t.message)).toEqual([
      'Notes: Saved',
    ]);
    expect(useTabs.getState().open).toEqual(['notes', 'budget']);
    expect(useTabs.getState().active).toBe('budget');
  });

  it('ignores messages from the wrong origin or malformed ones', () => {
    const { send } = setup();
    send(
      { hub: 1, type: 'setBadge', count: 4 },
      'http://budget.hub.localhost:7000',
    );
    send({ hub: 1, type: 'setBadge', count: -4 });
    send({ type: 'setBadge', count: 4 });
    expect(useBridgeState.getState().badges).toEqual({});
  });
});
