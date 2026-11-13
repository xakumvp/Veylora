import { useEffect, useState } from 'react';
import { FileText, LayoutTemplate } from 'lucide-react';
import { api, errorMessage } from '../services/api';
import { toast, useUI } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Template } from '../types';
import { Button, cx, Input, Modal } from './ui';

const BLANK = '';

/** "New Workflow": name + optional template. */
export function NewWorkflowDialog() {
  const projectId = useUI((s) => s.newWorkflowFor);
  const close = () => useUI.getState().setNewWorkflowFor(null);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [choice, setChoice] = useState(BLANK);
  const [name, setName] = useState('Новый Workflow');
  const [busy, setBusy] = useState(false);
  const [openFor, setOpenFor] = useState(projectId);
  if (openFor !== projectId) {
    setOpenFor(projectId);
    if (projectId) {
      setChoice(BLANK);
      setName('Новый Workflow');
    }
  }

  useEffect(() => {
    if (!projectId) return;
    api
      .templates()
      .then((t) => setTemplates(t ?? []))
      .catch(() => setTemplates([]));
  }, [projectId]);

  if (!projectId) return null;

  const create = async () => {
    setBusy(true);
    try {
      const ws = useWorkspace.getState();
      if (choice === BLANK) {
        await ws.createWorkflow(projectId, name.trim() || 'Новый Workflow');
      } else {
        const tplName = templates.find((t) => t.id === choice)?.name ?? '';
        const finalName = name.trim() && name !== 'Новый Workflow' ? name.trim() : tplName;
        const w = await api.createFromTemplate(projectId, choice, finalName);
        await ws.refreshWorkflows(projectId);
        await ws.openWorkflow(w.id);
        toast('success', 'Workflow создан из шаблона', w.name);
      }
      close();
    } catch (err) {
      toast('error', 'Workflow не создан', errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const cards: { id: string; title: string; description: string; meta: string }[] = [
    {
      id: BLANK,
      title: 'Пустой Workflow',
      description: 'Только Manual Trigger — соберите сценарий с нуля.',
      meta: '1 Node',
    },
    ...templates.map((t) => ({
      id: t.id,
      title: t.name,
      description: t.description,
      meta: `${t.nodes} Nodes${t.triggers.length ? ' · ' + t.triggers.join(', ') : ''}`,
    })),
  ];

  return (
    <Modal
      title="Новый Workflow"
      width="w-160"
      onClose={close}
      footer={
        <>
          <Button variant="ghost" onClick={close}>
            Отмена
          </Button>
          <Button variant="primary" disabled={busy} onClick={() => void create()}>
            Создать
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <label className="flex flex-col gap-1.5">
          <span className="text-xs font-medium text-muted">Название</span>
          <Input
            value={name}
            maxLength={120}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && void create()}
          />
        </label>
        <div>
          <div className="mb-2 text-xs font-medium text-muted">Шаблон</div>
          <div className="grid max-h-80 grid-cols-2 gap-2 overflow-y-auto">
            {cards.map((c) => (
              <button
                key={c.id || 'blank'}
                onClick={() => setChoice(c.id)}
                className={cx(
                  'flex flex-col gap-1 rounded-lg border p-3 text-left transition-colors duration-150',
                  choice === c.id
                    ? 'border-accent bg-accent-soft'
                    : 'border-line hover:border-line-strong hover:bg-panel-2',
                )}
              >
                <span className="flex items-center gap-2 text-[13px] font-medium">
                  {c.id === BLANK ? (
                    <FileText size={14} className="text-muted" />
                  ) : (
                    <LayoutTemplate size={14} className="text-accent" />
                  )}
                  {c.title}
                </span>
                <span className="line-clamp-2 text-[11px] leading-snug text-muted">{c.description}</span>
                <span className="text-[10px] text-faint">{c.meta}</span>
              </button>
            ))}
          </div>
        </div>
      </div>
    </Modal>
  );
}
