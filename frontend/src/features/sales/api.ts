import api from '../../api/client';
import type { SalesImportReport, SalesImportRun, UnmappedSaleLine } from './types';
export const salesApi = {
  template: () => api.get<string>('/product/sales/imports/template', { responseType: 'text' }).then(r => r.data),
  dryRun: (input: { content: string; source: string; mapping?: Record<string, string>; idempotencyKey: string }) => api.post<SalesImportReport>('/product/sales/imports/dry-run', input).then(r => r.data),
  apply: (input: { content: string; source: string; mapping?: Record<string, string>; idempotencyKey: string }) => api.post<SalesImportReport>('/product/sales/imports', input).then(r => r.data),
  runs: () => api.get<{ runs: SalesImportRun[] }>('/product/sales/imports').then(r => r.data),
  run: (id: string) => api.get<{ run: SalesImportRun }>(`/product/sales/imports/${id}`).then(r => r.data),
  unmapped: (source?: string) => api.get<{ lines: UnmappedSaleLine[] }>('/product/sales/unmapped', { params: source ? { source } : undefined }).then(r => r.data),
};
