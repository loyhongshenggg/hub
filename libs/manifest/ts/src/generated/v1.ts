/* Generated from libs/manifest/schema/v1.schema.json by scripts/codegen.mjs — do not edit. */

/**
 * hub app manifest, version 1 (apps/<id>/app.manifest.yaml).
 */
export interface AppManifestV1 {
  /**
   * Contract version. Breaking changes bump this; old versions keep working.
   */
  manifestVersion: 1;
  /**
   * Unique app id. Becomes the subdomain (<id>.hub.localhost) and compose project (hub-<id>).
   */
  id: string;
  /**
   * Display name in the dock and tabs.
   */
  name: string;
  /**
   * One line shown on the home grid.
   */
  description?: string;
  /**
   * Icon file (SVG or PNG), relative to the manifest.
   */
  icon?: string;
  /**
   * How the core runs the app.
   */
  runtime: {
    /**
     * Runtime driver.
     */
    driver: 'docker';
    /**
     * Compose file, relative to the manifest.
     */
    compose?: string;
    /**
     * Compose service the gateway routes to.
     */
    service: string;
    /**
     * HTTP port inside the container.
     */
    port: number;
  };
  /**
   * Readiness check.
   */
  health: {
    /**
     * GET path that returns 2xx when the app is ready.
     */
    path: string;
    /**
     * How long to wait for the first 2xx after start, e.g. 60s, 2m.
     */
    startTimeout?: string;
  };
  /**
   * How the shell shows the app.
   */
  ui?: {
    /**
     * Page opened in the tab.
     */
    path?: string;
    /**
     * Keep the iframe mounted when switching tabs.
     */
    keepAlive?: boolean;
  };
  /**
   * Start the app when the core starts.
   */
  autoStart?: boolean;
}
