import { useMemo, useState, type ReactNode } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { AlertCircle, AlertTriangle, Copy, CopyPlus, Plus, Trash2, Variable, X } from 'lucide-react';
import { categoryColor, NodeIcon } from '../components/icons';
import { Button, cx, IconButton, Input, SectionTitle, Select, Textarea, Toggle } from '../components/ui';
import { canvasBridge } from '../lib/canvasBridge';
import { ancestors, KEY_PATTERN, keyValues, RESERVED_KEYS, str, type FlowNode } from '../lib/graph';
import { NodeSettingsForm, SystemVariables, WorkflowAutomation } from './InspectorExtras';
import { statusMeta } from '../nodes/status';
import { useEditor } from '../stores/editor';
import { formatDuration, useRun } from '../stores/run';
import { toast } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Issue, KeyValueRow, ParamSpec, Params } from '../types';

function Field({
  label,
  hint,
  error,
  required,
  children,
}: {
  label: string;
  hint?: ReactNode;
  error?: string;
  required?: boolean;
  children: ReactNode;
}) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="flex items-center gap-1 text-xs font-medium text-muted">
        {label}
        {required && <span className="text-error">*</span>}
      </span>
      {children}
      {error ? (
        <span className="flex items-start gap-1 text-[11px] leading-snug text-error">
          <AlertCircle size={12} className="mt-px shrink-0" />
          {error}
        </span>
      ) : (
        hint && <span className="text-[11px] leading-snug text-faint">{hint}</span>
      )}
    </label>
  );
}

function KeyValueEditor({
  rows,
  onChange,
  templatable,
}: {
  rows: KeyValueRow[];
  onChange: (rows: KeyValueRow[]) => void;
  templatable: boolean;
}) {
  const update = (i: number, patch: Partial<KeyValueRow>) =>
    onChange(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  return (
    <div className="flex flex-col gap-1.5">
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-1.5">
          <Toggle checked={r.enabled} onChange={(v) => update(i, { enabled: v })} label="Включено" />
          <Input
            value={r.key}
            onChange={(e) => update(i, { key: e.target.value })}
            placeholder="Ключ"
            mono
            className={cx('w-[42%]', !r.enabled && 'opacity-50')}
          />
          <Input
            value={r.value}
            onChange={(e) => update(i, { value: e.target.value })}
            placeholder={templatable ? 'Значение или {{var}}' : 'Значение'}
            mono
            className={cx('flex-1', !r.enabled && 'opacity-50')}
          />
          <IconButton
            size="sm"
            label="Удалить строку"
            onClick={() => onChange(rows.filter((_, j) => j !== i))}
          >
            <X size={13} />
          </IconButton>
        </div>
      ))}
      <Button
        size="sm"
        variant="ghost"
        icon={<Plus size={13} />}
        className="self-start"
        onClick={() => onChange([...rows, { key: '', value: '', enabled: true }])}
      >
        Добавить
      </Button>
    </div>
  );
}

function WorkflowSelect({
  value,
  onChange,
  invalid,
}: {
  value: string;
  onChange: (v: string) => void;
  invalid: boolean;
}) {
  const projects = useWorkspace((s) => s.projects);
  const workflows = useWorkspace((s) => s.workflows);
  const currentId = useEditor((s) => s.workflowId);
  return (
    <Select value={value} onChange={(e) => onChange(e.target.value)} invalid={invalid}>
      <option value="">— выберите Workflow —</option>
      {projects.map((p) => (
        <optgroup key={p.id} label={p.name}>
          {(workflows[p.id] ?? [])
            .filter((w) => w.id !== currentId)
            .map((w) => (
              <option key={w.id} value={w.id}>
                {w.name}
              </option>
            ))}
        </optgroup>
      ))}
    </Select>
  );
}

function isOptional(spec: ParamSpec, params: Params, specs: ParamSpec[]): boolean {
  if (!spec.optionalWhen) return false;
  const dep = specs.find((s) => s.key === spec.optionalWhen!.param);
  const current = str(params, spec.optionalWhen.param, String(dep?.default ?? ''));
  return spec.optionalWhen.in.includes(current);
}

