import { CalendarClock, Plus, X, Zap } from 'lucide-react';
import { Button, cx, IconButton, Input, SectionTitle, Select, Toggle } from '../components/ui';
import { DEFAULT_NODE_SETTINGS, KEY_PATTERN, SYSTEM_VARIABLES, type FlowNode } from '../lib/graph';
import { useEditor } from '../stores/editor';
import { useWorkspace } from '../stores/workspace';
import type { NodeSettings, OnError } from '../types';

function Row({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-muted">{label}</span>
      {children}
      {hint && <span className="text-[11px] leading-snug text-faint">{hint}</span>}
    </label>
  );
}

const onErrorHints: Record<OnError, string> = {
  stop: 'Node получает статус FAILED, дальнейшие Nodes этого пути пропускаются, запуск завершается с ошибкой.',
  continue:
    'Ошибка записывается в лог, выполнение продолжается по основному выходу. Текст ошибки доступен как {{key.error}}.',
  error_output: 'Появляется выход ERROR: подключите к нему Nodes для обработки ошибки ({{key.error}}).',
};

/** Retry / timeout / error handling for regular nodes. */
export function NodeSettingsForm({ node }: { node: FlowNode }) {
  const setNodeSettings = useEditor((s) => s.setNodeSettings);
  const s: NodeSettings = {
    ...DEFAULT_NODE_SETTINGS,
    ...node.data.settings,
    retry: { ...DEFAULT_NODE_SETTINGS.retry, ...node.data.settings?.retry },
  };
  const update = (patch: Partial<NodeSettings>) => setNodeSettings(node.id, { ...s, ...patch });
  const updateRetry = (patch: Partial<NodeSettings['retry']>) => update({ retry: { ...s.retry, ...patch } });
  const num = (v: string, min: number, max: number) => Math.min(Math.max(Number(v) || 0, min), max);

  return (
    <div className="flex flex-col gap-5">
      <section className="flex flex-col gap-3">
        <SectionTitle>Обработка ошибок</SectionTitle>
        <Row label="При ошибке" hint={onErrorHints[s.onError]}>
          <Select value={s.onError} onChange={(e) => update({ onError: e.target.value as OnError })}>
            <option value="stop">Остановить путь (по умолчанию)</option>
            <option value="continue">Continue On Error — продолжить</option>
            <option value="error_output">Error Output — выход ERROR</option>
          </Select>
        </Row>
      </section>

      <section className="flex flex-col gap-3">
        <SectionTitle>Повтор (Retry)</SectionTitle>
        <Row label="Всего попыток" hint="1 — без повторов. Максимум 10.">
          <Input
            type="number"
            min={1}
            max={10}
            mono
            value={s.retry.maxAttempts}
            onChange={(e) => updateRetry({ maxAttempts: num(e.target.value, 1, 10) })}
          />
        </Row>
        {s.retry.maxAttempts > 1 && (
          <>
            <Row label="Пауза между попытками, мс">
              <Input
                type="number"
                min={0}
                mono
                value={s.retry.delayMs}
                onChange={(e) => updateRetry({ delayMs: num(e.target.value, 0, 600_000) })}
              />
            </Row>
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs font-medium text-muted">Exponential backoff (×2 каждую попытку)</span>
              <Toggle
                checked={s.retry.backoff}
                onChange={(v) => updateRetry({ backoff: v })}
                label="Backoff"
              />
            </div>
            {s.retry.backoff && (
              <Row label="Максимальная пауза, мс" hint="0 — без ограничения (до 10 минут).">
                <Input
                  type="number"
                  min={0}
                  mono
                  value={s.retry.maxDelayMs}
                  onChange={(e) => updateRetry({ maxDelayMs: num(e.target.value, 0, 600_000) })}
                />
              </Row>
            )}
          </>
        )}
      </section>

      <section className="flex flex-col gap-3">
        <SectionTitle>Таймаут</SectionTitle>
        <Row label="Таймаут Node, сек" hint="0 — без ограничения. Применяется к каждой попытке.">
          <Input
            type="number"
            min={0}
            max={3600}
            mono
            value={s.timeoutSec}
            onChange={(e) => update({ timeoutSec: num(e.target.value, 0, 3600) })}
          />
        </Row>
      </section>
    </div>
  );
}

