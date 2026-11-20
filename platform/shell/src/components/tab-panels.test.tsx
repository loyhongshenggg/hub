import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
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
      (f) => !f.hidden,
    );
    expect(visible.map((f) => f.title)).toEqual(['Drop']);
  });
});
