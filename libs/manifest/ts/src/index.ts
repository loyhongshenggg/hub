import type { AppManifestV1 } from './generated/v1.js';

export type { AppManifestV1 };

/** The newest manifest shape. */
export type AppManifest = AppManifestV1;

/** File every app ships at its folder root. */
export const MANIFEST_FILE = 'app.manifest.yaml';

/** Every supported manifestVersion, oldest first. */
export const MANIFEST_VERSIONS = [1] as const;

export const LATEST_MANIFEST_VERSION = 1;
