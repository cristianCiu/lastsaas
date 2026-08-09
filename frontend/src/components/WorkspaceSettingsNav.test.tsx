import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import WorkspaceSettingsNav from './WorkspaceSettingsNav';

describe('WorkspaceSettingsNav', () => {
  it('makes every settings route reachable and keeps account matching exact', () => {
    render(<MemoryRouter initialEntries={['/settings/restaurant']}><WorkspaceSettingsNav /></MemoryRouter>);

    expect(screen.getByRole('link', { name: 'Account' })).toHaveAttribute('href', '/settings');
    expect(screen.getByRole('link', { name: 'Restaurant' })).toHaveAttribute('href', '/settings/restaurant');
    expect(screen.getByRole('link', { name: 'Branding' })).toHaveAttribute('href', '/settings/branding');
    expect(screen.getByRole('link', { name: 'Locations' })).toHaveAttribute('href', '/settings/locations');
    expect(screen.getByRole('link', { name: 'Storage areas' })).toHaveAttribute('href', '/settings/storage-areas');
    expect(screen.getByRole('link', { name: 'Account' })).not.toHaveAttribute('aria-current');
    expect(screen.getByRole('link', { name: 'Restaurant' })).toHaveAttribute('aria-current', 'page');
  });
});
