# kronos-gateway-service

[简体中文](./README.md) | **English**

The gateway: the only process that speaks HTTP. It aggregates the HTTP APIs of downstream gRPC services and asks the authorization service before every request.

**It holds no business code.** The whole repository is four Go files, and the only logic in it is the authorization middleware.

## Why it is this short

Downstream HTTP routes are not hand-written. `AutoRegisterWithHealthCheck` walks the `grpc.clients` entries in the config, connects to each downstream, reads its service definitions and `google.api.http` annotations through **gRPC server reflection**, and builds the matching HTTP routes **inside the gateway process**.

So:

- Adding a downstream service is one entry under `grpc.clients` — **no Go change** and no `import` of its pb package
- A new endpoint downstream is a proto annotation there, and the gateway has it after a restart

Two premises:

| Premise | Symptom when unmet |
|---|---|
| Downstream must enable `enable-reflection` | **Every endpoint of that service 404s while the service is plainly alive** |
| An RPC must carry a `google.api.http` annotation | That RPC has no HTTP entrance |

The second is a feature: `AuthService.Authorities` deliberately omits the annotation and therefore never gets an HTTP route. Otherwise anyone who could reach the gateway could call authorization directly, and its response carries the caller's identity.

A discovery summary is logged at startup. **Read that line** — a misspelled downstream name, disabled reflection and a downstream that has not started yet all leave the gateway running with some routes missing rather than failing. Without it, an investigation starts from a 404 and works its way to network policy.

## The four templates

| Template | Role | gRPC | Own HTTP | Business logic |
|---|---|---|---|---|
| `kronos-service-template` | Business service with a database | yes | yes | yes |
| `kronos-key-service-template` | Infrastructure: keys and ids | yes | no | yes |
| `kronos-access-control-template` | Authorization | yes | no | yes |
| **this one** | The HTTP face | no | yes | no |

The request path:

```
Browser ──HTTP──▶ Gateway ──Authorities(path, method)──▶ authorization service
                    │                                      │
                    │◀──────── allowed + Payload ──────────┘
                    │
                    └──gRPC (payload in headers)──▶ business service / key-service
```

## The authorization middleware

`bootstrap/middleware.go`, the gateway's only business logic.

1. Take the authorization service's connection from the pool — never dial here, since pooled connections already carry health checking and propagation interceptors
2. Call `Authorities(path, method)` with the request's metadata forwarded wholesale; the token travels in the `authorization` header, because the contract deliberately has no token field
3. On a denial, answer 401 or 403 according to `token_valid`
4. On success, inject the `Payload` into the HTTP headers and the context and continue

### 401 or 403

| `token_valid` | HTTP | Meaning | What the client should do |
|---|---|---|---|
| `false` | 401 | Unauthenticated: missing, expired or forged | Log in again |
| `true` | 403 | Authenticated but insufficiently authorized | Logging in again changes nothing |

Conflating them sends the frontend back to the login page on every permission error and traps the user in a login loop.

### An unavailable authorization service means 500, never a pass-through

Passing through would make every endpoint public for however long the outage lasts. A 401 is wrong too: it misleads the client into logging in again, which cannot help, and only adds load to the login endpoint during an incident.

### Payload injection is the architecture's crux

**Downstream services do not re-verify the token**; they read identity from the header. Whatever is injected here is believed.

Verifying once is enough. Verifying twice means two implementations that must agree forever and eventually will not, where "gateway allows, downstream rejects" is the good outcome and the reverse is an incident.

**The premise: downstream services must never be exposed directly.** Anything that can reach them can forge any user. That is enforced by network policy at deployment time, not by code.

Both the header and the context are written: the header serves downstream across the process boundary, the context serves this process's later middleware and logs. Header only would leave the gateway's own access log without the caller's identity.

The full payload is Base64-encoded, because a nickname may hold non-ASCII characters while gRPC metadata values must be ASCII, and a non-ASCII value fails the whole request at metadata validation.

### One gateway serves one domain

`gatewayDomain` is fixed at compile time from a kopy variable rather than read from config: config is hot-reloadable, while which side a gateway serves is a deployment decision that must not change at runtime.

The middleware verifies that the domain returned by authorization matches. **That check matters more than it looks**: an ops gateway and a tenant gateway usually share one authorization service, and without it a tenant user's token presented to the ops gateway is accepted — the token really is valid — and that user reaches ops endpoints.

Serving both sides means two deployments with their own resources rather than routing by path inside one process: a routing rule gets changed, and getting it wrong is a cross-domain privilege escalation whose symptom is endpoints becoming reachable, with no error.

## The most dangerous misconfiguration here

The middleware must be attached **before** `RebuildHTTPGateway`. Middleware is woven into the mux during the rebuild, so attaching it afterwards silently does nothing and leaves every endpoint credential-free with everything apparently working — and no error anywhere.

The order in `bootstrap/server.go` is correct; take care not to swap those two lines.

## Quick start

```bash
kopy new https://github.com/kamalyes/kronos-gateway-template -o ./kronos-gateway-service
```

The gateway itself has no external dependencies, since it touches no storage, but it needs downstream services alive to have routes to build. A full run:

```bash
# 1. Authorization (a stub, no external dependencies)
cd kronos-access-control-service && go run .     # :9390

# 2. Keys and ids (needs Redis)
docker run -d -p 6379:6379 redis:7-alpine
cd kronos-key-service && go run .                # :9290

# 3. Business service (needs MySQL and Redis)
cd kronos-my-service && go run .                 # :9190

# 4. The gateway
cd kronos-gateway-service && go run .                # :8490
```

Then:

```bash
# An allow-listed path
curl localhost:8490/health

# A business endpoint with no token: 401
curl -i localhost:8490/v1/products

# With any token, since the stub allows everything: forwarded to the business service
curl -H 'authorization: Bearer whatever' localhost:8490/v1/products

# A key endpoint: key-service has no HTTP entrance of its own, so this route exists only here
curl -X POST -H 'authorization: Bearer whatever' \
  -H 'content-type: application/json' \
  -d '{"scenario":"KEY_SCENARIO_LOGIN"}' \
  localhost:8490/v1/temp-keys/issue
```

That last one is the most instructive part of this architecture: `key-service` deliberately registers no HTTP handler, so the **only entrance to the key endpoints is the gateway** and the authorization middleware is unavoidable. Even if someone exposes key-service's HTTP port, there is no key endpoint on it.

## Service names must match in three places

The three downstream names in `constants/service_name.go` must match exactly:

1. The keys under `grpc.clients` in `resources/kronos-gateway-service-dev.yaml`
2. Each service's own `constants.ServiceName`
3. Here

The pool is indexed by service name. A mismatch makes every gateway request report a missing authorization connection while that service is plainly alive.

## Layout

```
kronos-gateway-service/
├── main.go
├── bootstrap/
│   ├── server.go       Startup; AutoRegister runs before the middleware is attached
│   ├── middleware.go   The authorization middleware, the only business logic here
│   ├── domain.go       The domain this gateway serves, fixed at compile time
│   └── logger.go       Log context fields
├── constants/
│   └── service_name.go This service's name plus the three downstream names
├── locales/            Localized messages for authorization failures
└── resources/
    └── kronos-gateway-service-dev.yaml   grpc.clients is the only declaration of what is proxied
```
