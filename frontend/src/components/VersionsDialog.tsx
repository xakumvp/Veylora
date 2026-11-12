import { useCallback, useEffect, useState } from 'react';
import { Download, History, Loader2, Minus, Pencil, Plus, RotateCcw, Save } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { useEditor } from '../stores/editor';
import { useRun } from '../stores/run';
import { toast, useUI } from '../stores/ui';
import type { DiffResult, VersionMeta } from '../types';
import { Button, cx, Modal } from './ui';

const sourceLabel: Record<VersionMeta['source'], string> = {
  manual: 'Версия',
  auto: 'Автосохранение',
  restore: 'До восстановления',
  import: 'Импорт',
};

const fieldLabel = (f: string) =>
  f === 'name'
    ? 'название'
    : f === 'key'
      ? 'имя переменной'
      : f === 'type'
        ? 'тип'
        : f === 'settings'
          ? 'настройки'
          : f === 'position'
            ? 'положение'
            : f.replace(/^params\./, '');

function DiffView({ diff }: { diff: DiffResult }) {
  if (diff.identical) return <p className="text-xs text-success">Совпадает с текущим состоянием Workflow.</p>;
  return (
    <div className="flex flex-col gap-2 text-xs">
      <p className="text-[11px] text-faint">Что изменилось с этой версии до текущего состояния:</p>
      {diff.addedNodes.map((n) => (
        <div key={n.id} className="flex items-center gap-2 text-success">
          <Plus size={12} /> добавлен «{n.name}»
        </div>
      ))}
      {diff.removedNodes.map((n) => (
        <div key={n.id} className="flex items-center gap-2 text-error">
          <Minus size={12} /> удалён «{n.name}»
        </div>
      ))}
      {diff.changedNodes.map((n) => (
        <div key={n.id} className="flex items-start gap-2 text-warning">
          <Pencil size={12} className="mt-0.5 shrink-0" />
          <span>
            изменён «{n.name}»: <span className="text-muted">{n.fields.map(fieldLabel).join(', ')}</span>
          </span>
        </div>
      ))}
      {(diff.addedEdges > 0 || diff.removedEdges > 0) && (
        <div className="text-muted">
          Connections: +{diff.addedEdges} / −{diff.removedEdges}
        </div>
      )}
      {diff.variablesChanged && <div className="text-muted">Изменены переменные Workflow</div>}
      {diff.settingsChanged && <div className="text-muted">Изменены настройки Workflow</div>}
    </div>
  );
}

