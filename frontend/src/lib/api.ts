import axios from 'axios';

import { sessionCreateAuthHeaders } from './createAuth';

export const api = axios.create({
  baseURL: '/',
  headers: { 'Content-Type': 'application/json' },
});

api.interceptors.request.use((config) => {
  if (config.method?.toLowerCase() === 'post' && config.url === '/api/sessions') {
    // AxiosHeaders is a class. Replacing it with a spread object fails the
    // request type and drops its methods.
    for (const [key, value] of Object.entries(sessionCreateAuthHeaders())) {
      config.headers.set(key, value);
    }
  }
  return config;
});

