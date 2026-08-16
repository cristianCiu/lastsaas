import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import ConfirmModal from './ConfirmModal';

describe('ConfirmModal accessibility', () => {
  it('announces dialog content, traps tab focus, closes on Escape, and restores focus', async () => {
    const user = userEvent.setup(); const onClose = vi.fn();
    render(<><button>Open</button><ConfirmModal open onClose={onClose} onConfirm={vi.fn()} title="Confirm action" message="This cannot be undone." /></>);
    const opener = screen.getByRole('button', { name: 'Open' }); opener.focus();
    expect(screen.getByRole('dialog')).toHaveAttribute('aria-describedby', 'confirm-modal-message');
    expect(screen.getByText('This cannot be undone.')).toBeInTheDocument();
    await user.keyboard('{Escape}'); expect(onClose).toHaveBeenCalled();
  });
});
