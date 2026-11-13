import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { AlertCircle, Check, CircleSlash, Loader2, X } from 'lucide-react';
import { categoryColor, NodeIcon } from '../components/icons';
import { cx } from '../components/ui';
import { str, type FlowNode } from '../lib/graph';
import { useEditor } from '../stores/editor';
import { useRun, formatDuration } from '../stores/run';
import { useWorkspace } from '../stores/workspace';
import type { Params } from '../types';

const opLabels: Record<string, string> = {
  equals: '=',
  not_equals: '≠',
  greater_than: '>',
  less_than: '<',
  contains: 'contains',
  is_empty: 'is empty',
};

const unitLabels: Record<string, string> = { ms: 'мс', s: 'с', min: 'мин' };
const intervalUnits: Record<string, string> = { s: 'сек', min: 'мин', h: 'ч' };

function Summary({ type, params }: { type: string; params: Params }) {
  switch (type) {
    case 'trigger.manual':
      return <span>{str(params, 'runName') || 'Ручной запуск'}</span>;
    case 'http.request': {
      const url = str(params, 'url');
      return (
        <span className="flex min-w-0 items-center gap-1.5">
          <span className="rounded bg-info/15 px-1 font-semibold text-info">
            {str(params, 'method', 'GET')}
          </span>
          <span className="truncate">{url || 'URL не задан'}</span>
        </span>
      );
    }
    case 'logic.condition': {
      const op = str(params, 'operator', 'equals');
      return (
        <span className="truncate">
          {str(params, 'variable') || '…'} <span className="text-warning">{opLabels[op] ?? op}</span>{' '}
          {op === 'is_empty' ? '' : str(params, 'value') || '…'}
        </span>
      );
    }
    case 'utility.delay':
      return (
        <span>
          {str(params, 'duration', '0')} {unitLabels[str(params, 'unit', 's')] ?? ''}
        </span>
      );
    case 'trigger.cron':
      return (
        <span className="truncate">
          {str(params, 'expression') || '…'} <span className="text-faint">{str(params, 'timezone')}</span>
        </span>
      );
    case 'trigger.interval':
      return (
        <span>
          каждые {str(params, 'every', '?')} {intervalUnits[str(params, 'unit', 'min')] ?? ''}
        </span>
      );
    case 'trigger.schedule':
      return <span className="truncate">{str(params, 'at').replace('T', ' ') || 'дата не задана'}</span>;
    case 'trigger.file':
      return (
        <span className="truncate">
          {str(params, 'pattern', '*')} в {str(params, 'directory') || 'каталог не задан'}
        </span>
      );
    case 'trigger.startup':
      return <span>при запуске Veylora</span>;
    case 'logic.stop':
      return (
        <span className="truncate">
          <span className={str(params, 'status') === 'failed' ? 'text-error' : 'text-success'}>
            {str(params, 'status') === 'failed' ? 'ошибка' : 'успех'}
          </span>{' '}
          {str(params, 'message')}
        </span>
      );
    case 'logic.subflow':
      return <SubflowSummary id={str(params, 'workflowId')} />;
    case 'logging.logger':
      return (
        <span className="truncate">
          <span className="text-muted">[{str(params, 'level', 'INFO')}]</span> {str(params, 'message') || '…'}
        </span>
      );
    default:
      return null;
  }
}

function SubflowSummary({ id }: { id: string }) {
  const name = useWorkspace(
    (s) =>
      Object.values(s.workflows)
        .flat()
        .find((w) => w.id === id)?.name,
  );
  return <span className="truncate">{id ? (name ?? 'Workflow удалён') : 'Workflow не выбран'}</span>;
}

function StatusBadge({ status, durationMs }: { status: string; durationMs?: number }) {
  switch (status) {
    case 'RUNNING':
      return <Loader2 size={14} className="animate-spin text-accent" aria-label="Выполняется" />;
    case 'SUCCESS':
      return (
        <span className="flex items-center gap-1 text-[10px] text-success">
          <Check size={13} strokeWidth={3} />
          {formatDuration(durationMs)}
        </span>
      );
    case 'FAILED':
      return <X size={14} strokeWidth={3} className="text-error" aria-label="Ошибка" />;
    case 'TIMEOUT':
      return <X size={14} strokeWidth={3} className="text-error" aria-label="Таймаут" />;
    case 'CANCELLED':
      return <CircleSlash size={13} className="text-warning" aria-label="Отменено" />;
    case 'WAITING':
      return <span className="size-2 rounded-full bg-muted/60" aria-label="Ожидание" />;
    default:
      return null;
  }
}

