import { describe, it, expect } from 'vitest';
import axios, { AxiosError } from 'axios';
import { getErrorMessage } from './errors';

describe('getErrorMessage', () => {
  it('uses a generic status message for unstructured API errors', () => {
    const err = new AxiosError('Request failed', '404', undefined, undefined, {
      status: 404,
      statusText: 'Not Found',
      headers: {},
      config: { headers: new axios.AxiosHeaders() },
      data: { error: 'User not found' },
    });
    expect(getErrorMessage(err)).toBe('The requested resource was not found.');
  });

  it('does not expose raw string response data', () => {
    const err = new AxiosError('Request failed', '400', undefined, undefined, {
      status: 400,
      statusText: 'Bad Request',
      headers: {},
      config: { headers: new axios.AxiosHeaders() },
      data: 'Raw error string',
    });
    expect(getErrorMessage(err)).toBe('Invalid request. Please check your input.');
  });

  it('uses user-safe messages from structured API errors', () => {
    const err = new AxiosError('Request failed', '404', undefined, undefined, {
      status: 404,
      statusText: 'Not Found',
      headers: {},
      config: { headers: new axios.AxiosHeaders() },
      data: { code: 'USER_NOT_FOUND', error: 'User not found' },
    });
    expect(getErrorMessage(err)).toBe('User not found');
  });

  it('handles 429 rate limit without error field', () => {
    const err = new AxiosError('Too Many Requests', '429', undefined, undefined, {
      status: 429,
      statusText: 'Too Many Requests',
      headers: {},
      config: { headers: new axios.AxiosHeaders() },
      data: {},
    });
    expect(getErrorMessage(err)).toBe('Too many requests. Please try again later.');
  });

  it('returns message from Error instances', () => {
    expect(getErrorMessage(new Error('Something broke'))).toBe('Something broke');
  });

  it('returns fallback for unknown types', () => {
    expect(getErrorMessage(null)).toBe('An unexpected error occurred');
    expect(getErrorMessage(42)).toBe('An unexpected error occurred');
    expect(getErrorMessage(undefined)).toBe('An unexpected error occurred');
  });
});
