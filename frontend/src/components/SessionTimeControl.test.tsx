import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import SessionTimeControl, { formatTime } from './SessionTimeControl';

afterEach(cleanup);

const base = {
  remainingSec: 222,
  isUnlimited: false,
  status: 'running',
  extendAvailable: true,
  viewOnly: false,
  onExtend: () => true,
};

describe('formatTime', () => {
  it('formats mm:ss and clamps at zero', () => {
    expect(formatTime(222)).toBe('3:42');
    expect(formatTime(5)).toBe('0:05');
    expect(formatTime(0)).toBe('0:00');
  });
});

describe('SessionTimeControl', () => {
  it('shows the countdown and the extend action for an owner', () => {
    render(<SessionTimeControl {...base} />);

    expect(screen.getByText('3:42')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Extend session by 5 minutes' })).toBeTruthy();
  });

  it('hides the extend action from view-only watchers', () => {
    render(<SessionTimeControl {...base} viewOnly />);

    expect(screen.getByText('3:42')).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('hides the extend action when no headroom remains', () => {
    render(<SessionTimeControl {...base} extendAvailable={false} />);

    expect(screen.queryByRole('button')).toBeNull();
  });

  it('reads "unlimited" and offers no action for unlimited sessions', () => {
    render(<SessionTimeControl {...base} isUnlimited remainingSec={0} />);

    expect(screen.getByText('unlimited')).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('renders nothing once the session errored', () => {
    const { container } = render(<SessionTimeControl {...base} status="error" />);

    expect(container.firstChild).toBeNull();
  });

  it('disables extend while the session is expired', () => {
    render(<SessionTimeControl {...base} remainingSec={0} />);

    const button = screen.getByRole('button', { name: 'Extend session by 5 minutes' });
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });

  it('confirms a successful extend', async () => {
    const onExtend = vi.fn().mockResolvedValue(true);
    render(<SessionTimeControl {...base} onExtend={onExtend} />);

    fireEvent.click(screen.getByRole('button', { name: 'Extend session by 5 minutes' }));

    await waitFor(() => expect(screen.getByText('ok')).toBeTruthy());
    expect(onExtend).toHaveBeenCalledTimes(1);
  });

  it('does not confirm when the extend request fails', async () => {
    const onExtend = vi.fn().mockResolvedValue(false);
    render(<SessionTimeControl {...base} onExtend={onExtend} />);

    fireEvent.click(screen.getByRole('button', { name: 'Extend session by 5 minutes' }));

    await waitFor(() => expect(onExtend).toHaveBeenCalledTimes(1));
    expect(screen.queryByText('ok')).toBeNull();
    expect(screen.getByText('+5m')).toBeTruthy();
  });
});
