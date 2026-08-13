import { describe, expect, it } from 'vitest';
import { conversionPreview, normalizeConversion, validateConversion } from './conversions.validation';

describe('item conversion validation', () => {
  it('normalizes factors and creates a readable exact preview', () => {
    expect(normalizeConversion({ fromUnitId: 'box', numerator: ' 12 ', denominator: ' 1 ' })).toEqual({ fromUnitId: 'box', numerator: '12', denominator: '1' });
    expect(conversionPreview('12', '1', 'box', 'each')).toBe('1 box = 12/1 each');
  });

  it('requires positive bounded whole-number factors', () => {
    expect(validateConversion({ fromUnitId: '', numerator: '0', denominator: '1000000001' })).toMatchObject({ fromUnitId: expect.any(String), numerator: expect.any(String), denominator: expect.any(String) });
  });
});
