import { describe, expect, it } from 'vitest';
import { normalizeCategory, validateCategory } from './validation';

describe('category validation', () => {
  it('normalizes category identity', () => {
    expect(normalizeCategory({ code: ' Produce ', name: ' Produce ' })).toEqual({ code: 'produce', name: 'Produce' });
  });

  it('rejects malformed codes and empty names', () => {
    expect(validateCategory({ code: 'Bad Code', name: '' })).toMatchObject({ code: expect.any(String), name: expect.any(String) });
  });
});
