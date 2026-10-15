import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { AudioToggleButton } from './AudioToggleButton';

afterEach(cleanup);

describe('AudioToggleButton', () => {
  it('exposes unmute when muted and mute when live', () => {
    const onToggle = vi.fn();
    const { rerender } = render(<AudioToggleButton muted onToggle={onToggle} />);
    fireEvent.click(screen.getByRole('button', { name: 'Unmute audio' }));
    expect(onToggle).toHaveBeenCalledTimes(1);

    rerender(<AudioToggleButton muted={false} onToggle={onToggle} />);
    expect(screen.getByRole('button', { name: 'Mute audio' }).getAttribute('aria-pressed')).toBe('true');
  });

  it('does not fire when disabled', () => {
    const onToggle = vi.fn();
    render(<AudioToggleButton muted onToggle={onToggle} disabled />);
    fireEvent.click(screen.getByRole('button', { name: 'Unmute audio' }));
    expect(onToggle).not.toHaveBeenCalled();
  });
});
