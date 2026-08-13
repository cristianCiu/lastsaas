import type { CreateItemConversionInput } from './types';

const FACTOR = /^[1-9]\d*$/;
export type ConversionValidationErrors = Partial<Record<keyof CreateItemConversionInput, string>>;

export function normalizeConversion(input: CreateItemConversionInput): CreateItemConversionInput {
  return { fromUnitId: input.fromUnitId, numerator: input.numerator.trim(), denominator: input.denominator.trim() };
}

export function validateConversion(input: CreateItemConversionInput): ConversionValidationErrors {
  const value = normalizeConversion(input); const errors: ConversionValidationErrors = {};
  if (!value.fromUnitId) errors.fromUnitId = 'Choose a source unit.';
  for (const field of ['numerator', 'denominator'] as const) {
    const factor = value[field];
    if (!FACTOR.test(factor) || Number(factor) > 1_000_000_000) errors[field] = 'Use a whole number from 1 to 1,000,000,000.';
  }
  return errors;
}

export function conversionPreview(numerator: string, denominator: string, sourceName: string, baseName: string) {
  return `1 ${sourceName} = ${numerator}/${denominator} ${baseName}`;
}