/** Automation status, variables and workflow-level settings. */
export function WorkflowAutomation() {
  const workflowId = useEditor((s) => s.workflowId);
  const variables = useEditor((s) => s.variables);
  const wfSettings = useEditor((s) => s.wfSettings);
  const { setVariables, setWorkflowSettings } = useEditor.getState();
  const automated = useWorkspace((s) => s.isActive(workflowId));
  const triggers = useWorkspace((s) => s.triggers);
  const toggling = useWorkspace((s) => s.togglingActive);
  const setActive = useWorkspace((s) => s.setActive);

  const update = (i: number, patch: Partial<(typeof variables)[number]>) =>
    setVariables(variables.map((v, j) => (j === i ? { ...v, ...patch } : v)));

  return (
    <>
      <section className="flex flex-col gap-2">
        <SectionTitle>
          <span className="flex items-center gap-1.5">
            <Zap size={12} /> Автоматизация
          </span>
        </SectionTitle>
        <div className="flex items-center justify-between gap-3 rounded-md border border-line bg-bg p-2.5">
          <div className="text-xs leading-snug">
            <div className={automated ? 'text-success' : 'text-muted'}>
              {automated ? 'Включена' : 'Выключена'}
            </div>
            <div className="text-[11px] text-faint">Cron, Interval, Schedule, File Watcher, Startup</div>
          </div>
          <Toggle
            checked={automated}
            onChange={(v) => workflowId && !toggling && void setActive(workflowId, v)}
            label="Автоматизация"
          />
        </div>
        {automated && triggers.length > 0 && (
          <ul className="flex flex-col gap-1">
            {triggers.map((t) => (
              <li key={t.nodeId} className="flex items-start gap-2 text-xs">
                <CalendarClock size={13} className="mt-0.5 shrink-0 text-success" />
                <div className="min-w-0">
                  <div className="truncate">{t.nodeName}</div>
                  <div className="text-[11px] text-faint">
                    {t.nextRun > 0 ? `ближайший: ${new Date(t.nextRun).toLocaleString('ru-RU')}` : t.detail}
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="flex flex-col gap-2">
        <SectionTitle>Переменные Workflow</SectionTitle>
        {variables.map((v, i) => (
          <div key={i} className="flex items-center gap-1.5">
            <Input
              value={v.key}
              placeholder="имя"
              mono
              className={cx('w-[40%]', v.key && !KEY_PATTERN.test(v.key) && 'border-error/70')}
              onChange={(e) => update(i, { key: e.target.value.replace(/\s/g, '_') })}
            />
            <Input
              value={v.value}
              placeholder="значение"
              mono
              className="flex-1"
              onChange={(e) => update(i, { value: e.target.value })}
            />
            <IconButton
              size="sm"
              label="Удалить"
              onClick={() => setVariables(variables.filter((_, j) => j !== i))}
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
          onClick={() => setVariables([...variables, { key: '', value: '' }])}
        >
          Добавить переменную
        </Button>
        <p className="text-[11px] leading-snug text-faint">
          Доступны как <span className="font-mono text-accent">{'{{variables.имя}}'}</span>. Значения
          переменных с именами вроде token/password маскируются в логах и не экспортируются.
        </p>
      </section>

      <section className="flex flex-col gap-2">
        <SectionTitle>Ограничения</SectionTitle>
        <Row
          label="Таймаут Workflow, сек"
          hint="0 — без ограничения. При превышении запуск получает статус TIMEOUT."
        >
          <Input
            type="number"
            min={0}
            max={86400}
            mono
            value={wfSettings.timeoutSec}
            onChange={(e) =>
              setWorkflowSettings({ timeoutSec: Math.min(Math.max(Number(e.target.value) || 0, 0), 86400) })
            }
          />
        </Row>
      </section>
    </>
  );
}

/** System, workflow and environment variables shown in the Inspector "Data" tab. */
export function SystemVariables({ copy }: { copy: (text: string) => void }) {
  const variables = useEditor((s) => s.variables);
  const envVars = useWorkspace((s) => s.settings?.allowedEnvVars ?? []);
  const chips = [
    ...SYSTEM_VARIABLES.map((v) => ({ path: v.path, description: v.description })),
    ...variables
      .filter((v) => v.key)
      .map((v) => ({ path: `variables.${v.key}`, description: 'Переменная Workflow' })),
    ...envVars.map((name) => ({ path: `env.${name}`, description: 'Переменная окружения' })),
  ];
  return (
    <div>
      <div className="mb-1 text-[11px] text-muted">Системные и переменные Workflow</div>
      <div className="flex flex-wrap gap-1">
        {chips.map((c) => (
          <button
            key={c.path}
            title={`${c.description} — нажмите, чтобы скопировать`}
            onClick={() => copy(`{{${c.path}}}`)}
            className="rounded border border-line bg-bg px-1.5 py-0.5 font-mono text-[10.5px] text-info transition-colors duration-150 hover:border-info/50"
          >
            {`{{${c.path}}}`}
          </button>
        ))}
      </div>
    </div>
  );
}
