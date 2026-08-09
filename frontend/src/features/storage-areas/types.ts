export const STORAGE_AREA_TYPES = ['refrigerated', 'frozen', 'bar', 'dry', 'other'] as const;

export type StorageAreaType = typeof STORAGE_AREA_TYPES[number];

export interface StorageArea {
  id: string;
  locationId: string;
  name: string;
  type: StorageAreaType;
  isActive: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateStorageAreaInput {
  name: string;
  type: StorageAreaType;
}

export interface UpdateStorageAreaInput {
  version: number;
  name?: string;
  type?: StorageAreaType;
  isActive?: boolean;
}
