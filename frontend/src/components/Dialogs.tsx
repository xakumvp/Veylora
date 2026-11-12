import { useEffect, useRef, useState } from 'react';
import { AlertTriangle, CheckCircle2, FolderPlus, Info, ShieldAlert, X, XCircle } from 'lucide-react';
import { api } from '../services/api';
import { useUI } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Settings } from '../types';
import { Logo } from './icons';
import { Button, cx, Input, Modal, Toggle } from './ui';

function PromptDialog() {
  const prompt = useUI((s) => s.prompt);
  const close = useUI((s) => s.closePrompt);
  const [value, setValue] = useState('');
  const [forPrompt, setForPrompt] = useState(prompt);
  const ref = useRef<HTMLInputElement>(null);
  if (forPrompt !== prompt) {
    setForPrompt(prompt);
    setValue(prompt?.initial ?? '');
  }
  useEffect(() => {
    if (prompt) window.setTimeout(() => ref.current?.select(), 30);
  }, [prompt]);
  if (!prompt) return null;
  const submit = () => value.trim() && close(value.trim());
  return (
    <Modal
      title={prompt.title}
      onClose={() => close(null)}
      footer={
        <>
          <Button variant="ghost" onClick={() => close(null)}>
            Отмена
          </Button>
          <Button variant="primary" onClick={submit} disabled={!value.trim()}>
            {prompt.confirmLabel ?? 'OK'}
          </Button>
        </>
      }
    >
      <label className="flex flex-col gap-1.5">
        {prompt.label && <span className="text-xs font-medium text-muted">{prompt.label}</span>}
        <Input
          ref={ref}
          value={value}
          maxLength={120}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
        />
      </label>
    </Modal>
  );
}

function ConfirmDialog() {
  const confirm = useUI((s) => s.confirm);
  const close = useUI((s) => s.closeConfirm);
  if (!confirm) return null;
  return (
    <Modal
      title={confirm.title}
      onClose={() => close(false)}
      footer={
        <>
          <Button variant="ghost" onClick={() => close(false)}>
            Отмена
          </Button>
          <Button variant={confirm.danger ? 'danger' : 'primary'} onClick={() => close(true)} autoFocus>
            {confirm.confirmLabel ?? 'OK'}
          </Button>
        </>
      }
    >
      <p className="text-[13px] leading-relaxed text-muted">{confirm.message}</p>
    </Modal>
  );
}

function SettingsDialog() {
  const open = useUI((s) => s.settingsOpen);
  const setOpen = useUI((s) => s.setSettingsOpen);
  const settings = useWorkspace((s) => s.settings);
  const saveSettings = useWorkspace((s) => s.saveSettings);
  const [draft, setDraft] = useState<Settings | undefined>(settings);
  const [envText, setEnvText] = useState('');
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) {
      setDraft(settings);
      setEnvText((settings?.allowedEnvVars ?? []).join(', '));
    }
  }
  if (!open || !draft) return null;

  return (
    <Modal
      title="Settings"
      width="w-120"
      onClose={() => setOpen(false)}
      footer={
        <>
          <Button variant="ghost" onClick={() => setOpen(false)}>
            Отмена
          </Button>
          <Button
            variant="primary"
            onClick={async () => {
              await saveSettings({
                ...draft,
                allowedEnvVars: envText
                  .split(',')
                  .map((v) => v.trim())
                  .filter(Boolean),
              });
              setOpen(false);
            }}
          >
            Сохранить
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-[13px] font-medium">Доступ к локальной сети</div>
            <p className="mt-1 text-xs leading-relaxed text-muted">
              Разрешить HTTP Request обращаться к localhost, 127.0.0.1 и адресам локальной сети (192.168.x.x,
              10.x.x.x). По умолчанию выключено для безопасности.
            </p>
            {draft.allowLocalNetwork && (
              <p className="mt-1.5 flex items-center gap-1 text-[11px] text-warning">
                <ShieldAlert size={12} /> Workflow из чужих файлов смогут обращаться к вашей локальной сети
              </p>
            )}
          </div>
          <Toggle
            checked={draft.allowLocalNetwork}
            onChange={(v) => setDraft({ ...draft, allowLocalNetwork: v })}
            label="Доступ к локальной сети"
          />
        </div>
        <label className="flex items-center justify-between gap-4">
          <div>
            <div className="text-[13px] font-medium">Максимальный размер HTTP-ответа</div>
            <p className="mt-1 text-xs text-muted">
              От 1 до 50 МБ. Более крупные ответы завершаются ошибкой.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Input
              type="number"
              min={1}
              max={50}
              className="w-20"
              value={draft.maxResponseMb}
              onChange={(e) => setDraft({ ...draft, maxResponseMb: Number(e.target.value) })}
              mono
            />
            <span className="text-xs text-muted">МБ</span>
          </div>
        </label>
        <label className="flex items-center justify-between gap-4">
          <div>
            <div className="text-[13px] font-medium">Хранить запусков на Workflow</div>
            <p className="mt-1 text-xs text-muted">Старые запуски и их логи удаляются автоматически.</p>
          </div>
          <Input
            type="number"
            min={1}
            max={500}
            className="w-20"
            value={draft.keepExecutions}
            onChange={(e) => setDraft({ ...draft, keepExecutions: Number(e.target.value) })}
            mono
          />
        </label>
        <label className="flex items-center justify-between gap-4">
          <div>
            <div className="text-[13px] font-medium">Одновременных запусков</div>
            <p className="mt-1 text-xs text-muted">Сколько Workflow могут выполняться параллельно (1–16).</p>
          </div>
          <Input
            type="number"
            min={1}
            max={16}
            className="w-20"
            value={draft.maxConcurrentRuns}
            onChange={(e) => setDraft({ ...draft, maxConcurrentRuns: Number(e.target.value) })}
            mono
          />
        </label>
        <div>
          <div className="flex items-center justify-between gap-4">
            <div>
              <div className="text-[13px] font-medium">Разрешённые каталоги</div>
              <p className="mt-1 text-xs text-muted">
                File Watcher и будущие файловые Nodes работают только внутри этих каталогов.
              </p>
            </div>
            <Button
              size="sm"
              icon={<FolderPlus size={13} />}
              onClick={async () => {
                const dir = await api.pickDirectory('Разрешить доступ к каталогу');
                if (dir) setDraft({ ...draft, allowedDirectories: [...draft.allowedDirectories, dir] });
              }}
            >
              Добавить
            </Button>
          </div>
          <ul className="mt-2 flex flex-col gap-1">
            {draft.allowedDirectories.map((d, i) => (
              <li
                key={d + i}
                className="flex items-center gap-2 rounded-md border border-line bg-bg px-2 py-1"
              >
                <span
                  className="selectable min-w-0 flex-1 truncate font-mono text-[11px] text-muted"
                  title={d}
                >
                  {d}
                </span>
                <button
                  className="text-faint hover:text-error"
                  aria-label="Удалить каталог"
                  onClick={() =>
                    setDraft({
                      ...draft,
                      allowedDirectories: draft.allowedDirectories.filter((_, j) => j !== i),
                    })
                  }
                >
                  <X size={13} />
                </button>
              </li>
            ))}
            {draft.allowedDirectories.length === 0 && (
              <li className="text-[11px] text-faint">
                Нет разрешённых каталогов — файловые триггеры недоступны.
              </li>
            )}
          </ul>
        </div>
        <label className="flex flex-col gap-1.5">
          <div className="text-[13px] font-medium">Переменные окружения</div>
          <p className="text-xs text-muted">
            Имена через запятую. Только они будут доступны как {'{{env.ИМЯ}}'}.
          </p>
          <Input
            value={envText}
            placeholder="HOME, USERNAME"
            mono
            onChange={(e) => setEnvText(e.target.value)}
            onBlur={() =>
              setDraft({
                ...draft,
                allowedEnvVars: envText
                  .split(',')
                  .map((v) => v.trim())
                  .filter(Boolean),
              })
            }
          />
        </label>
      </div>
    </Modal>
  );
}

