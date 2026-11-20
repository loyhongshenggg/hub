import { readJson, type Tree } from '@nx/devkit';
import { createTreeWithEmptyWorkspace } from '@nx/devkit/testing';
import { addToGoWork, appGenerator, normalize } from './generator';

describe('app generator', () => {
  let tree: Tree;

  beforeEach(() => {
    tree = createTreeWithEmptyWorkspace();
    tree.write(
      'go.work',
      'go 1.27.4\n\nuse (\n\t./libs/go-appkit\n\t./platform/core\n)\n',
    );
    const pkg = readJson(tree, 'package.json');
    pkg.devDependencies = {
      ...pkg.devDependencies,
      react: '^19.0.0',
      vite: '^8.0.0',
    };
    tree.write('package.json', JSON.stringify(pkg));
  });

  it('creates a Go + React app like apps/hello', async () => {
    await appGenerator(tree, {
      id: 'time-tracker',
      description: 'Track time',
      skipInstall: true,
    });
    const root = 'apps/time-tracker';
    for (const f of [
      'app.manifest.yaml',
      'compose.yaml',
      'Dockerfile',
      'icon.svg',
      'project.json',
      'README.md',
      'api/go.mod',
      'api/main.go',
      'api/main_test.go',
      'api/project.json',
      'api/ui/.gitkeep',
      'web/package.json',
      'web/vite.config.mts',
      'web/index.html',
      'web/src/app.tsx',
      'web/src/main.tsx',
      'web/public/icon.svg',
    ]) {
      expect(tree.exists(`${root}/${f}`), f).toBe(true);
    }
    const manifest = tree.read(`${root}/app.manifest.yaml`, 'utf-8');
    expect(manifest).toContain('id: time-tracker');
    expect(manifest).toMatch(/name: ["']Time Tracker["']/);
    expect(manifest).toMatch(/description: ["']Track time["']/);
    expect(manifest).toContain('service: web');
    expect(manifest).toContain('path: /healthz');

    expect(tree.read(`${root}/api/go.mod`, 'utf-8')).toContain(
      'module hub.localhost/apps/time-tracker/api\n\ngo 1.27\n',
    );
    expect(tree.read('go.work', 'utf-8')).toContain(
      '\t./apps/time-tracker/api\n\t./libs/go-appkit\n',
    );
    expect(tree.read(`${root}/Dockerfile`, 'utf-8')).toContain(
      '--filter @hub/time-tracker-web...',
    );

    const web = readJson(tree, `${root}/web/package.json`);
    expect(web.name).toBe('@hub/time-tracker-web');
    expect(web.dependencies).toMatchObject({
      react: '^19.0.0',
      '@hub/app-sdk': 'workspace:*',
      '@hub/ui-kit': 'workspace:*',
    });
    expect(web.devDependencies.vite).toBe('^8.0.0');
    expect(web.nx.tags).toEqual(['scope:app', 'app:time-tracker', 'lang:ts']);
    expect(readJson(tree, `${root}/api/project.json`).name).toBe(
      'time-tracker-api',
    );
    expect(readJson(tree, `${root}/project.json`).tags).toEqual([
      'scope:app',
      'app:time-tracker',
    ]);
  });

  it('wraps an existing image', async () => {
    await appGenerator(tree, {
      id: 'uptime',
      template: 'image',
      image: 'louislam/uptime-kuma:2',
      port: 3001,
    });
    const manifest = tree.read('apps/uptime/app.manifest.yaml', 'utf-8');
    expect(manifest).toContain('service: app');
    expect(manifest).toContain('port: 3001');
    expect(manifest).toContain('path: /\n');
    expect(tree.read('apps/uptime/compose.yaml', 'utf-8')).toContain(
      'image: "louislam/uptime-kuma:2"',
    );
    expect(tree.exists('apps/uptime/api')).toBe(false);
    expect(tree.read('go.work', 'utf-8')).not.toContain('uptime');
  });

  it('rejects bad input', async () => {
    expect(() => normalize({ id: 'Bad_Id' })).toThrow(/Invalid app id/);
    expect(() => normalize({ id: 'x-' })).toThrow(/Invalid app id/);
    expect(() => normalize({ id: 'x', template: 'image' })).toThrow(
      /needs --image/,
    );
    tree.write('apps/taken/app.manifest.yaml', '');
    await expect(
      appGenerator(tree, { id: 'taken', skipInstall: true }),
    ).rejects.toThrow(/already exists/);
  });

  it('adds to go.work without duplicates', () => {
    addToGoWork(tree, 'apps/a/api');
    addToGoWork(tree, 'apps/a/api');
    expect(tree.read('go.work', 'utf-8')?.match(/apps\/a\/api/g)).toHaveLength(
      1,
    );
  });
});

describe('escaping', () => {
  it('keeps free text valid in YAML, Go, JSON and TSX', async () => {
    const tree = createTreeWithEmptyWorkspace();
    tree.write('go.work', 'go 1.27\n');
    const name = `Tom's "R&D" <notes>`;
    await appGenerator(tree, {
      id: 'rd',
      name,
      description: 'a: b # c',
      skipInstall: true,
    });
    expect(tree.read('apps/rd/app.manifest.yaml', 'utf-8')).toContain(
      `name: ${JSON.stringify(name)}`,
    );
    expect(tree.read('apps/rd/app.manifest.yaml', 'utf-8')).toContain(
      'description: "a: b # c"',
    );
    expect(tree.read('apps/rd/api/main.go', 'utf-8')).toContain(
      JSON.stringify(`Hello from ${name}!`),
    );
    expect(readJson(tree, 'apps/rd/web/package.json').description).toContain(
      name,
    );
    expect(tree.read('apps/rd/web/src/app.tsx', 'utf-8')).toContain(
      `{${JSON.stringify(name)}}`,
    );
    expect(tree.read('apps/rd/web/index.html', 'utf-8')).toContain(
      '<title>Tom&#39;s &#34;R&amp;D&#34; &lt;notes&gt;</title>',
    );
  });
});
