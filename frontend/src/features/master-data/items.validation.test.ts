import { describe, expect, it } from 'vitest';
import type { AllergenCode } from './types';
import { normalizeItem, validateItem } from './validation';

const valid = { sku: ' Tomato-Sauce ', name: ' Sauce ', categoryId: 'category', baseUnitId: 'unit', allergens: [] as AllergenCode[], stockable: true };

describe('item validation', () => {
  it('normalizes the immutable lowercase SKU and text fields', () => {
    expect(normalizeItem(valid)).toMatchObject({ sku: 'tomato-sauce', name: 'Sauce' });
  });

  it('enforces required references and bounded shelf life', () => {
    expect(validateItem({ ...valid, categoryId: '', shelfLifeDays: 36501 })).toMatchObject({ categoryId: expect.any(String), shelfLifeDays: expect.any(String) });
  });
});
