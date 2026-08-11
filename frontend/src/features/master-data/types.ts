export type UnitDimension = 'mass' | 'volume' | 'count';

export interface Unit {
  id: string;
  code: string;
  name: string;
  symbol: string;
  dimension: UnitDimension;
  precision: number;
  isActive: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateUnitInput {
  code: string;
  name: string;
  symbol: string;
  dimension: UnitDimension;
  precision: number;
}

export interface UpdateUnitInput {
  version: number;
  name?: string;
  symbol?: string;
  precision?: number;
  isActive?: boolean;
}
