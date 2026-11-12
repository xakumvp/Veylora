import {
  ChevronRight,
  FilePlus2,
  FolderPlus,
  Info,
  Pencil,
  Settings,
  Trash2,
  Upload,
  Workflow as WorkflowIcon,
} from 'lucide-react';
import { Logo } from '../components/icons';
import { cx, IconButton, SectionTitle } from '../components/ui';
import { useRun } from '../stores/run';
import { useUI } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Project, WorkflowMeta } from '../types';

function WorkflowRow({ w }: { w: WorkflowMeta }) {
  const active = useWorkspace((s) => s.activeWorkflowId === w.id);
  const running = useRun((s) => !!s.activeRunId && s.runWorkflowId === w.id);
  const { openWorkflow, renameWorkflow, deleteWorkflow } = useWorkspace.getState();
  const { askText, askConfirm } = useUI.getState();

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => void openWorkflow(w.id)}
      onKeyDown={(e) => e.key === 'Enter' && void openWorkflow(w.id)}
      className={cx(
        'group flex h-7 cursor-pointer items-center gap-2 rounded-md pr-1 pl-7 text-[13px] transition-colors duration-150',
        active ? 'bg-accent-soft text-fg' : 'text-muted hover:bg-elevated hover:text-fg',
      )}
    >
      <WorkflowIcon size={13} className={cx('shrink-0', active ? 'text-accent' : 'text-faint')} />
      <span className="min-w-0 flex-1 truncate">{w.name}</span>
      {running && (
        <span className="size-1.5 shrink-0 animate-pulse rounded-full bg-accent" title="Выполняется" />
      )}
      <div className="hidden items-center group-hover:flex">
        <IconButton
          size="sm"
          label="Переименовать"
          onClick={async (e) => {
            e.stopPropagation();
            const name = await askText({
              title: 'Переименовать Workflow',
              initial: w.name,
              confirmLabel: 'Сохранить',
            });
            if (name) await renameWorkflow(w.id, name);
          }}
        >
          <Pencil size={12} />
        </IconButton>
        <IconButton
          size="sm"
          label="Удалить"
          className="hover:text-error"
          onClick={async (e) => {
            e.stopPropagation();
            const ok = await askConfirm({
              title: 'Удалить Workflow?',
              message: `«${w.name}» и вся история его запусков будут удалены без возможности восстановления.`,
              confirmLabel: 'Удалить',
              danger: true,
            });
            if (ok) await deleteWorkflow(w.id);
          }}
        >
          <Trash2 size={12} />
        </IconButton>
      </div>
    </div>
  );
}

function ProjectGroup({ p }: { p: Project }) {
  const expanded = useWorkspace((s) => !!s.expanded[p.id]);
  const workflows = useWorkspace((s) => s.workflows[p.id]);
  const active = useWorkspace((s) => s.activeProjectId === p.id);
  const { toggleProject, createWorkflow, renameProject, deleteProject, importWorkflow } =
    useWorkspace.getState();
  const { askText, askConfirm } = useUI.getState();

  return (
    <div>
      <div
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        onClick={() => toggleProject(p.id)}
        onKeyDown={(e) => e.key === 'Enter' && toggleProject(p.id)}
        className={cx(
          'group flex h-7 cursor-pointer items-center gap-1.5 rounded-md pr-1 pl-1.5 text-[13px] font-medium transition-colors duration-150 hover:bg-elevated',
          active ? 'text-fg' : 'text-muted',
        )}
      >
        <ChevronRight
          size={14}
          className={cx('shrink-0 text-faint transition-transform duration-150', expanded && 'rotate-90')}
        />
        <span className="min-w-0 flex-1 truncate">{p.name}</span>
        <div className="hidden items-center group-hover:flex">
          <IconButton
            size="sm"
            label="Новый Workflow"
            onClick={async (e) => {
              e.stopPropagation();
              const name = await askText({
                title: 'Новый Workflow',
                label: 'Название',
                initial: 'Новый Workflow',
                confirmLabel: 'Создать',
              });
              if (name) await createWorkflow(p.id, name);
            }}
          >
            <FilePlus2 size={12} />
          </IconButton>
          <IconButton
            size="sm"
            label="Импорт Workflow в проект"
            onClick={(e) => {
              e.stopPropagation();
              void importWorkflow(p.id);
            }}
          >
            <Upload size={12} />
          </IconButton>
          <IconButton
            size="sm"
            label="Переименовать проект"
            onClick={async (e) => {
              e.stopPropagation();
              const name = await askText({
                title: 'Переименовать проект',
                initial: p.name,
                confirmLabel: 'Сохранить',
              });
              if (name) await renameProject(p.id, name);
            }}
          >
            <Pencil size={12} />
          </IconButton>
          <IconButton
            size="sm"
            label="Удалить проект"
            className="hover:text-error"
            onClick={async (e) => {
              e.stopPropagation();
              const ok = await askConfirm({
                title: 'Удалить проект?',
                message: `Проект «${p.name}», все его Workflow (${workflows?.length ?? 0}) и история запусков будут удалены.`,
                confirmLabel: 'Удалить',
                danger: true,
              });
              if (ok) await deleteProject(p.id);
            }}
          >
            <Trash2 size={12} />
          </IconButton>
        </div>
      </div>
      {expanded && (
        <div className="mt-0.5 flex flex-col gap-0.5">
          {workflows?.map((w) => (
            <WorkflowRow key={w.id} w={w} />
          ))}
          {workflows?.length === 0 && <div className="py-1 pl-7 text-xs text-faint">Пока нет Workflow</div>}
        </div>
      )}
    </div>
  );
}

