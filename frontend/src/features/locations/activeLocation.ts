import type { Location } from './types';

const ACTIVE_LOCATION_PREFIX = 'lastsaas_active_location';

export function activeLocationStorageKey(tenantId: string): string {
  return `${ACTIVE_LOCATION_PREFIX}:${tenantId}`;
}

export function resolveActiveLocation(locations: Location[], savedLocationId: string | null): Location | null {
  const activeLocations = locations.filter((location) => location.isActive);
  return activeLocations.find((location) => location.id === savedLocationId) ?? activeLocations[0] ?? null;
}

export function isValidActiveLocation(locations: Location[], location: Location): boolean {
  return location.isActive && locations.some((candidate) => candidate.id === location.id && candidate.isActive);
}
