// Module-boundary rules, shared by eslint.config.mjs (TS imports) and
// check.mjs (whole Nx project graph, incl. Go module imports).
//
// Tags every project must carry:
//   scope:platform | scope:app | scope:lib | scope:tool
//   app:<id>       — every project under apps/<id>/
import { existsSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export const workspaceRoot = join(
  dirname(fileURLToPath(import.meta.url)),
  '..',
  '..',
);

export const SCOPES = [
  'scope:platform',
  'scope:app',
  'scope:lib',
  'scope:tool',
];

export function appIds() {
  const dir = join(workspaceRoot, 'apps');
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);
}

export function depConstraints() {
  return [
    { sourceTag: 'scope:lib', onlyDependOnLibsWithTags: ['scope:lib'] },
    {
      sourceTag: 'scope:platform',
      onlyDependOnLibsWithTags: ['scope:lib', 'scope:platform'],
    },
    {
      sourceTag: 'scope:tool',
      onlyDependOnLibsWithTags: ['scope:lib', 'scope:tool'],
    },
    {
      sourceTag: 'scope:app',
      onlyDependOnLibsWithTags: ['scope:lib', 'scope:app'],
    },
    // Projects inside one app may depend on each other, never on another app.
    ...appIds().map((id) => ({
      sourceTag: `app:${id}`,
      onlyDependOnLibsWithTags: ['scope:lib', `app:${id}`],
    })),
  ];
}
