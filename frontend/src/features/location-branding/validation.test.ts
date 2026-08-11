import { describe, expect, it } from 'vitest';
import { normalizeLocationBranding, validateLocationBranding } from './validation';

describe('location branding validation', () => {
  it('normalizes safe override fields while preserving inheritance blanks', () => {
    expect(normalizeLocationBranding({ displayName: ' Flagship ', primaryColor: ' #AABBCC ', accentColor: '', font: '' })).toEqual({
      displayName: 'Flagship', primaryColor: '#aabbcc', accentColor: '', font: '',
    });
  });

  it('rejects malformed colors and overlong display names', () => {
    const errors = validateLocationBranding({ displayName: 'x'.repeat(201), primaryColor: 'red', accentColor: '#123', font: '' });
    expect(errors).toMatchObject({ displayName: expect.any(String), primaryColor: expect.any(String), accentColor: expect.any(String) });
  });
});
