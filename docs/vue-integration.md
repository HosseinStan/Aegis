# Vue.js integration

The existing office application should send requests only to Aegis API. In a
reverse-proxy deployment, use the same origin and `/api/v1` as the base path.

```javascript
import axios from "axios";

export const aegis = axios.create({
  baseURL: "/api/v1",
  timeout: 60_000
});

aegis.interceptors.request.use((request) => {
  const token = sessionStorage.getItem("aegis_access_token");
  if (token) request.headers.Authorization = `Bearer ${token}`;
  return request;
});

export async function getRunningConfig(deviceId, refresh = false) {
  const response = await aegis.get(
    `/devices/${encodeURIComponent(deviceId)}/config/running`,
    { params: { refresh } }
  );
  return response.data.data;
}

export async function createRunningBackup(deviceId) {
  const response = await aegis.post(
    `/devices/${encodeURIComponent(deviceId)}/backups`,
    { config_type: "running" }
  );
  return response.data.data;
}
```

Do not render configuration text with `v-html`. Use `v-text`, interpolation, or
the `textContent` DOM property. Display `error.request_id` with user-facing
errors so administrators can correlate the request with server and audit logs.
Buttons should be hidden or disabled using the permissions returned by
`GET /api/v1/auth/me`, but the API remains the authoritative permission check.

For production, replace local login with the organization's JWT/OIDC flow once
that adapter is implemented. Avoid long-lived tokens in `localStorage`; an
HTTP-only same-site session design is preferred for the office application.

