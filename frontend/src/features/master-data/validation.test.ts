import { describe, expect, it } from 'vitest';
import { normalizeUnit, validateUnit } from './validation';

describe('unit validation', () => {
  it('normalizes canonical unit identity', () => {
    expect(normalizeUnit({ code: ' KG ', name: ' Kilogram ', symbol: ' kg ', dimension: 'mass', precision: 3 })).toEqual({ code: 'kg', name: 'Kilogram', symbol: 'kg', dimension: 'mass', precision: 3 });
  });

  it('rejects malformed codes and precision', () => {
    expect(validateUnit({ code: 'Bad Code', name: '', symbol: '', dimension: 'mass', precision: 7 })).toMatchObject({ code: expect.any(String), name: expect.any(String), symbol: expect.any(String), precision: expect.any(String) });
  });
});