export function VersionsDialog() {
  const open = useUI((s) => s.versionsOpen);
  const setOpen = useUI((s) => s.setVersionsOpen);
  const workflowId = useEditor((s) => s.workflowId);
  const [versions, setVersions] = useState<VersionMeta[]>([]);
  const [selected, setSelected] = useState<VersionMeta | null>(null);
  const [diffFor, setDiffFor] = useState<{ id: number; diff: DiffResult } | null>(null);
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) setSelected(null);
  }
  const diff = selected && diffFor?.id === selected.id ? diffFor.diff : null;
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    if (!workflowId) return;
    try {
      const list = await api.listVersions(workflowId);
      setVersions(list ?? []);
    } catch (err) {
      toast('error', 'Версии не загружены', errorMessage(err));
    }
  }, [workflowId]);

  useEffect(() => {
    if (!open || !workflowId) return;
    void useEditor.getState().save().then(load);
  }, [open, workflowId, load]);

  useEffect(() => {
    if (!selected || !workflowId) return;
    let cancelled = false;
    const id = selected.id;
    api
      .compareVersion(workflowId, id)
      .then((d) => !cancelled && setDiffFor({ id, diff: d }))
      .catch((err) => toast('error', 'Сравнение не удалось', errorMessage(err)));
    return () => {
      cancelled = true;
    };
  }, [selected, workflowId]);

  if (!open || !workflowId) return null;

  const create = async () => {
    const description = await useUI.getState().askText({
      title: 'Новая версия',
      label: 'Описание изменений',
      initial: '',
      confirmLabel: 'Сохранить версию',
    });
    if (description === null) return;
    try {
      await useEditor.getState().save();
      const v = await api.createVersion(workflowId, description);
      toast('success', `Версия ${v.number} сохранена`);
      await load();
    } catch (err) {
      toast('error', 'Версия не создана', errorMessage(err));
    }
  };

  const restore = async () => {
    if (!selected) return;
    const ok = await useUI.getState().askConfirm({
      title: `Восстановить версию ${selected.number}?`,
      message:
        'Текущее состояние будет автоматически сохранено как отдельная версия, так что восстановление можно отменить.',
      confirmLabel: 'Восстановить',
    });
    if (!ok) return;
    setBusy(true);
    try {
      const w = await api.restoreVersion(workflowId, selected.id);
      useEditor.getState().load(w);
      useRun.getState().resetForWorkflow(w.id);
      toast('success', `Версия ${selected.number} восстановлена`);
      setOpen(false);
    } catch (err) {
      toast('error', 'Не удалось восстановить', errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const exportVersion = async () => {
    if (!selected) return;
    try {
      const res = await api.exportVersion(workflowId, selected.id);
      if (res) toast('success', 'Версия экспортирована', res.path);
    } catch (err) {
      toast('error', 'Экспорт не выполнен', errorMessage(err));
    }
  };

  return (
    <Modal title="История версий" width="w-200" onClose={() => setOpen(false)}>
      <div className="flex h-105 gap-4">
        <div className="flex w-72 shrink-0 flex-col gap-2">
          <Button variant="primary" icon={<Save size={14} />} onClick={() => void create()}>
            Сохранить версию…
          </Button>
          <ul className="min-h-0 flex-1 overflow-y-auto rounded-lg border border-line">
            {versions.map((v) => (
              <li key={v.id}>
                <button
                  onClick={() => setSelected(v)}
                  className={cx(
                    'flex w-full flex-col gap-0.5 border-b border-line/60 px-3 py-2 text-left transition-colors duration-150',
                    selected?.id === v.id ? 'bg-accent-soft' : 'hover:bg-panel-2',
                  )}
                >
                  <span className="flex items-center gap-2 text-[13px]">
                    <span className="font-mono text-faint">v{v.number}</span>
                    <span className="truncate">{v.description || sourceLabel[v.source]}</span>
                  </span>
                  <span className="text-[11px] text-faint">
                    {new Date(v.createdAt).toLocaleString('ru-RU')} · {v.nodes} Nodes
                    {v.source !== 'manual' && ` · ${sourceLabel[v.source].toLowerCase()}`}
                  </span>
                </button>
              </li>
            ))}
            {versions.length === 0 && (
              <li className="px-3 py-6 text-center text-xs leading-relaxed text-faint">
                Версий пока нет. Они создаются автоматически при сохранении (не чаще раза в 10 минут) или
                вручную.
              </li>
            )}
          </ul>
        </div>
        <div className="flex min-w-0 flex-1 flex-col rounded-lg border border-line bg-bg p-4">
          {selected ? (
            <>
              <div className="mb-3">
                <div className="text-sm font-semibold">
                  Версия {selected.number}
                  {selected.description && ` — ${selected.description}`}
                </div>
                <div className="text-xs text-faint">
                  {new Date(selected.createdAt).toLocaleString('ru-RU')} · {selected.nodes} Nodes ·{' '}
                  {selected.edges} Connections
                </div>
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto">
                {diff ? <DiffView diff={diff} /> : <Loader2 size={16} className="animate-spin text-faint" />}
              </div>
              <div className="mt-3 flex justify-end gap-2 border-t border-line pt-3">
                <Button icon={<Download size={14} />} onClick={() => void exportVersion()}>
                  Экспорт
                </Button>
                <Button
                  variant="primary"
                  icon={<RotateCcw size={14} />}
                  disabled={busy || diff?.identical}
                  onClick={() => void restore()}
                >
                  Восстановить
                </Button>
              </div>
            </>
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-xs text-faint">
              <History size={22} />
              Выберите версию, чтобы увидеть изменения
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
}
