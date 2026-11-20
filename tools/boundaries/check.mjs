// Enforces module boundaries on the whole Nx project graph — TS and Go alike.
// ESLint only sees TS imports; this also catches Go imports (via gonx) and
// implicit dependencies. Exit 1 on any violation.
import { createProjectGraphAsync } from '@nx/devkit';
import { SCOPES, depConstraints } from './constraints.mjs';

const SELF = 'boundaries';
const graph = await createProjectGraphAsync({ exitOnError: true });
const constraints = depConstraints();
const errors = [];

for (const [name, node] of Object.entries(graph.nodes)) {
  const tags = node.data.tags ?? [];
  const root = node.data.root;
  const scopes = tags.filter((t) => SCOPES.includes(t));
  if (scopes.length !== 1) {
    errors.push(
      `${name} (${root}): needs exactly one of ${SCOPES.join(', ')}; has [${tags.join(', ')}]`,
    );
  }
  const appId = root.match(/^apps\/([^/]+)/)?.[1];
  if (appId && !(tags.includes('scope:app') && tags.includes(`app:${appId}`))) {
    errors.push(
      `${name} (${root}): projects under apps/${appId}/ need tags scope:app and app:${appId}`,
    );
  }
}

for (const [source, deps] of Object.entries(graph.dependencies)) {
  if (source === SELF || !graph.nodes[source]) continue;
  const sourceTags = graph.nodes[source].data.tags ?? [];
  for (const dep of deps) {
    const target = graph.nodes[dep.target];
    if (!target) continue; // external (npm:, go module outside the workspace)
    const targetTags = target.data.tags ?? [];
    for (const c of constraints) {
      if (!sourceTags.includes(c.sourceTag)) continue;
      if (!c.onlyDependOnLibsWithTags.some((t) => targetTags.includes(t))) {
        errors.push(
          `${source} → ${dep.target} (${dep.type}): ${c.sourceTag} may only depend on [${c.onlyDependOnLibsWithTags.join(', ')}]`,
        );
      }
    }
  }
}

if (errors.length) {
  console.error(
    `Module boundary violations (${errors.length}):\n  ` + errors.join('\n  '),
  );
  process.exit(1);
}
console.log(
  `Module boundaries OK (${Object.keys(graph.nodes).length} projects).`,
);
