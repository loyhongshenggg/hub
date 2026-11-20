// Generates src/generated/v<N>.ts from ../schema/v<N>.schema.json.
// The JSON Schemas are the source of truth; never edit generated files.
import { readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { compileFromFile } from 'json-schema-to-typescript';

const here = dirname(fileURLToPath(import.meta.url));
const schemaDir = join(here, '..', '..', 'schema');
const outDir = join(here, '..', 'src', 'generated');

for (const file of readdirSync(schemaDir)
  .filter((f) => /^v\d+\.schema\.json$/.test(f))
  .sort()) {
  const version = file.match(/^v(\d+)/)[1];
  const ts = await compileFromFile(join(schemaDir, file), {
    bannerComment: `/* Generated from libs/manifest/schema/${file} by scripts/codegen.mjs — do not edit. */`,
    additionalProperties: false,
    style: { singleQuote: true, printWidth: 100 },
  });
  writeFileSync(join(outDir, `v${version}.ts`), ts);
  console.log(`generated src/generated/v${version}.ts`);
}
