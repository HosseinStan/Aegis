# Aegis

Aegis is an internal infrastructure-management platform. The first production
application is Aegis API, a read-only network configuration and device
management service for approved Cisco IOS/IOS XE devices.

The implementation lives in [`apps/aegis-api`](apps/aegis-api). The existing
Vue.js office application integrates with it over an authenticated JSON REST
API; browsers never connect directly to managed devices.

See [`apps/aegis-api/README.md`](apps/aegis-api/README.md) for setup and usage.

