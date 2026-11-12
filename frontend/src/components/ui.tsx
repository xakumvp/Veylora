import {
  forwardRef,
  useEffect,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react';
import { X } from 'lucide-react';

export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(' ');
}

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'success';

const variants: Record<Variant, string> = {
  primary: 'bg-accent text-white hover:bg-accent-strong disabled:bg-accent/40',
  secondary: 'bg-panel-2 text-fg border border-line hover:bg-elevated hover:border-line-strong',
  ghost: 'text-muted hover:text-fg hover:bg-elevated',
  danger: 'bg-error/15 text-error border border-error/30 hover:bg-error/25',
  success: 'bg-success/90 text-[#062a1b] hover:bg-success disabled:bg-success/30',
};

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: 'sm' | 'md';
  icon?: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', icon, className, children, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      className={cx(
        'inline-flex shrink-0 items-center justify-center gap-1.5 rounded-md font-medium whitespace-nowrap transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-50',
        size === 'sm' ? 'h-7 px-2.5 text-xs' : 'h-8 px-3 text-[13px]',
        variants[variant],
        className,
      )}
      {...rest}
    >
      {icon}
      {children}
    </button>
  );
});

interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string;
  active?: boolean;
  size?: 'sm' | 'md';
}

export function IconButton({ label, active, size = 'md', className, children, ...rest }: IconButtonProps) {
  return (
    <button
      aria-label={label}
      title={label}
      className={cx(
        'inline-flex shrink-0 items-center justify-center rounded-md text-muted transition-colors duration-150 hover:bg-elevated hover:text-fg disabled:pointer-events-none disabled:opacity-35',
        size === 'sm' ? 'size-6' : 'size-8',
        active && 'bg-elevated text-fg',
        className,
      )}
      {...rest}
    >
      {children}
    </button>
  );
}

const fieldBase =
  'w-full rounded-md border bg-bg px-2.5 text-[13px] text-fg placeholder:text-faint transition-colors duration-150 focus:border-accent focus:outline-none';

export const Input = forwardRef<
  HTMLInputElement,
  InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean; mono?: boolean }
>(function Input({ className, invalid, mono, ...rest }, ref) {
  return (
    <input
      ref={ref}
      spellCheck={false}
      className={cx(
        fieldBase,
        'h-8',
        invalid ? 'border-error/70' : 'border-line',
        mono && 'font-mono text-xs',
        className,
      )}
      {...rest}
    />
  );
});

export function Textarea({
  className,
  invalid,
  ...rest
}: TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }) {
  return (
    <textarea
      spellCheck={false}
      className={cx(
        fieldBase,
        'min-h-20 resize-y py-2 font-mono text-xs leading-relaxed',
        invalid ? 'border-error/70' : 'border-line',
        className,
      )}
      {...rest}
    />
  );
}

export function Select({
  className,
  invalid,
  ...rest
}: SelectHTMLAttributes<HTMLSelectElement> & { invalid?: boolean }) {
  return (
    <select
      className={cx(
        fieldBase,
        'h-8 cursor-pointer pr-7',
        invalid ? 'border-error/70' : 'border-line',
        className,
      )}
      {...rest}
    />
  );
}

export function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label?: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className={cx(
        'relative inline-flex h-4.5 w-8 shrink-0 items-center rounded-full transition-colors duration-150',
        checked ? 'bg-accent' : 'bg-line-strong',
      )}
    >
      <span
        className={cx(
          'inline-block size-3.5 rounded-full bg-white shadow transition-transform duration-150',
          checked ? 'translate-x-4' : 'translate-x-0.5',
        )}
      />
    </button>
  );
}

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="rounded border border-line bg-panel-2 px-1 font-mono text-[10px] text-muted">
      {children}
    </kbd>
  );
}

export function SectionTitle({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex h-7 items-center justify-between">
      <span className="text-[11px] font-semibold tracking-wider text-faint uppercase">{children}</span>
      {action}
    </div>
  );
}

export function Modal({
  title,
  onClose,
  children,
  footer,
  width = 'w-105',
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  width?: string;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);
  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/55 backdrop-blur-[2px]"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={cx(
          'max-w-[92vw] animate-slide-up rounded-xl border border-line bg-panel shadow-2xl shadow-black/50',
          width,
        )}
      >
        <div className="flex h-12 items-center justify-between border-b border-line pr-2 pl-4">
          <h2 className="text-sm font-semibold">{title}</h2>
          <IconButton label="Закрыть" onClick={onClose}>
            <X size={16} />
          </IconButton>
        </div>
        <div className="p-4">{children}</div>
        {footer && <div className="flex justify-end gap-2 border-t border-line px-4 py-3">{footer}</div>}
      </div>
    </div>
  );
}
