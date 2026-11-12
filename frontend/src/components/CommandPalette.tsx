import { useEffect, useMemo, useRef, useState } from 'react';
import { Search, Workflow as WorkflowIcon } from 'lucide-react';
import { canvasBridge } from '../lib/canvasBridge';
import { useEditor } from '../stores/editor';
import { useUI } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import { categoryColor, NodeIcon } from './icons';
import { cx, Kbd } from './ui';

interface Result {
  id: string;
  kind: 'node' | 'workflow';
  title: string;
  subtitle: string;
  icon?: string;
  color?: string;
  run: () => void;
}

/** Flattens params to searchable text ("url https://… headers Authorization …"). */
function paramText(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'object')
    return Object.values(v as Record<string, unknown>)
      .map(paramText)
      .join(' ');
  return String(v);
}

/** Ctrl+K: search nodes (name, type, variable, parameter values) and all workflows. */
export function CommandPalette() {
  const open = useUI((s) => s.paletteOpen);
  const close = () => useUI.getState().setPaletteOpen(false);
  const [query, setQuery] = useState('');
  const [active, setActive] = useState(0);
  const [wasOpen, setWasOpen] = useState(open);
  const inputRef = useRef<HTMLInputElement>(null);
  const nodes = useEditor((s) => s.nodes);
  const defByType = useWorkspace((s) => s.defByType);
  const projects = useWorkspace((s) => s.projects);
  const workflows = useWorkspace((s) => s.workflows);
  const currentId = useEditor((s) => s.workflowId);

  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) {
      setQuery('');
      setActive(0);
    }
  }

  useEffect(() => {
    if (open) window.setTimeout(() => inputRef.current?.focus(), 20);
  }, [open]);

  const results = useMemo<Result[]>(() => {
    const q = query.trim().toLowerCase();
    const match = (...parts: string[]) => !q || parts.some((p) => p.toLowerCase().includes(q));
    const nodeResults: Result[] = nodes
      .filter((n) => {
        const def = defByType[n.data.nodeType];
        return match(n.data.name, n.data.key, def?.name ?? '', n.data.nodeType, paramText(n.data.params));
      })
      .map((n) => {
        const def = defByType[n.data.nodeType];
        return {
          id: `n:${n.id}`,
          kind: 'node',
          title: n.data.name,
          subtitle: `${def?.name ?? n.data.nodeType} · {{${n.data.key}}}`,
          icon: def?.icon,
          color: categoryColor[def?.category ?? ''],
          run: () => canvasBridge.focusNode(n.id),
        };
      });
    const projectName = Object.fromEntries(projects.map((p) => [p.id, p.name]));
    const wfResults: Result[] = Object.values(workflows)
      .flat()
      .filter((w) => w.id !== currentId && match(w.name, w.description, projectName[w.projectId] ?? ''))
      .map((w) => ({
        id: `w:${w.id}`,
        kind: 'workflow',
        title: w.name,
        subtitle: projectName[w.projectId] ?? '',
        run: () => void useWorkspace.getState().openWorkflow(w.id),
      }));
    return [...nodeResults, ...wfResults].slice(0, 50);
  }, [query, nodes, defByType, projects, workflows, currentId]);

  if (!open) return null;

  const choose = (r: Result | undefined) => {
    if (!r) return;
    close();
    r.run();
  };

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in justify-center bg-black/45 pt-[12vh]"
      onMouseDown={(e) => e.target === e.currentTarget && close()}
    >
      <div className="h-fit w-130 max-w-[92vw] animate-slide-up overflow-hidden rounded-xl border border-line-strong bg-panel shadow-2xl shadow-black/60">
        <div className="flex items-center gap-2.5 border-b border-line px-3.5">
          <Search size={16} className="text-faint" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setActive(0);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Escape') close();
              else if (e.key === 'ArrowDown') {
                e.preventDefault();
                setActive((a) => Math.min(a + 1, results.length - 1));
              } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                setActive((a) => Math.max(a - 1, 0));
              } else if (e.key === 'Enter') choose(results[active]);
            }}
            placeholder="Найти Node, параметр или Workflow…"
            className="h-12 flex-1 bg-transparent text-sm text-fg placeholder:text-faint focus:outline-none"
            aria-label="Поиск"
          />
          <Kbd>Esc</Kbd>
        </div>
        <ul className="max-h-[50vh] overflow-y-auto p-1.5" role="listbox">
          {results.map((r, i) => (
            <li key={r.id} role="option" aria-selected={i === active}>
              <button
                onMouseEnter={() => setActive(i)}
                onClick={() => choose(r)}
                className={cx(
                  'flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left',
                  i === active ? 'bg-elevated' : '',
                )}
              >
                <span
                  className="flex size-7 shrink-0 items-center justify-center rounded-md bg-panel-2"
                  style={r.color ? { color: r.color, background: `${r.color}1f` } : undefined}
                >
                  {r.kind === 'node' ? (
                    <NodeIcon name={r.icon ?? ''} size={14} />
                  ) : (
                    <WorkflowIcon size={14} className="text-accent" />
                  )}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] text-fg">{r.title}</span>
                  <span className="block truncate font-mono text-[11px] text-faint">{r.subtitle}</span>
                </span>
                <span className="shrink-0 text-[10px] text-faint uppercase">
                  {r.kind === 'node' ? 'Node' : 'Workflow'}
                </span>
              </button>
            </li>
          ))}
          {results.length === 0 && (
            <li className="px-3 py-6 text-center text-xs text-faint">Ничего не найдено</li>
          )}
        </ul>
      </div>
    </div>
  );
}
