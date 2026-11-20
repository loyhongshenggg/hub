import {
  formatFiles,
  generateFiles,
  installPackagesTask,
  joinPathFragments,
  logger,
  readJson,
  type GeneratorCallback,
  type Tree,
} from '@nx/devkit';
import * as path from 'path';
import type { AppGeneratorSchema } from './schema';

const ID = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;

/** Web deps the generated app pins to the workspace's current versions. */
const WEB_DEPS = ['react', 'react-dom', 'lucide-react'];
const WEB_DEV_DEPS = [
  '@tailwindcss/vite',
  '@types/react',
  '@types/react-dom',
  '@vitejs/plugin-react',
  'tailwindcss',
  'typescript',
  'vite',
];

export interface Normalized {
  id: string;
  name: string;
  description: string;
  template: 'go-react' | 'image';
  image: string;
  port: number;
  healthPath: string;
  root: string;
}

export function normalize(options: AppGeneratorSchema): Normalized {
  const id = options.id?.trim() ?? '';
  if (!ID.test(id) || id.length > 40) {
    throw new Error(
      `Invalid app id "${id}": use lowercase letters, digits and single dashes, e.g. "notes" or "time-tracker".`,
    );
  }
  const template = options.template ?? 'go-react';
  if (template === 'image' && !options.image) {
    throw new Error(
      '--template=image needs --image (e.g. --image=ghcr.io/owner/app:1.2).',
    );
  }
  return {
    id,
    name:
      options.name?.trim() ||
      id
        .split('-')
        .map((w) => w[0].toUpperCase() + w.slice(1))
        .join(' '),
    description: options.description?.trim() ?? '',
    template,
    image: options.image ?? '',
    port: options.port ?? 8080,
    healthPath: options.healthPath ?? (template === 'image' ? '/' : '/healthz'),
    root: `apps/${id}`,
  };
}

/** A rounded tile with the app's initial, coloured by its id. */
export function iconSvg(o: Normalized): string {
  let h = 0;
  for (const c of o.id) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  const hue = h % 360;
  const initial = o.name.charAt(0).toUpperCase();
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="15" fill="hsl(${hue} 62% 48%)"/><text x="32" y="43" font-family="system-ui,sans-serif" font-size="30" font-weight="600" fill="#fff" text-anchor="middle">${initial}</text></svg>\n`;
}

function versions(tree: Tree, names: string[]): Record<string, string> {
  const pkg = readJson(tree, 'package.json');
  const all = { ...pkg.dependencies, ...pkg.devDependencies } as Record<
    string,
    string
  >;
  return Object.fromEntries(names.map((n) => [n, all[n] ?? 'latest']));
}

/** Adds ./apps/<id>/api to go.work's use block. */
export function addToGoWork(tree: Tree, dir: string) {
  const file = 'go.work';
  if (!tree.exists(file)) {
    tree.write(file, `go 1.27\n\nuse (\n\t./${dir}\n)\n`);
    return;
  }
  const src = tree.read(file, 'utf-8') ?? '';
  const entry = `./${dir}`;
  if (src.includes(entry)) return;
  const block = src.match(/use \(\n([\s\S]*?)\)/);
  if (block) {
    const lines = block[1]
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean);
    lines.push(entry);
    lines.sort();
    tree.write(
      file,
      src.replace(
        block[0],
        `use (\n${lines.map((l) => `\t${l}`).join('\n')}\n)`,
      ),
    );
  } else {
    tree.write(file, `${src.trimEnd()}\n\nuse ${entry}\n`);
  }
}

function goVersion(tree: Tree): string {
  const m = tree.read('go.work', 'utf-8')?.match(/^go (\d+\.\d+)/m);
  return m ? m[1] : '1.27';
}

export async function appGenerator(
  tree: Tree,
  options: AppGeneratorSchema,
): Promise<GeneratorCallback> {
  const o = normalize(options);
  if (tree.exists(o.root)) throw new Error(`${o.root} already exists.`);

  const vars = {
    ...o,
    tmpl: '',
    goVersion: goVersion(tree),
    webPackage: `@hub/${o.id}-web`,
    deps: JSON.stringify(versions(tree, WEB_DEPS), null, 2),
    devDeps: JSON.stringify(versions(tree, WEB_DEV_DEPS), null, 2),
  };
  generateFiles(tree, path.join(__dirname, 'files', 'common'), o.root, vars);
  generateFiles(tree, path.join(__dirname, 'files', o.template), o.root, vars);
  tree.write(joinPathFragments(o.root, 'icon.svg'), iconSvg(o));
  if (o.template === 'go-react') {
    tree.write(joinPathFragments(o.root, 'web/public/icon.svg'), iconSvg(o));
    addToGoWork(tree, `${o.root}/api`);
  }
  await formatFiles(tree);

  return () => {
    if (o.template === 'go-react' && !options.skipInstall)
      installPackagesTask(tree, true);
    logger.info(`
Created ${o.root} (${o.template}). Next:
  pnpm nx run ${o.id}:validate     # build, start, health-check, tear down
  Then power it on in the hub: http://hub.localhost:7000
Guide: docs/adding-an-app.md`);
  };
}

export default appGenerator;
