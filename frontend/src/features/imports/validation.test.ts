import { describe, expect, it } from 'vitest';
import { parseCsv, validateCsv } from './validation';

describe('CSV import validation', () => {
  it('detects comma and semicolon headers without persisting file data', () => {
    expect(parseCsv('code,name\nkg,Kilogram').headers).toEqual(['code', 'name']);
    expect(parseCsv('code;name\nkg;Kilogram').delimiter).toBe(';');
  });

  it('supports escaped quotes and embedded delimiters/newlines', () => {
    const parsed = parseCsv('name,description\n"Cafe, Inc.","Line one\nLine ""two"""');
    expect(parsed.rows).toEqual([['Cafe, Inc.', 'Line one\nLine "two"']]);
  });

  it('enforces the import size and row limits', () => {
    expect(validateCsv('code,name\nkg,Kilogram', 'units')).toEqual([]);
    expect(validateCsv('code,name\n', 'units')).toEqual([]);
  });

  it('uses canonical reference headers for item and supplier-term imports', () => {
    expect(parseCsv('categoryCode,baseUnitCode\nproduce,kg').headers).toEqual(['categoryCode', 'baseUnitCode']);
    expect(parseCsv('supplierCode,itemSKU\nacme,tomato-sauce').headers).toEqual(['supplierCode', 'itemSKU']);
  });
});
