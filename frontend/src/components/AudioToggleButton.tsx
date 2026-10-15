import { Volume2, VolumeX } from 'lucide-react';

/**
 * Local playback mute. Does not enable sandbox capture — that stays an
 * owner-only POST /api/sessions/{id}/audio. Share and view-only tabs use
 * this to start AudioContext on a user gesture so Opus can reach the
 * speakers once the host has capture on.
 */
export function AudioToggleButton({
  muted,
  onToggle,
  disabled,
}: {
  muted: boolean;
  onToggle: () => void;
  disabled?: boolean;
}) {
  const label = muted ? 'Unmute audio' : 'Mute audio';
  return (
    <button
      type="button"
      onClick={onToggle}
      disabled={disabled}
      title={label}
      aria-label={label}
      aria-pressed={!muted}
      className={`p-1.5 rounded transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
        muted
          ? 'text-zinc-500 dark:text-gray-400 hover:text-zinc-900 dark:hover:text-white hover:bg-zinc-300 dark:hover:bg-gray-800'
          : 'text-cyan-600 dark:text-cyan-400 hover:bg-zinc-300 dark:hover:bg-gray-800'
      }`}
    >
      {muted ? <VolumeX size={16} /> : <Volume2 size={16} />}
    </button>
  );
}
