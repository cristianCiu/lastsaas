import { describe, expect, it } from 'vitest';
import { locationKeys } from './queries';
import { isValidTimezone, validateLocation } from './validation';

describe('location query keys', () => {
  it('isolates lists by tenant', () => {
    expect(locationKeys.list('tenant-a')).toEqual(['locations', 'list', 'tenant-a']);
    expect(locationKeys.list('tenant-a')).not.toEqual(locationKeys.list('tenant-b'));
  });
});

describe('location validation', () => {
  it('accepts a complete location', () => {
    expect(validateLocation({
      code: 'new-york-1',
      name: 'New York',
      timezone: 'America/New_York',
    })).toEqual({});
  });

  it('rejects non-slug codes and invalid fields', () => {
    expect(validateLocation({ code: 'New_York', name: '  ', timezone: 'Mars/Olympus' })).toEqual({
      code: 'Use a lower-case slug with letters, numbers, and single hyphens.',
      name: 'Name is required.',
      timezone: 'Enter a valid IANA timezone, such as America/New_York.',
    });
  });

  it('checks timezones without relying on supportedValuesOf', () => {
    expect(isValidTimezone('Europe/London')).toBe(true);
    expect(isValidTimezone('Not/A_Zone')).toBe(false);
  });

  it('enforces API length limits', () => {
    expect(validateLocation({
      code: 'a'.repeat(65),
      name: 'a'.repeat(201),
      timezone: 'a'.repeat(101),
    })).toEqual({
      code: 'Code must be 64 characters or fewer.',
      name: 'Name must be 200 characters or fewer.',
      timezone: 'Timezone must be 100 characters or fewer.',
    });
  });
});
