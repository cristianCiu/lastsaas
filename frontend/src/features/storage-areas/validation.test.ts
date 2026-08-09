import { describe, expect, it } from 'vitest';
import { storageAreaKeys } from './queries';
import { isStorageAreaType, validateStorageArea } from './validation';

describe('storage area query keys', () => {
  it('isolates lists by principal, tenant, and location', () => {
    expect(storageAreaKeys.list('user-a', 'tenant-a', 'location-a')).toEqual(['storage-areas', 'list', 'user-a', 'tenant-a', 'location-a']);
    expect(storageAreaKeys.list('user-a', 'tenant-a', 'location-a')).not.toEqual(storageAreaKeys.list('user-b', 'tenant-a', 'location-a'));
    expect(storageAreaKeys.list('user-a', 'tenant-a', 'location-a')).not.toEqual(storageAreaKeys.list('user-a', 'tenant-b', 'location-a'));
    expect(storageAreaKeys.list('user-a', 'tenant-a', 'location-a')).not.toEqual(storageAreaKeys.list('user-a', 'tenant-a', 'location-b'));
  });
});

describe('storage area validation', () => {
  it('accepts every API type and a trimmed name', () => {
    for (const type of ['refrigerated', 'frozen', 'bar', 'dry', 'other']) {
      expect(isStorageAreaType(type)).toBe(true);
      expect(validateStorageArea({ name: '  Main store  ', type })).toEqual({});
    }
  });

  it('rejects empty and oversized names and unknown types', () => {
    expect(validateStorageArea({ name: '', type: 'ambient' })).toEqual({
      name: 'Name is required.',
      type: 'Choose a valid storage area type.',
    });
    expect(validateStorageArea({ name: 'a'.repeat(201), type: 'dry' })).toEqual({ name: 'Name must be 200 characters or fewer.' });
  });
});
