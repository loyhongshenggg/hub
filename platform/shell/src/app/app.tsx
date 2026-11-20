import { TooltipProvider } from '@hub/ui-kit';
import { useEffect } from 'react';
import { useApps, useAppsStream } from '../api/apps';
import { useBridge } from '../api/bridge';
import { Dock } from '../components/dock';
import { Home } from '../components/home';
import { LogsDrawer } from '../components/logs-drawer';
import { TabBar } from '../components/tab-bar';
import { TabPanels } from '../components/tab-panels';
import { Toasts } from '../components/toasts';
import { useBridgeState } from '../state/bridge';
import { useTabs } from '../state/tabs';
import { useApplyTheme } from '../state/ui';

export function App() {
  useApplyTheme();
  const connected = useAppsStream();
  useBridge();
  const { data } = useApps();
  const active = useTabs((s) => s.active);
  const prune = useTabs((s) => s.prune);

  useEffect(() => {
    if (!data) return;
    prune(new Set(data.apps.map((a) => a.id)));
    // Badges and titles describe a running app; drop them once it stops.
    for (const a of data.apps)
      if (a.status !== 'running') useBridgeState.getState().reset(a.id);
  }, [data, prune]);

  return (
    <TooltipProvider>
      <div className="flex h-full">
        <Dock />
        <div className="flex min-w-0 flex-1 flex-col">
          <TabBar connected={connected} />
          <main className="relative min-h-0 flex-1">
            <div hidden={active !== null} className="absolute inset-0">
              <Home />
            </div>
            <TabPanels />
          </main>
          <LogsDrawer />
        </div>
      </div>
      <Toasts />
    </TooltipProvider>
  );
}

export default App;
