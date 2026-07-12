# Capacity Budget

## Database pools

The runtime validates this upper bound before registering each new tenant connection:

`APP_POD_COUNT * registered_tenant_pools * DB_POOL_MAX_OPEN <= TENANT_DATABASE_CONNECTION_BUDGET`

With defaults, eight registered pools require `3 * 8 * 20 = 480`, below the declared PostgreSQL budget of `500`; the ninth pool is rejected unless capacity is increased. Set `TENANT_DATABASE_CONNECTION_BUDGET_OVERRIDE=true` only with a reviewed database capacity record.

Goravel v1.17 exposes only global `Orm.Fresh()` and no per-connection purge API, so registered pools remain counted for the process lifetime. Restart a drained pod to reclaim pools safely; capacity metrics report the actual registered pool count.

## Casbin

Tenant and platform enforcers are cached per connection with a five-minute TTL and LRU bound. Concurrent cold loads share one loader. Successful RBAC mutations invalidate only the affected platform or tenant key; authorization fails closed when a reload fails.
