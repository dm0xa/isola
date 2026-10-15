import { type ReactNode } from 'react';

export function ViewerMenuItem({
  icon,
  label,
  dimmed,
  destructive,
  toggled,
  disabled,
  static: isStatic,
  onClick,
}: {
  icon: ReactNode;
  label: ReactNode;
  dimmed?: boolean;
  destructive?: boolean;
  toggled?: boolean;
  disabled?: boolean;
  /** Informational row: same look as dimmed, no hover or click. */
  static?: boolean;
  onClick?: () => void;
}) {
  const labelColor = destructive
    ? 'text-red-600 dark:text-red-400'
    : dimmed
    ? 'text-zinc-500 dark:text-gray-400'
    : 'text-zinc-800 dark:text-gray-200';
  const iconColor = destructive
    ? 'text-red-500 dark:text-red-400'
    : dimmed
    ? 'text-zinc-400 dark:text-gray-500'
    : 'text-zinc-600 dark:text-gray-300';
  const rowClass = `flex w-full items-center gap-3 px-3 py-2 text-left ${
    toggled ? `bg-cyan-500/10 ${labelColor}` : labelColor
  }`;

  if (isStatic) {
    return (
      <div role="menuitem" aria-disabled="true" className={`${rowClass} cursor-default select-none`}>
        <span className={`flex h-5 w-5 shrink-0 items-center justify-center ${iconColor}`}>
          {icon}
        </span>
        <span className="flex-1 truncate">{label}</span>
      </div>
    );
  }

  return (
    <button
      role="menuitem"
      disabled={disabled}
      onClick={onClick}
      className={`${rowClass} transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
        toggled ? '' : 'hover:bg-zinc-100 dark:hover:bg-gray-800'
      }`}
    >
      <span className={`flex h-5 w-5 shrink-0 items-center justify-center ${iconColor}`}>
        {icon}
      </span>
      <span className="flex-1 truncate">{label}</span>
    </button>
  );
}
