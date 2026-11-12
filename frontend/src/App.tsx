import { useEffect } from 'react';
import { AlertOctagon, Loader2 } from 'lucide-react';
import { Overlays } from './components/Dialogs';
import { useAutosave, useEngineEvents, useLiveValidation, useShortcuts } from './hooks/useAppEffects';
import { Canvas } from './panels/Canvas';
import { ExecutionPanel } from './panels/ExecutionPanel';
import { Inspector } from './panels/Inspector';
import { NodeLibrary } from './panels/NodeLibrary';
import { Sidebar } from './panels/Sidebar';
import { Topbar } from './panels/Topbar';
import { useWorkspace } from './stores/workspace';

export default function App() {
  const ready = useWorkspace((s) => s.ready);
  const fatalError = useWorkspace((s) => s.fatalError);

  useEngineEvents();
  useAutosave();
  useLiveValidation();
  useShortcuts();

  useEffect(() => {
    void useWorkspace.getState().init();
  }, []);

  if (!ready) {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-muted">
        <Loader2 size={18} className="animate-spin" /> Загрузка…
      </div>
    );
  }

  if (fatalError) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <div className="max-w-md rounded-xl border border-error/40 bg-panel p-6">
          <div className="flex items-center gap-2 text-error">
            <AlertOctagon size={18} /> <span className="font-semibold">Не удалось запустить Veylora</span>
          </div>
          <p className="selectable mt-3 text-[13px] leading-relaxed text-muted">{fatalError}</p>
          <p className="mt-3 text-xs text-faint">
            Проверьте, что приложение не запущено дважды и есть доступ к папке данных.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar />
        <div className="flex min-h-0 flex-1">
          <NodeLibrary />
          <div className="flex min-w-0 flex-1 flex-col">
            <main className="relative min-h-0 flex-1">
              <Canvas />
            </main>
            <ExecutionPanel />
          </div>
          <Inspector />
        </div>
      </div>
      <Overlays />
    </div>
  );
}
