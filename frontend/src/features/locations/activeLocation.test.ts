import { describe, expect, it } from 'vitest';
import { activeLocationStorageKey, isValidActiveLocation, resolveActiveLocation } from './activeLocation';
import type { Location } from './types';

const location = (id: string, isActive = true): Location => ({
  id,
  version: 1,
  code: id,
  name: id,
  timezone: 'Europe/London',
  isActive,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
});

describe('active location persistence', () => {
  it('uses a separate storage key per tenant', () => {
    expect(activeLocationStorageKey('tenant-a')).toBe('lastsaas_active_location:tenant-a');
    expect(activeLocationStorageKey('tenant-a')).not.toBe(activeLocationStorageKey('tenant-b'));
  });

  it('restores an active saved selection', () => {
    const locations = [location('first'), location('saved')];
    expect(resolveActiveLocation(locations, 'saved')?.id).toBe('saved');
  });

  it('falls back to the first active location for missing or inactive selections', () => {
    const locations = [location('inactive', false), location('first-active'), location('second-active')];
    expect(resolveActiveLocation(locations, 'inactive')?.id).toBe('first-active');
    expect(resolveActiveLocation(locations, 'missing')?.id).toBe('first-active');
  });

  it('returns null when no active location exists', () => {
    expect(resolveActiveLocation([location('inactive', false)], 'inactive')).toBeNull();
  });

  it('only validates active locations from the current tenant list', () => {
    const locations = [location('current'), location('inactive', false)];
    expect(isValidActiveLocation(locations, locations[0])).toBe(true);
    expect(isValidActiveLocation(locations, locations[1])).toBe(false);
    expect(isValidActiveLocation(locations, location('other'))).toBe(false);
  });
});
