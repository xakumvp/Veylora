import { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, ChevronUp, History, ScrollText, Trash2 } from 'lucide-react';
import { cx, IconButton } from '../components/ui';
import { canvasBridge } from '../lib/canvasBridge';
import { levelMeta, statusMeta } from '../nodes/status';
import { formatDuration, useRun } from '../stores/run';
import { useUI } from '../stores/ui';
import type { LogEntry, NodeStatus } from '../types';

const LEVELS = ['DEBUG', 'INFO', 'WARNING', 'ERROR'] as const;

const time = (ts: number) =>
  new Date(ts).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) +
  '.' +
  String(ts % 1000).padStart(3, '0');

const dateTime = (ts: number) =>
  new Date(ts).toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });

function StatusChip({ status }: { status: NodeStatus }) {
  const m = statusMeta[status] ?? statusMeta.IDLE;
  return (
    <span
      className={cx(
        'inline-flex items-center gap-1.5 rounded-full border border-line bg-bg px-2 py-0.5 text-[11px] font-medium',
        m.text,
      )}
    >
      <span className={cx('size-1.5 rounded-full', m.color, status === 'RUNNING' && 'animate-pulse')} />
      {m.label}
    </span>
  );
}

function LogRow({ l }: { l: LogEntry }) {
  const [open, setOpen] = useState(false);
  const hasData = l.data !== undefined && l.data !== null;
  return (
    <div className="group border-b border-line/50 px-3 py-1 font-mono text-[11.5px] leading-relaxed hover:bg-panel-2/60">
      <div className="flex items-start gap-3">
        <span className="shrink-0 text-faint">{time(l.timestamp)}</span>
        <span className={cx('w-14 shrink-0 font-semibold', levelMeta[l.level] ?? 'text-muted')}>
          {l.level}
        </span>
        {l.nodeName ? (
          <button
            className="max-w-40 shrink-0 truncate text-muted hover:text-accent"
            title="Показать Node на холсте"
            onClick={() => l.nodeId && canvasBridge.focusNode(l.nodeId)}
          >
            {l.nodeName}
          </button>
        ) : (
          <span className="w-0 shrink-0" />
        )}
        <span className="selectable min-w-0 flex-1 wrap-break-word whitespace-pre-wrap text-fg">
          {l.message}
        </span>
        {hasData && (
          <button className="shrink-0 text-faint hover:text-fg" onClick={() => setOpen(!open)}>
            {open ? 'скрыть' : 'данные'}
          </button>
        )}
      </div>
      {open && hasData && (
        <pre className="selectable mt-1 mb-1 ml-50 overflow-x-auto rounded border border-line bg-bg p-2 text-muted">
          {JSON.stringify(l.data, null, 2)}
        </pre>
      )}
    </div>
  );
}

