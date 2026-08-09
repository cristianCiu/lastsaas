import { describe, expect, it } from 'vitest';
import { normalizeTenantBranding, validateTenantBranding } from './validation';

describe('tenant branding validation', () => {
  it('normalizes safe colors and accepts inheritance defaults', () => {
    expect(normalizeTenantBranding({ primaryColor: ' #AABBCC ', accentColor: '#123456', font: 'humanist' })).toEqual({ primaryColor: '#aabbcc', accentColor: '#123456', font: 'humanist' });
    expect(validateTenantBranding({ primaryColor: '', accentColor: '', font: '' })).toEqual({});
  });

  it('rejects unsafe colors and unknown font tokens', () => {
    expect(validateTenantBranding({ primaryColor: 'red; color:white', accentColor: '#fff', font: 'remote' as never })).toEqual({
      primaryColor: 'Use a six-digit hex color, such as #0ea5e9.',
      accentColor: 'Use a six-digit hex color, such as #a855f7.',
      font: 'Choose one of the supported font styles.',
    });
  });
});
