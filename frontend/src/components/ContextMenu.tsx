import { useEffect, useRef, useState, type ReactNode } from 'react';
import {
  ChevronRight,
  ClipboardPaste,
  Copy,
  CopyPlus,
  LayoutGrid,
  Maximize,
  MousePointerSquareDashed,
  Play,
  Scissors,
  Search,
  Trash2,
} from 'lucide-react';
import { canvasBridge } from '../lib/canvasBridge';
import { useEditor } from '../stores/editor';
import { useRun } from '../stores/run';
import { toast, useUI } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import { categoryColor, NodeIcon } from './icons';
import { cx, Kbd } from './ui';

function Item({
  icon,
  label,
  hint,
  danger,
  disabled,
  onClick,
  children,
}: {
  icon?: ReactNode;
  label: string;
  hint?: string;
  danger?: boolean;
  disabled?: boolean;
  onClick?: () => void;
  children?: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="relative" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
      <button
        role="menuitem"
        disabled={disabled}
        onClick={onClick}
        className={cx(
          'flex h-8 w-full items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] transition-colors duration-100 disabled:pointer-events-none disabled:opacity-40',
          danger ? 'text-error hover:bg-error/10' : 'text-fg hover:bg-elevated',
        )}
      >
        <span className="flex w-4 shrink-0 justify-center text-muted">{icon}</span>
        <span className="flex-1">{label}</span>
        {hint && <Kbd>{hint}</Kbd>}
        {children && <ChevronRight size={14} className="text-faint" />}
      </button>
      {children && open && (
        <div className="absolute top-0 left-full z-10 ml-1 max-h-80 w-56 overflow-y-auto rounded-lg border border-line-strong bg-panel p-1 shadow-2xl shadow-black/50">
          {children}
        </div>
      )}
    </div>
  );
}

const Divider = () => <div className="my-1 h-px bg-line" />;

export function ContextMenu() {
  const menu = useUI((s) => s.contextMenu);
  const close = () => useUI.getState().openContextMenu(null);
  const ref = useRef<HTMLDivElement>(null);
  const definitions = useWorkspace((s) => s.definitions);
  const [pos, setPos] = useState({ x: 0, y: 0 });

  useEffect(() => {
    if (!menu) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) close();
    };
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && close();
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  }, [menu]);

  // Keep the menu inside the window.
  useEffect(() => {
    if (!menu || !ref.current) return;
    const r = ref.current.getBoundingClientRect();
    setPos({
      x: Math.min(menu.x, window.innerWidth - r.width - 8),
      y: Math.min(menu.y, window.innerHeight - r.height - 8),
    });
  }, [menu]);

  if (!menu) return null;
  const editor = useEditor.getState();
  const run = (fn: () => void | Promise<unknown>) => () => {
    close();
    void fn();
  };
  const node = menu.nodeId ? editor.nodes.find((n) => n.id === menu.nodeId) : undefined;
  const nodeDef = node ? useWorkspace.getState().defByType[node.data.nodeType] : undefined;
  const selectedCount = editor.nodes.filter((n) => n.selected).length;

  return (
    <div
      ref={ref}
      role="menu"
      style={{ left: pos.x || menu.x, top: pos.y || menu.y }}
      className="fixed z-50 w-60 animate-fade-in rounded-lg border border-line-strong bg-panel p-1 shadow-2xl shadow-black/50"
      onContextMenu={(e) => e.preventDefault()}
    >
      {menu.kind === 'node' && node ? (
        <>
          <div className="truncate px-2.5 py-1.5 text-[11px] text-faint">
            {selectedCount > 1 ? `Выбрано Nodes: ${selectedCount}` : node.data.name}
          </div>
          {nodeDef?.trigger && (
            <Item
              icon={<Play size={14} />}
              label="Запустить от этого триггера"
              onClick={run(() => useRun.getState().start(node.id))}
            />
          )}
          <Item
            icon={<Copy size={14} />}
            label="Копировать"
            hint="Ctrl+C"
            onClick={run(async () => {
              const n = await editor.copySelection();
              toast('info', `Скопировано Nodes: ${n}`);
            })}
          />
          <Item
            icon={<Scissors size={14} />}
            label="Вырезать"
            hint="Ctrl+X"
            onClick={run(() => editor.cutSelection())}
          />
          <Item
            icon={<CopyPlus size={14} />}
            label="Дублировать"
            hint="Ctrl+D"
            disabled={selectedCount > 1}
            onClick={run(() => editor.duplicateNode(node.id))}
          />
          <Divider />
          <Item
            icon={<Trash2 size={14} />}
            label={selectedCount > 1 ? 'Удалить выбранные' : 'Удалить'}
            hint="Del"
            danger
            onClick={run(() =>
              editor.deleteNodes(
                selectedCount > 1 ? editor.nodes.filter((n) => n.selected).map((n) => n.id) : [node.id],
              ),
            )}
          />
        </>
      ) : (
        <>
          <Item
            icon={<ClipboardPaste size={14} />}
            label="Вставить"
            hint="Ctrl+V"
            onClick={run(() => editor.paste(menu.flow))}
          />
          <Item icon={<NodeIcon name="" size={14} />} label="Добавить Node">
            {definitions.map((d) => (
              <Item
                key={d.type}
                icon={<NodeIcon name={d.icon} size={14} style={{ color: categoryColor[d.category] }} />}
                label={d.name}
                onClick={run(() => {
                  editor.addNode(d, menu.flow ?? canvasBridge.centerPosition());
                })}
              />
            ))}
          </Item>
          <Divider />
          <Item
            icon={<LayoutGrid size={14} />}
            label="Авто-раскладка"
            hint="Ctrl+Shift+L"
            onClick={run(() => {
              editor.layout();
              window.setTimeout(() => canvasBridge.fitView(), 50);
            })}
          />
          <Item
            icon={<MousePointerSquareDashed size={14} />}
            label="Выделить всё"
            hint="Ctrl+A"
            onClick={run(() => editor.selectAll())}
          />
          <Item
            icon={<Maximize size={14} />}
            label="Показать всё"
            onClick={run(() => canvasBridge.fitView())}
          />
          <Item
            icon={<Search size={14} />}
            label="Поиск"
            hint="Ctrl+K"
            onClick={run(() => useUI.getState().setPaletteOpen(true))}
          />
        </>
      )}
    </div>
  );
}