function AboutDialog() {
  const open = useUI((s) => s.aboutOpen);
  const setOpen = useUI((s) => s.setAboutOpen);
  const info = useWorkspace((s) => s.info);
  if (!open) return null;
  return (
    <Modal title="About" onClose={() => setOpen(false)}>
      <div className="flex flex-col items-center gap-3 py-2 text-center">
        <Logo size={48} />
        <div>
          <div className="text-lg font-semibold">Veylora</div>
          <div className="font-mono text-xs text-faint">v{info?.version}</div>
        </div>
        <p className="max-w-xs text-xs leading-relaxed text-muted">
          Локальное desktop-приложение для визуального создания и выполнения автоматизаций. Работает без
          регистрации и облака.
        </p>
        <div className="mt-2 w-full rounded-lg border border-line bg-bg p-3 text-left text-[11px] leading-relaxed">
          <div className="text-faint">Данные</div>
          <div className="selectable font-mono break-all text-muted">{info?.dataDir}</div>
          <div className="mt-2 text-faint">Журнал диагностики</div>
          <div className="selectable font-mono break-all text-muted">{info?.logFile || '—'}</div>
        </div>
        <p className="text-[11px] text-faint">Go · Wails · React · React Flow · SQLite — MIT License</p>
      </div>
    </Modal>
  );
}

const toastIcon = {
  success: <CheckCircle2 size={16} className="text-success" />,
  error: <XCircle size={16} className="text-error" />,
  warning: <AlertTriangle size={16} className="text-warning" />,
  info: <Info size={16} className="text-info" />,
};

function Toasts() {
  const toasts = useUI((s) => s.toasts);
  const dismiss = useUI((s) => s.dismissToast);
  return (
    <div
      className="pointer-events-none fixed right-4 bottom-4 z-60 flex w-90 flex-col gap-2"
      aria-live="polite"
    >
      {toasts.map((t) => (
        <div
          key={t.id}
          role={t.kind === 'error' ? 'alert' : 'status'}
          className={cx(
            'pointer-events-auto flex animate-slide-up items-start gap-2.5 rounded-lg border bg-elevated p-3 shadow-xl shadow-black/40',
            t.kind === 'error'
              ? 'border-error/40'
              : t.kind === 'warning'
                ? 'border-warning/40'
                : 'border-line-strong',
          )}
        >
          <span className="mt-px shrink-0">{toastIcon[t.kind]}</span>
          <div className="min-w-0 flex-1">
            <div className="text-[13px] font-medium">{t.title}</div>
            {t.message && (
              <div className="selectable mt-0.5 line-clamp-4 text-xs leading-snug wrap-break-word text-muted">
                {t.message}
              </div>
            )}
          </div>
          <button
            className="shrink-0 text-faint hover:text-fg"
            onClick={() => dismiss(t.id)}
            aria-label="Закрыть"
          >
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}

export function Overlays() {
  return (
    <>
      <PromptDialog />
      <ConfirmDialog />
      <SettingsDialog />
      <AboutDialog />
      <Toasts />
    </>
  );
}
