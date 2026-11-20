import { TooltipProvider } from '@hub/ui-kit';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { app, renderWithApps } from '../test/fixtures';
import { AppCard } from './app-card';

describe('AppCard', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('powers the app on through the API and shows the new status', async () => {
    const fetchMock = vi.fn(async () =>
      Response.json(app('notes', { status: 'starting' }), { status: 202 }),
    );
    vi.stubGlobal('fetch', fetchMock);
    const notes = app('notes', { description: 'Markdown notes' });
    renderWithApps(
      <TooltipProvider>
        <AppCard app={notes} />
      </TooltipProvider>,
      [notes],
    );
    expect(screen.getByText('Markdown notes')).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole('switch', { name: /power notes on/i }),
    );
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/apps/notes/start',
        expect.objectContaining({ method: 'POST' }),
      ),
    );
  });

  it('shows the error of a failed app', () => {
    const broken = app('broken', {
      status: 'error',
      error: 'not healthy after 60s',
    });
    renderWithApps(
      <TooltipProvider>
        <AppCard app={broken} />
      </TooltipProvider>,
      [broken],
    );
    expect(screen.getByText('not healthy after 60s')).toBeInTheDocument();
  });
});
