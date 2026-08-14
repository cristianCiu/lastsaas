import api from '../../api/client';
import type { ExternalProductMapping, Recipe, RecipeComponent, RecipeVersion, RecipeVersionInput } from './types';
export const recipesApi = {
  list: () => api.get<{ recipes: Recipe[] }>('/product/recipes').then(r => r.data),
  create: (input: { code: string; name: string; description?: string }) => api.post<{ recipe: Recipe }>('/product/recipes', input).then(r => r.data),
  update: (id: string, input: { version: number; name?: string; description?: string; isActive?: boolean }) => api.patch<{ recipe: Recipe }>(`/product/recipes/${id}`, input).then(r => r.data),
  versions: (id: string) => api.get<{ recipeVersions: RecipeVersion[] }>(`/product/recipes/${id}/versions`).then(r => r.data),
  createVersion: (id: string, input: RecipeVersionInput) => api.post<{ recipeVersion: RecipeVersion; components: RecipeComponent[] }>(`/product/recipes/${id}/versions`, input).then(r => r.data),
  components: (versionId: string) => api.get<{ components: RecipeComponent[] }>(`/product/recipe-versions/${versionId}/components`).then(r => r.data),
  release: (versionId: string, version: number) => api.post<{ recipeVersion: RecipeVersion }>(`/product/recipe-versions/${versionId}/release`, { version }).then(r => r.data),
};
export const mappingsApi = {
  list: (at?: string) => api.get<{ mappings: ExternalProductMapping[] }>('/product/external-product-mappings', { params: at ? { at } : undefined }).then(r => r.data),
  create: (input: Omit<ExternalProductMapping, 'id'|'isActive'|'version'|'createdAt'|'updatedAt'>) => api.post<{ mapping: ExternalProductMapping }>('/product/external-product-mappings', input).then(r => r.data),
  update: (id: string, input: { version: number; recipeVersionId?: string; effectiveFrom?: string; effectiveTo?: string | null; isActive?: boolean }) => api.patch<{ mapping: ExternalProductMapping }>(`/product/external-product-mappings/${id}`, input).then(r => r.data),
  remove: (id: string) => api.delete(`/product/external-product-mappings/${id}`),
};
