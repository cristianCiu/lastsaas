import type { CreateUnitInput } from './types';

export type UnitValidationErrors = Partial<Record<keyof CreateUnitInput, string>>;
const CODE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export function normalizeUnit(input: CreateUnitInput): CreateUnitInput {
  return { ...input, code: input.code.trim().toLowerCase(), name: input.name.trim(), symbol: input.symbol.trim() };
}

export function validateUnit(input: CreateUnitInput): UnitValidationErrors {
  const value = normalizeUnit(input);
  const errors: UnitValidationErrors = {};
  if (!value.code || value.code.length > 32 || !CODE.test(value.code)) errors.code = 'Use a lower-case code up to 32 characters.';
  if (!value.name || value.name.length > 100) errors.name = 'Name is required and must be at most 100 characters.';
  if (!value.symbol || value.symbol.length > 16) errors.symbol = 'Symbol is required and must be at most 16 characters.';
  if (!['mass', 'volume', 'count'].includes(value.dimension)) errors.dimension = 'Choose a supported dimension.';
  if (!Number.isInteger(value.precision) || value.precision < 0 || value.precision > 6) errors.precision = 'Precision must be a whole number from 0 to 6.';
  return errors;
}
