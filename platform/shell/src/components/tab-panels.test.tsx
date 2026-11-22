import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useTabs } from '../state/tabs';
import { app, renderWithApps } from '../test/fixtures';
import { TabPanels } from './tab-panels';

describe('TabPanels', () => {
  beforeEach(() =>
    useTabs.setState({ open: ['keep', 'drop', 'off'], active: 'off' }),
  );

  it('keeps keepAlive iframes mounted but hidden, unmounts the rest, shows the off panel', () => {
    const { container } = renderWithApps(<TabPanels />, [
      app('keep', { status: 'running' }),
      app('drop', { status: 'running', keepAlive: false }),
      app('off'),
    ]);
    const frames = container.querySelectorAll('iframe');
    expect(frames).toHaveLength(1);
    expect(frames[0]).toHaveAttribute('title', 'Keep');
    expect(frames[0]).toHaveAttribute('src', 'http://keep.hub.localhost:7000/');
    expect(frames[0]).not.toBeVisible();
    expect(screen.getByText('This app is off.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /power on/i })).toBeEnabled();
  });

  it('shows the active running app', () => {
    useTabs.setState({ active: 'drop' });
    const { container } = renderWithApps(<TabPanels />, [
      app('keep', { status: 'running' }),
      app('drop', { status: 'running', keepAlive: false }),
      app('off'),
    ]);
    const visible = [...container.querySelectorAll('iframe')].filter(
      (f) => f.style.visibility !== 'hidden' && !f.inert,
    );
    expect(visible.map((f) => f.title)).toEqual(['Drop']);
  });

  describe('dev mode', () => {
    afterEach(() => vi.unstubAllGlobals());

    it('offers Dev mode only when the manifest has a dev block', async () => {
      const fetchMock = vi.fn(async () =>
        Response.json(app('off', { status: 'starting', mode: 'dev' })),
      );
      vi.stubGlobal('fetch', fetchMock);
      renderWithApps(<TabPanels />, [app('off', { devAvailable: true })]);
      await userEvent.click(screen.getByRole('button', { name: /dev mode/i }));
      await waitFor(() =>
        expect(fetchMock).toHaveBeenCalledWith(
          '/api/apps/off/start?mode=dev',
          expect.objectContaining({ method: 'POST' }),
        ),
      );
    });

    it('hides Dev mode without a dev block', () => {
      renderWithApps(<TabPanels />, [app('off')]);
      expect(screen.queryByRole('button', { name: /dev mode/i })).toBeNull();
    });
  });
});
