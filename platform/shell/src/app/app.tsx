import { TooltipProvider } from '@hub/ui-kit';
import { useEffect } from 'react';
import { useApps, useAppsStream } from '../api/apps';
import { Dock } from '../components/dock';
import { Home } from '../components/home';
import { LogsDrawer } from '../components/logs-drawer';
import { TabBar } from '../components/tab-bar';
import { TabPanels } from '../components/tab-panels';
import { Toasts } from '../components/toasts';
import { useTabs } from '../state/tabs';
import { useApplyTheme } from '../state/ui';

export function App() {
  useApplyTheme();
  const connected = useAppsStream();
  const { data } = useApps();
  const active = useTabs((s) => s.active);
  const prune = useTabs((s) => s.prune);

  useEffect(() => {
    if (data) prune(new Set(data.apps.map((a) => a.id)));
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