export function Sidebar() {
  const projects = useWorkspace((s) => s.projects);
  const activeProjectId = useWorkspace((s) => s.activeProjectId);
  const { createProject, createWorkflow } = useWorkspace.getState();
  const { askText, setSettingsOpen, setAboutOpen } = useUI.getState();

  const newWorkflow = async () => {
    const projectId = activeProjectId ?? projects[0]?.id;
    if (!projectId) {
      const name = await askText({
        title: 'Сначала создайте проект',
        label: 'Название проекта',
        initial: 'Мой проект',
        confirmLabel: 'Создать',
      });
      if (name) await createProject(name);
      return;
    }
    const name = await askText({
      title: 'Новый Workflow',
      label: 'Название',
      initial: 'Новый Workflow',
      confirmLabel: 'Создать',
    });
    if (name) await createWorkflow(projectId, name);
  };

  return (
    <aside className="flex h-full w-56 shrink-0 flex-col border-r border-line bg-panel">
      <div className="flex h-12 items-center gap-2.5 border-b border-line px-4">
        <Logo size={22} />
        <span className="text-[15px] font-semibold tracking-tight">Veylora</span>
        <span className="ml-auto rounded bg-elevated px-1.5 py-0.5 font-mono text-[10px] text-faint">
          MVP
        </span>
      </div>

      <div className="flex gap-1.5 px-3 pt-3">
        <button
          onClick={() => void newWorkflow()}
          className="flex h-8 flex-1 items-center justify-center gap-1.5 rounded-md bg-accent text-[13px] font-medium text-white transition-colors duration-150 hover:bg-accent-strong"
        >
          <FilePlus2 size={14} /> Workflow
        </button>
        <IconButton
          label="Новый проект"
          className="border border-line"
          onClick={async () => {
            const name = await askText({
              title: 'Новый проект',
              label: 'Название',
              initial: 'Новый проект',
              confirmLabel: 'Создать',
            });
            if (name) await createProject(name);
          }}
        >
          <FolderPlus size={15} />
        </IconButton>
      </div>

      <div className="mt-3 min-h-0 flex-1 overflow-y-auto px-2 pb-2">
        <div className="px-1.5">
          <SectionTitle>Проекты</SectionTitle>
        </div>
        <div className="flex flex-col gap-0.5">
          {projects.map((p) => (
            <ProjectGroup key={p.id} p={p} />
          ))}
          {projects.length === 0 && (
            <p className="px-2 py-2 text-xs leading-relaxed text-faint">
              Создайте первый проект кнопкой справа сверху.
            </p>
          )}
        </div>
      </div>

      <div className="flex flex-col gap-0.5 border-t border-line p-2">
        <button
          onClick={() => setSettingsOpen(true)}
          className="flex h-8 items-center gap-2 rounded-md px-2.5 text-[13px] text-muted transition-colors duration-150 hover:bg-elevated hover:text-fg"
        >
          <Settings size={15} /> Settings
        </button>
        <button
          onClick={() => setAboutOpen(true)}
          className="flex h-8 items-center gap-2 rounded-md px-2.5 text-[13px] text-muted transition-colors duration-150 hover:bg-elevated hover:text-fg"
        >
          <Info size={15} /> About
        </button>
      </div>
    </aside>
  );
}
