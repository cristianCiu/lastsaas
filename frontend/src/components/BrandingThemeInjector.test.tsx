import { render, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import BrandingThemeInjector from './BrandingThemeInjector';

const mocks = vi.hoisted(() => ({
  effectiveBranding: { primaryColor: '#123456', accentColor: '#abcdef', font: 'serif' },
}));

vi.mock('../contexts/BrandingContext', () => ({
  useBranding: () => ({ branding: { primaryColor: '#ff0000', fontFamily: 'Inter', headingFont: '', appName: 'Platform', analyticsSnippet: '', customCss: '', headHtml: '', faviconUrl: '' }, loaded: true }),
}));
vi.mock('../contexts/TenantBrandingContext', () => ({ useTenantBranding: () => ({ effectiveBranding: mocks.effectiveBranding }) }));

describe('BrandingThemeInjector location resolution', () => {
  afterEach(() => document.documentElement.removeAttribute('style'));

  it('applies effective active-location tokens in a workspace route', async () => {
    render(<MemoryRouter initialEntries={['/dashboard']}><BrandingThemeInjector /></MemoryRouter>);
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--color-accent-purple')).toBe('#abcdef'));
    expect(document.documentElement.style.getPropertyValue('--font-sans')).toContain('Georgia');
    expect(document.documentElement.style.getPropertyValue('--color-primary-500')).not.toBe('');
  });
});
