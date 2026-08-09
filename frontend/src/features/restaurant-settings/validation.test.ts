import { describe, expect, it } from 'vitest';
import { restaurantSettingsKeys } from './queries';
import { normalizeRestaurantSettings, validateRestaurantSettings } from './validation';

describe('restaurant settings query keys', () => {
  it('isolates settings by tenant', () => {
    expect(restaurantSettingsKeys.detail('tenant-a')).toEqual(['restaurant-settings', 'detail', 'tenant-a']);
    expect(restaurantSettingsKeys.detail('tenant-a')).not.toEqual(restaurantSettingsKeys.detail('tenant-b'));
  });
});

describe('restaurant settings validation', () => {
  it('normalizes a complete settings payload', () => {
    expect(normalizeRestaurantSettings({ currency: ' eur ', language: 'EN_us', defaultTimezone: ' Europe/Berlin ' })).toEqual({
      currency: 'EUR',
      language: 'en-US',
      defaultTimezone: 'Europe/Berlin',
    });
    expect(validateRestaurantSettings({ currency: 'eur', language: 'EN_us', defaultTimezone: 'Europe/Berlin' })).toEqual({});
  });

  it('rejects malformed currency, language, and timezone values', () => {
    expect(validateRestaurantSettings({ currency: 'EU', language: 'english', defaultTimezone: 'Berlin' })).toEqual({
      currency: 'Enter a three-letter currency code, such as EUR.',
      language: 'Enter a practical BCP-47 language tag, such as de or en-US.',
      defaultTimezone: 'Enter a valid IANA timezone, such as Europe/Berlin.',
    });
  });

  it.each(['eng', 'zh-Hans', 'de-DE-extra', 'e', 'english'])('rejects unsupported language form %s', (language) => {
    expect(validateRestaurantSettings({ currency: 'EUR', language, defaultTimezone: 'Europe/Berlin' }).language).toBe(
      'Enter a practical BCP-47 language tag, such as de or en-US.',
    );
  });
});
