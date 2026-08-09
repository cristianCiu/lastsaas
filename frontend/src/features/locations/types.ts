export interface Location {
  id: string;
  version: number;
  code: string;
  name: string;
  timezone: string;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface CreateLocationInput {
  code: string;
  name: string;
  timezone: string;
}

export interface UpdateLocationInput {
  version: number;
  name?: string;
  timezone?: string;
  isActive?: boolean;
}
