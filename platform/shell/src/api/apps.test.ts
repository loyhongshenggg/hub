import { describe, expect, it } from 'vitest';
import { app } from '../test/fixtures';
import { dockApps } from './apps';

describe('dockApps', () => {
  it('lists pinned apps by position then name, then other apps that are not off', () => {
    const apps = [
      app('zed', { pinned: true }),
      app('beta', { pinned: true, position: 2 }),
      app('alpha', { pinned: true, position: 1 }),
      app('off'),
      app('busy', { status: 'running' }),
      app('broken', { status: 'error' }),
    ];
    const { pinned, running } = dockApps(apps);
    expect(pinned.map((a) => a.id)).toEqual(['alpha', 'beta', 'zed']);
    expect(running.map((a) => a.id)).toEqual(['broken', 'busy']);
  });
});
