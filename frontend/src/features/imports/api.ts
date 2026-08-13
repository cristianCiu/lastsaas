import api from '../../api/client';
import type { ImportReport, ImportRequest, ImportRun, ImportTarget } from './types';

export const importsApi = {
  template: (target: ImportTarget) => api.get<string>(`/product/import-templates/${target}`, { responseType: 'text' }).then((r) => r.data),
  dryRun: (input: Omit<ImportRequest, 'idempotencyKey'>) => api.post<ImportReport>('/product/imports/dry-run', input).then((r) => r.data),
  apply: (input: ImportRequest) => api.post<ImportReport>('/product/imports', input).then((r) => r.data),
  list: () => api.get<{ runs: ImportRun[] }>('/product/imports').then((r) => r.data),
  get: (id: string) => api.get<{ run: ImportRun }>(`/product/imports/${id}`).then((r) => r.data),
};
