import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { locationsApi } from '../features/locations/api';
import { activeLocationStorageKey, isValidActiveLocation, resolveActiveLocation } from '../features/locations/activeLocation';
import { locationKeys } from '../features/locations/queries';
import type { Location } from '../features/locations/types';

interface ActiveLocationContextType {
  locations: Location[];
  loading: boolean;
  error: unknown;
  activeLocation: Location | null;
  setActiveLocation: (location: Location | null) => void;
}

interface Selection {
  tenantId: string;
  locationId: string;
}

const ActiveLocationContext = createContext<ActiveLocationContextType | null>(null);
const EMPTY_LOCATIONS: Location[] = [];

export function ActiveLocationProvider({ tenantId, enabled, children }: { tenantId: string; enabled: boolean; children: ReactNode }) {
  const [selection, setSelection] = useState<Selection | null>(null);
  const locationsQuery = useQuery({
    queryKey: locationKeys.list(tenantId),
    queryFn: () => locationsApi.list(),
    enabled: enabled && tenantId.length > 0,
  });
  const locations = enabled ? locationsQuery.data?.locations ?? EMPTY_LOCATIONS : EMPTY_LOCATIONS;
  const activeLocation = selection?.tenantId === tenantId
    ? locations.find((location) => location.id === selection.locationId && location.isActive) ?? null
    : null;

  useEffect(() => {
    if (!enabled || !tenantId) {
      setSelection(null);
      return;
    }
    if (!locationsQuery.isSuccess) return;

    const storageKey = activeLocationStorageKey(tenantId);
    const resolved = resolveActiveLocation(locations, localStorage.getItem(storageKey));
    if (resolved) {
      setSelection({ tenantId, locationId: resolved.id });
      localStorage.setItem(storageKey, resolved.id);
    } else {
      setSelection(null);
      localStorage.removeItem(storageKey);
    }
  }, [enabled, locations, locationsQuery.isSuccess, tenantId]);

  const setActiveLocation = useCallback((location: Location | null) => {
    if (!enabled || !tenantId) return;
    const storageKey = activeLocationStorageKey(tenantId);
    if (!location) {
      setSelection(null);
      localStorage.removeItem(storageKey);
      return;
    }
    if (!isValidActiveLocation(locations, location)) return;
    setSelection({ tenantId, locationId: location.id });
    localStorage.setItem(storageKey, location.id);
  }, [enabled, locations, tenantId]);

  return (
    <ActiveLocationContext.Provider value={{
      locations,
      loading: enabled && locationsQuery.isPending,
      error: enabled ? locationsQuery.error : null,
      activeLocation,
      setActiveLocation,
    }}>
      {children}
    </ActiveLocationContext.Provider>
  );
}

export function useActiveLocation() {
  const context = useContext(ActiveLocationContext);
  if (!context) throw new Error('useActiveLocation must be used within an ActiveLocationProvider');
  return context;
}