function ParamInput({
  node,
  spec,
  specs,
  error,
}: {
  node: FlowNode;
  spec: ParamSpec;
  specs: ParamSpec[];
  error?: string;
}) {
  const setParam = useEditor((s) => s.setParam);
  const params = node.data.params;
  const value = params[spec.key];
  const set = (v: unknown) => setParam(node.id, spec.key, v);
  const optional = isOptional(spec, params, specs);
  const required = spec.required && !optional;
  const hint =
    optional && spec.key === 'body'
      ? 'Для GET тело запроса не отправляется'
      : (spec.description ?? (spec.templatable ? 'Поддерживает переменные {{node.field}}' : undefined));

  let control: ReactNode;
  switch (spec.kind) {
    case 'text':
      control = (
        <Textarea
          value={str(params, spec.key)}
          onChange={(e) => set(e.target.value)}
          placeholder={spec.placeholder}
          invalid={!!error}
          rows={spec.key === 'body' ? 6 : 3}
        />
      );
      break;
    case 'number':
      control = (
        <Input
          type="number"
          value={value === undefined || value === null ? '' : String(value)}
          min={spec.min}
          max={spec.max}
          onChange={(e) => set(e.target.value === '' ? '' : Number(e.target.value))}
          invalid={!!error}
          mono
        />
      );
      break;
    case 'select':
      control = (
        <Select
          value={str(params, spec.key, String(spec.default ?? ''))}
          onChange={(e) => set(e.target.value)}
          invalid={!!error}
        >
          {spec.options?.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </Select>
      );
      break;
    case 'boolean':
      return (
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs font-medium text-muted">{spec.label}</span>
          <Toggle checked={Boolean(value)} onChange={set} label={spec.label} />
        </div>
      );
    case 'workflow':
      control = <WorkflowSelect value={str(params, spec.key)} onChange={set} invalid={!!error} />;
      break;
    case 'keyvalue':
      control = (
        <KeyValueEditor rows={keyValues(params, spec.key)} onChange={set} templatable={spec.templatable} />
      );
      break;
    default:
      control = (
        <Input
          value={str(params, spec.key)}
          onChange={(e) => set(e.target.value)}
          placeholder={spec.placeholder}
          invalid={!!error}
          mono={spec.templatable}
        />
      );
  }
  return (
    <Field label={spec.label} required={required} error={error} hint={hint}>
      {control}
    </Field>
  );
}

function VariablesHelp({ node }: { node: FlowNode }) {
  const nodes = useEditor((s) => s.nodes);
  const edges = useEditor((s) => s.edges);
  const defByType = useWorkspace((s) => s.defByType);
  const upstream = useMemo(() => {
    const ids = ancestors(edges, node.id);
    return nodes.filter((n) => ids.has(n.id));
  }, [nodes, edges, node.id]);

  const copy = (text: string) => {
    void navigator.clipboard?.writeText(text).then(
      () => toast('info', 'Скопировано', text),
      () => toast('warning', 'Буфер обмена недоступен', text),
    );
  };
  return (
    <div className="flex flex-col gap-2">
      <SystemVariables copy={copy} />
      {!upstream.length && (
        <p className="text-xs leading-relaxed text-faint">
          Подключите этот Node к предыдущим — их результаты станут доступны как переменные.
        </p>
      )}
      {upstream.map((n) => (
        <div key={n.id}>
          <div className="mb-1 text-[11px] text-muted">{n.data.name}</div>
          <div className="flex flex-wrap gap-1">
            {(defByType[n.data.nodeType]?.outputDoc ?? []).map((f) => {
              const tpl = `{{${n.data.key}.${f.name}}}`;
              return (
                <button
                  key={f.name}
                  title={`${f.description} — нажмите, чтобы скопировать`}
                  onClick={() => copy(tpl)}
                  className="rounded border border-line bg-bg px-1.5 py-0.5 font-mono text-[10.5px] text-accent transition-colors duration-150 hover:border-accent/50"
                >
                  {tpl}
                </button>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}

function OutputSection({ node }: { node: FlowNode }) {
  const def = useWorkspace((s) => s.defByType[node.data.nodeType]);
  const info = useRun((s) => s.nodes[node.id]);
  const output = info?.output;
  const json = useMemo(() => (output === undefined ? '' : JSON.stringify(output, null, 2)), [output]);
  return (
    <div className="flex flex-col gap-2">
      {info && (
        <div className="flex items-center gap-2 text-xs">
          <span className={cx('size-2 rounded-full', statusMeta[info.status].color)} />
          <span className={statusMeta[info.status].text}>{statusMeta[info.status].label}</span>
          {info.durationMs !== undefined && info.status !== 'WAITING' && (
            <span className="text-faint">{formatDuration(info.durationMs)}</span>
          )}
          {info.branch && <span className="font-mono text-faint">→ {info.branch.toUpperCase()}</span>}
        </div>
      )}
      {info?.error && (
        <div className="selectable rounded-md border border-error/30 bg-error/10 p-2 text-xs leading-relaxed text-error">
          {info.error}
        </div>
      )}
      {json ? (
        <div className="relative">
          <pre className="selectable max-h-72 overflow-auto rounded-md border border-line bg-bg p-2.5 font-mono text-[11px] leading-relaxed text-muted">
            {json}
          </pre>
          <IconButton
            size="sm"
            label="Копировать JSON"
            className="absolute top-1.5 right-1.5"
            onClick={() => void navigator.clipboard?.writeText(json)}
          >
            <Copy size={12} />
          </IconButton>
        </div>
      ) : (
        <ul className="flex flex-col gap-1">
          {def?.outputDoc.map((f) => (
            <li key={f.name} className="flex gap-2 text-xs">
              <span className="font-mono text-accent">{f.name}</span>
              <span className="text-faint">{f.description}</span>
            </li>
          ))}
          <li className="pt-1 text-[11px] text-faint">Результат появится после запуска.</li>
        </ul>
      )}
    </div>
  );
}

function NodeInspector({ node, issues }: { node: FlowNode; issues: Issue[] }) {
  const def = useWorkspace((s) => s.defByType[node.data.nodeType]);
  const { renameNode, setNodeKey, deleteNodes, duplicateNode } = useEditor.getState();
  const [tab, setTab] = useState<'params' | 'settings' | 'data'>('params');
  if (!def) return <div className="p-4 text-xs text-error">Неизвестный тип Node: {node.data.nodeType}</div>;

  const nodeIssues = issues.filter((i) => i.nodeId === node.id);
  const fieldError = (field: string) =>
    nodeIssues.find((i) => i.field === field && i.severity === 'error')?.message.replace(/^«[^»]*» → /, '');
  const general = nodeIssues.filter((i) => !i.field);
  const keyLocalError = !KEY_PATTERN.test(node.data.key)
    ? 'Только латиница, цифры и _, начинается с буквы'
    : RESERVED_KEYS.has(node.data.key)
      ? 'Это имя зарезервировано (execution, workflow, variables, env)'
      : undefined;
  const color = categoryColor[def.category] ?? '#94a3b8';

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-b border-line p-4">
        <div className="flex items-start gap-3">
          <div
            className="flex size-9 shrink-0 items-center justify-center rounded-lg"
            style={{ background: `${color}22`, color }}
          >
            <NodeIcon name={def.icon} size={17} strokeWidth={2.2} />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold">{def.name}</div>
            <div className="font-mono text-[11px] text-faint">{def.type}</div>
          </div>
          <IconButton label="Дублировать (Ctrl+D)" onClick={() => duplicateNode(node.id)}>
            <CopyPlus size={15} />
          </IconButton>
          <IconButton
            label="Удалить Node (Delete)"
            className="hover:text-error"
            onClick={() => deleteNodes([node.id])}
          >
            <Trash2 size={15} />
          </IconButton>
        </div>
        <p className="mt-2.5 text-xs leading-relaxed text-muted">{def.description}</p>
        {general.map((i, k) => (
          <div
            key={k}
            className={cx(
              'mt-2 flex items-start gap-1.5 text-[11px] leading-snug',
              i.severity === 'error' ? 'text-error' : 'text-warning',
            )}
          >
            {i.severity === 'error' ? (
              <AlertCircle size={12} className="mt-px shrink-0" />
            ) : (
              <AlertTriangle size={12} className="mt-px shrink-0" />
            )}
            {i.message}
          </div>
        ))}
      </div>

      <div className="flex gap-1 border-b border-line px-3">
        {(
          [
            ['params', 'Параметры'],
            ...(def.trigger ? [] : ([['settings', 'Настройки']] as const)),
            ['data', 'Данные'],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            onClick={() => setTab(id)}
            className={cx(
              '-mb-px border-b-2 px-2 py-2 text-xs font-medium transition-colors duration-150',
              tab === id ? 'border-accent text-fg' : 'border-transparent text-muted hover:text-fg',
            )}
          >
            {label}
          </button>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {tab === 'settings' && !def.trigger ? (
          <NodeSettingsForm node={node} />
        ) : tab === 'params' ? (
          <div className="flex flex-col gap-4">
            <Field label="Название" required error={fieldError('name')}>
              <Input value={node.data.name} onChange={(e) => renameNode(node.id, e.target.value)} />
            </Field>
            <Field
              label="Имя переменной"
              required
              error={keyLocalError ?? fieldError('key')}
              hint={
                <>
                  Результат доступен как{' '}
                  <span className="font-mono text-accent">{`{{${node.data.key || '…'}.${def.outputDoc[0]?.name ?? 'field'}}}`}</span>
                </>
              }
            >
              <Input
                value={node.data.key}
                onChange={(e) => setNodeKey(node.id, e.target.value.replace(/\s/g, '_'))}
                mono
              />
            </Field>
            <div className="h-px bg-line" />
            {def.params.map((spec) => (
              <ParamInput
                key={spec.key}
                node={node}
                spec={spec}
                specs={def.params}
                error={fieldError(spec.key)}
              />
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-5">
            <section>
              <SectionTitle>
                <span className="flex items-center gap-1.5">
                  <Variable size={12} /> Входные данные
                </span>
              </SectionTitle>
              <VariablesHelp node={node} />
            </section>
            <section>
              <SectionTitle>Выходные данные</SectionTitle>
              <OutputSection node={node} />
            </section>
          </div>
        )}
      </div>
    </div>
  );
}

function WorkflowInspector({ issues }: { issues: Issue[] }) {
  const name = useEditor((s) => s.name);
  const description = useEditor((s) => s.description);
  const workflowId = useEditor((s) => s.workflowId);
  const nodeCount = useEditor((s) => s.nodes.length);
  const edgeCount = useEditor((s) => s.edges.length);
  const renameWorkflow = useWorkspace((s) => s.renameWorkflow);
  const [desc, setDesc] = useState(description);
  const [descFor, setDescFor] = useState(workflowId);
  if (descFor !== workflowId) {
    setDescFor(workflowId);
    setDesc(description);
  }
  if (!workflowId)
    return <div className="p-4 text-xs text-faint">Откройте Workflow, чтобы увидеть его свойства.</div>;
  const errors = issues.filter((i) => i.severity === 'error');
  const warnings = issues.filter((i) => i.severity === 'warning');

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4">
      <div>
        <div className="text-sm font-semibold">{name}</div>
        <div className="mt-0.5 text-xs text-faint">
          {nodeCount} Nodes · {edgeCount} Connections
        </div>
      </div>
      <Field label="Описание">
        <Textarea
          value={desc}
          onChange={(e) => setDesc(e.target.value)}
          onBlur={() => desc !== description && void renameWorkflow(workflowId, name, desc)}
          placeholder="Что делает этот Workflow?"
          className="font-sans text-[13px]"
          rows={3}
        />
      </Field>
      <WorkflowAutomation />
      <section>
        <SectionTitle>Проверка</SectionTitle>
        {issues.length === 0 ? (
          <p className="text-xs text-success">Ошибок не найдено — Workflow готов к запуску.</p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {[...errors, ...warnings].map((i, k) => (
              <li key={k}>
                <button
                  disabled={!i.nodeId}
                  onClick={() => i.nodeId && canvasBridge.focusNode(i.nodeId)}
                  className={cx(
                    'flex w-full items-start gap-1.5 rounded-md p-1.5 text-left text-xs leading-snug transition-colors duration-150 enabled:hover:bg-elevated',
                    i.severity === 'error' ? 'text-error' : 'text-warning',
                  )}
                >
                  {i.severity === 'error' ? (
                    <AlertCircle size={13} className="mt-px shrink-0" />
                  ) : (
                    <AlertTriangle size={13} className="mt-px shrink-0" />
                  )}
                  {i.message}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
      <section className="text-xs leading-relaxed text-muted">
        <SectionTitle>Подсказки</SectionTitle>
        <ul className="list-disc space-y-1 pl-4">
          <li>Выберите Node, чтобы настроить его параметры.</li>
          <li>
            Переменные: <span className="font-mono text-accent">{'{{http.status}}'}</span>,{' '}
            <span className="font-mono text-accent">{'{{http.body.message}}'}</span>.
          </li>
          <li>Shift + перетаскивание — выделение рамкой, Ctrl + клик — несколько Nodes.</li>
        </ul>
      </section>
    </div>
  );
}

export function Inspector() {
  const selected = useEditor(useShallow((s) => s.nodes.filter((n) => n.selected)));
  const issues = useRun((s) => s.issues);
  const deleteNodes = useEditor((s) => s.deleteNodes);

  return (
    <aside className="flex h-full w-80 shrink-0 flex-col border-l border-line bg-panel">
      <div className="flex h-10 shrink-0 items-center border-b border-line px-4">
        <span className="text-[11px] font-semibold tracking-wider text-faint uppercase">Inspector</span>
      </div>
      {selected.length === 1 ? (
        <NodeInspector key={selected[0].id} node={selected[0]} issues={issues} />
      ) : selected.length > 1 ? (
        <div className="flex flex-col gap-3 p-4">
          <p className="text-sm">Выбрано Nodes: {selected.length}</p>
          <Button
            variant="danger"
            icon={<Trash2 size={14} />}
            onClick={() => deleteNodes(selected.map((n) => n.id))}
          >
            Удалить выбранные
          </Button>
        </div>
      ) : (
        <WorkflowInspector issues={issues} />
      )}
    </aside>
  );
}
