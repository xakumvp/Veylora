import {
  AlertTriangle,
  Check,
  CloudOff,
  Download,
  Loader2,
  Play,
  Redo2,
  Save,
  Square,
  Undo2,
  Upload,
  Zap,
} from 'lucide-react';
import { Button, cx, IconButton, Toggle } from '../components/ui';
import { useEditor } from '../stores/editor';
import { useRun } from '../stores/run';
import { useUI } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';

function SaveIndicator() {
  const state = useEditor((s) => s.saveState);
  const error = useEditor((s) => s.saveError);
  const map = {
    saved: { icon: <Check size={13} />, text: 'Сохранено', cls: 'text-faint' },
    dirty: {
      icon: <span className="size-1.5 rounded-full bg-warning" />,
      text: 'Есть изменения',
      cls: 'text-muted',
    },
    saving: { icon: <Loader2 size={13} className="animate-spin" />, text: 'Сохранение…', cls: 'text-muted' },
    error: { icon: <CloudOff size={13} />, text: 'Ошибка сохранения', cls: 'text-error' },
  }[state];
  return (
    <span className={cx('flex items-center gap-1.5 text-xs', map.cls)} title={error} aria-live="polite">
      {map.icon}
      {map.text}
    </span>
  );
}

export function Topbar() {
  const workflowId = useEditor((s) => s.workflowId);
  const name = useEditor((s) => s.name);
  const canUndo = useEditor((s) => s.past.length > 0);
  const canRedo = useEditor((s) => s.future.length > 0);
  const { undo, redo, save } = useEditor.getState();
  const activeRunId = useRun((s) => (workflowId ? s.activeRuns[workflowId] : undefined));
  const otherRuns = useRun((s) => Object.keys(s.activeRuns).filter((id) => id !== workflowId).length);
  const starting = useRun((s) => s.starting);
  const errorCount = useRun((s) => s.issues.filter((i) => i.severity === 'error').length);
  const activeProjectId = useWorkspace((s) => s.activeProjectId);
  const automated = useWorkspace((s) => s.isActive(workflowId));
  const toggling = useWorkspace((s) => s.togglingActive);
  const nextRun = useWorkspace((s) => s.triggers.find((t) => t.nextRun > 0)?.nextRun);
  const { renameWorkflow, importWorkflow, exportWorkflow, setActive } = useWorkspace.getState();
  const askText = useUI((s) => s.askText);

  const runningHere = !!activeRunId;

  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b border-line bg-panel px-3">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        {workflowId ? (
          <>
            <button
              className="min-w-0 truncate rounded-md px-1.5 py-1 text-left text-sm font-semibold transition-colors duration-150 hover:bg-elevated"
              title="Переименовать"
              onClick={async () => {
                const v = await askText({
                  title: 'Переименовать Workflow',
                  initial: name,
                  confirmLabel: 'Сохранить',
                });
                if (v) await renameWorkflow(workflowId, v);
              }}
            >
              {name}
            </button>
            <SaveIndicator />
          </>
        ) : (
          <span className="px-1.5 text-sm text-faint">Нет открытого Workflow</span>
        )}
      </div>

      <div className="flex items-center gap-0.5">
        <IconButton label="Отменить (Ctrl+Z)" onClick={undo} disabled={!workflowId || !canUndo}>
          <Undo2 size={16} />
        </IconButton>
        <IconButton label="Повторить (Ctrl+Shift+Z)" onClick={redo} disabled={!workflowId || !canRedo}>
          <Redo2 size={16} />
        </IconButton>
        <IconButton label="Сохранить (Ctrl+S)" onClick={() => void save()} disabled={!workflowId}>
          <Save size={16} />
        </IconButton>
        <div className="mx-1.5 h-5 w-px bg-line" />
        <Button
          variant="ghost"
          size="sm"
          icon={<Download size={14} />}
          title="Импорт Workflow из JSON в текущий проект"
          onClick={() => activeProjectId && void importWorkflow(activeProjectId)}
          disabled={!activeProjectId}
        >
          Import
        </Button>
        <Button
          variant="ghost"
          size="sm"
          icon={<Upload size={14} />}
          title="Экспорт Workflow в JSON (без секретов)"
          onClick={() => void exportWorkflow()}
          disabled={!workflowId}
        >
          Export
        </Button>
        <div className="mx-1.5 h-5 w-px bg-line" />
      </div>

      {errorCount > 0 && !runningHere && (
        <span className="flex items-center gap-1 text-xs text-error" title="Исправьте ошибки в Inspector">
          <AlertTriangle size={13} /> {errorCount}
        </span>
      )}
      {otherRuns > 0 && (
        <span className="text-xs whitespace-nowrap text-muted" title="Другие Workflow выполняются в фоне">
          фоновых запусков: {otherRuns}
        </span>
      )}
      {workflowId && (
        <label
          className={cx(
            'flex h-8 shrink-0 cursor-pointer items-center gap-2 rounded-md border px-2.5 text-xs transition-colors duration-150',
            automated
              ? 'border-success/40 bg-success/10 text-success'
              : 'border-line text-muted hover:text-fg',
          )}
          title={
            automated
              ? nextRun
                ? `Ближайший запуск: ${new Date(nextRun).toLocaleString('ru-RU')}`
                : 'Триггеры ожидают событий'
              : 'Включить автоматические триггеры (Cron, Interval, Schedule, File Watcher, Startup)'
          }
        >
          <Zap size={13} className={automated ? 'fill-current' : ''} />
          Авто
          <Toggle
            checked={automated}
            onChange={(v) => !toggling && void setActive(workflowId, v)}
            label="Автоматизация"
          />
        </label>
      )}
      <Button
        variant="success"
        icon={
          starting ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} fill="currentColor" />
        }
        onClick={() => void useRun.getState().start()}
        disabled={!workflowId || runningHere || starting}
        title="Запустить (Ctrl+Enter). Если выделен триггер — запуск от него"
      >
        Run
      </Button>
      <Button
        variant="danger"
        icon={<Square size={12} fill="currentColor" />}
        onClick={() => void useRun.getState().stop()}
        disabled={!runningHere}
        title="Остановить выполнение этого Workflow"
      >
        Stop
      </Button>
    </header>
  );
}
