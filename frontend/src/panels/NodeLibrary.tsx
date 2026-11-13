import { useMemo, useState } from 'react';
import { Search } from 'lucide-react';
import { categoryColor, NodeIcon } from '../components/icons';
import { cx, Input, SectionTitle } from '../components/ui';
import { canvasBridge, DND_MIME } from '../lib/canvasBridge';
import { useEditor } from '../stores/editor';
import { useWorkspace } from '../stores/workspace';
import type { NodeDefinition } from '../types';

const CATEGORY_ORDER = [
  'Triggers',
  'HTTP',
  'Logic',
  'Data Processing',
  'Filesystem',
  'Notifications',
  'Utilities',
  'Logging',
];

function LibraryItem({ def, disabled }: { def: NodeDefinition; disabled: boolean }) {
  const color = categoryColor[def.category] ?? '#94a3b8';
  const add = () => {
    if (disabled) return;
    useEditor.getState().addNode(def, canvasBridge.centerPosition());
  };
  return (
    <button
      draggable={!disabled}
      disabled={disabled}
      onDragStart={(e) => {
        e.dataTransfer.setData(DND_MIME, def.type);
        e.dataTransfer.effectAllowed = 'copy';
      }}
      onClick={add}
      title={`${def.description}\n\nПеретащите на холст или кликните, чтобы добавить.`}
      className={cx(
        'group flex w-full items-start gap-2.5 rounded-lg border border-transparent p-2 text-left transition-colors duration-150',
        disabled
          ? 'cursor-not-allowed opacity-40'
          : 'cursor-grab hover:border-line hover:bg-panel-2 active:cursor-grabbing',
      )}
    >
      <div
        className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md"
        style={{ background: `${color}1f`, color }}
      >
        <NodeIcon name={def.icon} size={14} strokeWidth={2.2} />
      </div>
      <div className="min-w-0">
        <div className="text-[13px] font-medium text-fg">{def.name}</div>
        <div className="line-clamp-2 text-[11px] leading-snug text-muted">{def.description}</div>
      </div>
    </button>
  );
}

export function NodeLibrary() {
  const definitions = useWorkspace((s) => s.definitions);
  const hasWorkflow = useEditor((s) => !!s.workflowId);
  const [query, setQuery] = useState('');

  const groups = useMemo(() => {
    const q = query.trim().toLowerCase();
    const filtered = definitions.filter(
      (d) => !q || [d.name, d.description, d.category, d.type].some((t) => t.toLowerCase().includes(q)),
    );
    return CATEGORY_ORDER.map((c) => ({
      category: c,
      items: filtered.filter((d) => d.category === c),
    })).filter((g) => g.items.length);
  }, [definitions, query]);

  return (
    <aside className="flex h-full w-56 shrink-0 flex-col border-r border-line bg-panel">
      <div className="p-3 pb-2">
        <div className="relative">
          <Search
            size={14}
            className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-faint"
          />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Поиск Nodes…"
            className="pl-8"
            aria-label="Поиск Nodes"
          />
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {groups.map((g) => (
          <section key={g.category} className="mb-2">
            <div className="px-2">
              <SectionTitle>{g.category}</SectionTitle>
            </div>
            <div className="flex flex-col gap-0.5">
              {g.items.map((d) => (
                <LibraryItem key={d.type} def={d} disabled={!hasWorkflow} />
              ))}
            </div>
          </section>
        ))}
        {groups.length === 0 && <p className="px-2 py-4 text-center text-xs text-faint">Ничего не найдено</p>}
      </div>
    </aside>
  );
}
