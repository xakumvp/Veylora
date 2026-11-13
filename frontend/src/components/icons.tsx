import {
  AlarmClock,
  Binary,
  Box,
  Braces,
  Calendar,
  CalendarClock,
  Dices,
  FilePenLine,
  FileText,
  Fingerprint,
  FolderSearch,
  GitBranch,
  Globe,
  Hash,
  MessageSquare,
  OctagonX,
  Play,
  Power,
  Regex,
  Repeat,
  ScrollText,
  Send,
  Timer,
  Webhook,
  Workflow,
  type LucideIcon,
  type LucideProps,
} from 'lucide-react';

const byName: Record<string, LucideIcon> = {
  play: Play,
  globe: Globe,
  'git-branch': GitBranch,
  timer: Timer,
  'scroll-text': ScrollText,
  'calendar-clock': CalendarClock,
  repeat: Repeat,
  'alarm-clock': AlarmClock,
  'folder-search': FolderSearch,
  power: Power,
  'octagon-x': OctagonX,
  workflow: Workflow,
  braces: Braces,
  regex: Regex,
  binary: Binary,
  hash: Hash,
  calendar: Calendar,
  fingerprint: Fingerprint,
  dice: Dices,
  send: Send,
  'message-square': MessageSquare,
  'file-text': FileText,
  'file-pen': FilePenLine,
  webhook: Webhook,
};

/** Accent colour per category, used for node icons. */
export const categoryColor: Record<string, string> = {
  Triggers: '#3ecf8e',
  HTTP: '#5aa7f5',
  Logic: '#f5b83d',
  'Data Processing': '#38bdf8',
  Filesystem: '#f97316',
  Notifications: '#ec4899',
  Utilities: '#a78bfa',
  Logging: '#94a3b8',
};

export function NodeIcon({ name, ...props }: { name: string } & LucideProps) {
  const Icon = byName[name] ?? Box;
  return <Icon {...props} />;
}

export function Logo({ size = 24 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <rect width="32" height="32" rx="8" fill="#8b7cf6" />
      <path
        d="M8.5 9.5 16 23l7.5-13.5"
        stroke="#fff"
        strokeWidth="3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="16" cy="23" r="2.4" fill="#fff" />
      <circle cx="8.5" cy="9.5" r="2" fill="#d9d3ff" />
      <circle cx="23.5" cy="9.5" r="2" fill="#d9d3ff" />
    </svg>
  );
}
