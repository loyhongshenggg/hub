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
  /**
   * Optional dev mode: the core runs these processes on the host (live reload) instead of the containers, and routes the app's URL to one of them.
   */
  dev?: {
    /**
     * Name of the process the gateway routes to (it serves the UI and health path). Default: the first process.
     */
    route?: string;
    /**
     * Processes to run. Each gets its own free port in $PORT; every process also sees $HUB_PORT_<NAME> for all of them.
     *
     * @minItems 1
     * @maxItems 8
     */
    processes:
      | [
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          }
        ]
      | [
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          }
        ]
      | [
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          }
        ]
      | [
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          }
        ]
      | [
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          }
        ]
      | [
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          }
        ]
      | [
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          }
        ]
      | [
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          },
          {
            /**
             * Short name, e.g. api or web.
             */
            name: string;
            /**
             * Working directory, relative to the manifest. Default: the app folder.
             */
            dir?: string;
            /**
             * Shell command (run with /bin/sh -c).
             */
            command: string;
            /**
             * File name patterns (e.g. *.go, go.mod); the core restarts the process when a matching file under dir changes.
             */
            watch?: string[];
          }
        ];
  };
}
