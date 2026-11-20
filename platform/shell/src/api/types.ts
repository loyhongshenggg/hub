// Mirrors platform/core/internal/api (AppInfo, AppsResponse) and runtime.LogLine.

export type AppStatus =
  | 'stopped'
  | 'starting'
  | 'running'
  | 'stopping'
  | 'error';

export interface AppInfo {
  id: string;
  name: string;
  description: string;
  /** URL path of the icon, if the manifest has one. */
  icon?: string;
  /** App origin, e.g. http://notes.hub.localhost:7000 */
  url: string;
  uiPath: string;
  keepAlive: boolean;
  autoStart: boolean;
  pinned: boolean;
  position: number | null;
  status: AppStatus;
  error?: string;
  since: string;
  removed?: boolean;
}

export interface Problem {
  id: string;
  path: string;
  error: string;
}

export interface AppsResponse {
  apps: AppInfo[];
  problems: Problem[];
}

export interface LogLine {
  service: string;
  stream: 'stdout' | 'stderr';
  text: string;
}

export type AppAction = 'start' | 'stop' | 'restart';

export interface AppPrefsPatch {
  pinned?: boolean;
  position?: number | null;
  /** null = use the manifest default */
  autoStart?: boolean | null;
}