function VeyloraNodeImpl({ id, data, selected }: NodeProps<FlowNode>) {
  const def = useWorkspace((s) => s.defByType[data.nodeType]);
  const showRun = useRun((s) => s.runWorkflowId === useEditor.getState().workflowId);
  const run = useRun((s) => s.nodes[id]);
  const hasError = useRun((s) => s.issues.some((i) => i.nodeId === id && i.severity === 'error'));
  const info = showRun ? run : undefined;
  const status = info?.status ?? 'IDLE';
  const color = categoryColor[def?.category ?? ''] ?? '#94a3b8';
  const regular = (def?.outputs ?? []).filter((p) => p.role !== 'error');
  const errorPort = def?.outputs.find((p) => p.role === 'error');
  const errorEdge = useEditor((s) => s.edges.some((e) => e.source === id && e.sourceHandle === 'error'));
  const showErrorPort = !!errorPort && (data.settings?.onError === 'error_output' || errorEdge);
  const multi = regular.length > 1;
  const nextRun = useWorkspace((s) =>
    s.isActive(s.activeWorkflowId) ? s.triggers.find((t) => t.nodeId === id) : undefined,
  );
  const retries = (data.settings?.retry?.maxAttempts ?? 1) > 1;

  return (
    <div
      className={cx(
        'group relative w-60 rounded-[10px] border bg-panel-2 shadow-lg shadow-black/30 transition-[border-color,opacity,box-shadow] duration-200',
        selected ? 'border-accent ring-1 ring-accent/40' : 'border-line hover:border-line-strong',
        status === 'RUNNING' && 'animate-pulse-ring border-accent',
        status === 'SUCCESS' && !selected && 'border-success/50',
        status === 'FAILED' && !info?.handled && 'border-error ring-1 ring-error/30',
        status === 'FAILED' && info?.handled && 'border-warning/70',
        status === 'TIMEOUT' && 'border-error',
        status === 'CANCELLED' && !selected && 'border-warning/60',
        status === 'SKIPPED' && 'opacity-45',
      )}
    >
      {def?.inputs.map((p) => (
        <Handle key={p.id} id={p.id} type="target" position={Position.Left} className="!top-6.5" />
      ))}

      <div className="flex items-center gap-2.5 px-3 pt-2.5 pb-2">
        <div
          className="flex size-7 shrink-0 items-center justify-center rounded-md"
          style={{ background: `${color}22`, color }}
        >
          <NodeIcon name={def?.icon ?? ''} size={15} strokeWidth={2.2} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13px] leading-tight font-semibold text-fg">{data.name}</div>
          <div className="truncate font-mono text-[10px] leading-tight text-faint">
            {def?.name ?? data.nodeType} · {data.key}
          </div>
        </div>
        {hasError && status === 'IDLE' ? (
          <AlertCircle size={14} className="shrink-0 text-error" aria-label="Есть ошибки настройки" />
        ) : (
          <StatusBadge status={status} durationMs={info?.durationMs} />
        )}
      </div>

      <div className="mx-3 mb-2.5 truncate rounded-md bg-bg/70 px-2 py-1.5 font-mono text-[11px] text-muted">
        <Summary type={data.nodeType} params={data.params} />
      </div>

      {info?.error && status === 'FAILED' && (
        <div
          className={cx(
            'mx-3 -mt-1 mb-2.5 line-clamp-2 text-[11px] leading-snug',
            info.handled ? 'text-warning' : 'text-error',
          )}
        >
          {info.handled ? 'Обработано: ' : ''}
          {info.error}
        </div>
      )}

      {(nextRun || retries || (info?.attempts ?? 0) > 1) && (
        <div className="mx-3 -mt-1 mb-2.5 flex flex-wrap gap-x-3 text-[10px] text-faint">
          {nextRun && (
            <span className="text-success" title={nextRun.detail}>
              {nextRun.nextRun > 0
                ? `▶ ${new Date(nextRun.nextRun).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })}`
                : `● ${nextRun.detail}`}
            </span>
          )}
          {retries && <span>повторов: до {data.settings!.retry.maxAttempts}</span>}
          {(info?.attempts ?? 0) > 1 && <span>попыток: {info!.attempts}</span>}
        </div>
      )}

      {multi ? (
        <div className="flex flex-col gap-1 border-t border-line py-1.5">
          {regular.map((p) => {
            const taken = info?.status === 'SUCCESS' && info.branch === p.id;
            return (
              <div key={p.id} className="relative flex h-5 items-center justify-end pr-3.5">
                <span
                  className={cx(
                    'text-[10px] font-semibold tracking-wide',
                    taken ? (p.id === 'true' ? 'text-success' : 'text-error') : 'text-faint',
                  )}
                >
                  {p.label || p.id}
                </span>
                <Handle id={p.id} type="source" position={Position.Right} className="!top-1/2" />
              </div>
            );
          })}
        </div>
      ) : (
        regular.map((p) => (
          <Handle key={p.id} id={p.id} type="source" position={Position.Right} className="!top-6.5" />
        ))
      )}

      {showErrorPort && (
        <div className="relative flex h-6 items-center justify-end border-t border-error/25 pr-3.5">
          <span
            className={cx(
              'text-[10px] font-semibold tracking-wide',
              info?.branch === 'error' ? 'text-error' : 'text-error/60',
            )}
          >
            ERROR
          </span>
          <Handle id="error" type="source" position={Position.Right} className="!top-1/2 !border-error/70" />
        </div>
      )}
    </div>
  );
}

export const VeyloraNode = memo(VeyloraNodeImpl);