export function ExecutionPanel() {
  const open = useUI((s) => s.executionPanelOpen);
  const setOpen = useUI((s) => s.setExecutionPanelOpen);
  const askConfirm = useUI((s) => s.askConfirm);
  const status = useRun((s) => s.status);
  const startedAt = useRun((s) => s.startedAt);
  const durationMs = useRun((s) => s.durationMs);
  const error = useRun((s) => s.error);
  const logs = useRun((s) => s.logs);
  const executions = useRun((s) => s.executions);
  const shownRunId = useRun((s) => s.shownRunId);
  const activeRunId = useRun((s) => s.activeRunId);
  const [tab, setTab] = useState<'logs' | 'history'>('logs');
  const [levels, setLevels] = useState<Record<string, boolean>>({
    DEBUG: false,
    INFO: true,
    WARNING: true,
    ERROR: true,
  });
  const [height, setHeight] = useState(240);
  const [now, setNow] = useState(() => Date.now());
  const scrollRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);

  const running = !!activeRunId && activeRunId === shownRunId;

  useEffect(() => {
    if (!running) return;
    const t = window.setInterval(() => setNow(Date.now()), 100);
    return () => window.clearInterval(t);
  }, [running]);

  const visible = useMemo(() => logs.filter((l) => levels[l.level] ?? true), [logs, levels]);

  useEffect(() => {
    const el = scrollRef.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [visible.length, tab]);

  const startResize = (e: React.MouseEvent) => {
    e.preventDefault();
    const startY = e.clientY;
    const startH = height;
    const move = (ev: MouseEvent) =>
      setHeight(Math.min(Math.max(startH + startY - ev.clientY, 120), window.innerHeight * 0.7));
    const up = () => {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  };

  const elapsed = running && startedAt ? now - startedAt : durationMs;

  return (
    <section
      className="relative flex shrink-0 flex-col border-t border-line bg-panel"
      style={{ height: open ? height : 40 }}
    >
      {open && (
        <div
          onMouseDown={startResize}
          className="absolute -top-1 right-0 left-0 z-10 h-2 cursor-row-resize"
          aria-hidden="true"
        />
      )}
      <div className="flex h-10 shrink-0 items-center gap-3 border-b border-line px-3">
        <div className="flex shrink-0 gap-1">
          {(
            [
              ['logs', 'Логи', ScrollText],
              ['history', 'История', History],
            ] as const
          ).map(([id, label, Icon]) => (
            <button
              key={id}
              onClick={() => {
                setTab(id);
                setOpen(true);
              }}
              className={cx(
                'flex h-7 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition-colors duration-150',
                tab === id && open ? 'bg-elevated text-fg' : 'text-muted hover:text-fg',
              )}
            >
              <Icon size={13} /> {label}
              {id === 'history' && executions.length > 0 && (
                <span className="text-faint">{executions.length}</span>
              )}
            </button>
          ))}
        </div>
        <div className="h-4 w-px shrink-0 bg-line" />
        <span
          className="flex shrink-0 items-center gap-2 whitespace-nowrap"
          title={startedAt && status !== 'IDLE' ? `Запуск: ${dateTime(startedAt)}` : undefined}
        >
          <StatusChip status={status} />
          {elapsed !== undefined && status !== 'IDLE' && (
            <span className="font-mono text-xs text-muted">{formatDuration(elapsed)}</span>
          )}
        </span>
        {error && (
          <span className="min-w-0 truncate text-xs text-error" title={error}>
            {error}
          </span>
        )}

        <div className="ml-auto flex shrink-0 items-center gap-1">
          {open && tab === 'logs' && (
            <div className="mr-2 flex items-center gap-0.5">
              {LEVELS.map((lv) => (
                <button
                  key={lv}
                  onClick={() => setLevels((s) => ({ ...s, [lv]: !s[lv] }))}
                  className={cx(
                    'rounded px-1.5 py-0.5 font-mono text-[10px] font-semibold transition-opacity duration-150',
                    levelMeta[lv],
                    levels[lv] ? 'bg-elevated opacity-100' : 'opacity-35 hover:opacity-70',
                  )}
                  title={`Показывать ${lv}`}
                >
                  {lv}
                </button>
              ))}
            </div>
          )}
          <IconButton
            label="Очистить логи и историю"
            onClick={async () => {
              const ok = await askConfirm({
                title: 'Очистить логи?',
                message: 'Будут удалены все логи и история запусков этого Workflow.',
                confirmLabel: 'Очистить',
                danger: true,
              });
              if (ok) await useRun.getState().clearHistory();
            }}
            disabled={!logs.length && !executions.length}
          >
            <Trash2 size={15} />
          </IconButton>
          <IconButton label={open ? 'Свернуть панель' : 'Развернуть панель'} onClick={() => setOpen(!open)}>
            {open ? <ChevronDown size={16} /> : <ChevronUp size={16} />}
          </IconButton>
        </div>
      </div>

      {open && tab === 'logs' && (
        <div
          ref={scrollRef}
          onScroll={(e) => {
            const el = e.currentTarget;
            stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
          }}
          className="min-h-0 flex-1 overflow-y-auto"
        >
          {visible.map((l, i) => (
            <LogRow key={l.id ?? `${l.timestamp}-${i}`} l={l} />
          ))}
          {visible.length === 0 && (
            <div className="flex h-full items-center justify-center text-xs text-faint">
              {logs.length
                ? 'Все сообщения скрыты фильтром'
                : 'Нажмите Run, чтобы запустить Workflow — здесь появятся логи выполнения'}
            </div>
          )}
        </div>
      )}

      {open && tab === 'history' && (
        <div className="min-h-0 flex-1 overflow-y-auto">
          {executions.map((ex) => (
            <button
              key={ex.id}
              onClick={() => {
                void useRun.getState().showExecution(ex);
                setTab('logs');
              }}
              className={cx(
                'flex w-full items-center gap-4 border-b border-line/50 px-3 py-1.5 text-left text-xs transition-colors duration-150 hover:bg-panel-2/60',
                ex.id === shownRunId && 'bg-accent-soft',
              )}
            >
              <span className="w-28 shrink-0">
                <StatusChip status={ex.id === activeRunId ? 'RUNNING' : ex.status} />
              </span>
              <span className="w-32 shrink-0 text-muted">{dateTime(ex.startedAt)}</span>
              <span className="w-20 shrink-0 font-mono text-muted">{formatDuration(ex.durationMs)}</span>
              <span className="min-w-0 flex-1 truncate text-error">{ex.error}</span>
              <span className="shrink-0 font-mono text-[10px] text-faint">{ex.id.slice(0, 8)}</span>
            </button>
          ))}
          {executions.length === 0 && (
            <div className="flex h-full items-center justify-center text-xs text-faint">
              Запусков пока не было
            </div>
          )}
        </div>
      )}
    </section>
  );
}
