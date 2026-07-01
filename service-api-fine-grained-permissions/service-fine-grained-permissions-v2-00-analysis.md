# Fine-Grained Service Permissions on Tsuru Service API V2

| Field      | Value                                             |
|------------|---------------------------------------------------|
| **Status** | Draft / analysis                                  |
| **Scope**  | Backport of V3 action-based authorization to V2   |
| **Source** | `tsuru-service-api-v3.md`, `example-acl-manifest.yaml` |

## Goal

Bring the V3 RFC's **fine-grained, action-based authorization** to the *current*
Service API V2 **without** re-implementing the entity/operation model. Concretely:
let a service provider ship a manifest that maps `Method + Path` to a named
**action**, and have tsuru enforce a per-action permission check on the existing
proxy routes — instead of the single coarse `service-instance.update.proxy`
check used today.

This is explicitly the *first* of the two V3 implementation steps
(RFC §Implementation Plan): "Implement the actions and authorization based on the
path matching with the operations". It intentionally leaves entity management,
status tracking, and async operations for later.

## Current State in V2 (what exists today)

### Proxy entry points

All provider-specific traffic reaches the backend through a handful of proxy
handlers (`tsuru/api/server.go`):

| Route | Handler | Auth check today |
|-------|---------|------------------|
| `/services/{service}/proxy/{instance}` | `serviceInstanceProxy` | `PermServiceInstanceUpdateProxy` |
| `/services/{service}/resources/{instance}/{path:.*}` | `serviceInstanceProxyV2` | `PermServiceInstanceUpdateProxy` |
| `/services/proxy/service/{service}` | `serviceProxy` | `PermServiceUpdateProxy` |
| `/services/{service}/authenticated-resources/{path:.*}` | `serviceAuthenticatedResourcesProxy` | none (authenticated passthrough) |

Key observations:

- The **method and callback/`:path` are already available** in the handler
  (`r.Method`, `r.URL.Query().Get("callback")` or the `:path` catch-all). This is
  exactly the `Method + Path` pair the manifest needs to match.
- Every non-GET/HEAD proxy call already creates an `event.Event` keyed by a
  permission `Kind`. Actions map naturally onto this existing event machinery.
- The check is **coarse**: any holder of `service-instance.update.proxy` on the
  instance/team can call *any* path with *any* method.

### Permission model (`tsuru/permission`)

- Permissions are **hierarchical dotted schemes** (e.g.
  `service-instance.update.proxy`) with declared **context types**
  (`CtxServiceInstance`, `CtxTeam`, `CtxService`, `CtxGlobal`, ...).
- `permission.Check(ctx, token, scheme, contexts...)` walks the parent chain;
  a `CtxGlobal` grant or a matching `(ctxType, value)` grant authorizes.
- The registry (`permission/permlist.go` → generated `permitems.go`) is built
  **statically at compile time** via `PermissionRegistry.addWithCtx(...)`.
  `SafeGet(name)` returns an error for unregistered permissions.
- Roles reference permission schemes **by name**; a permission must resolve in
  the registry to be assignable to a role.

### Service model (`tsuru/service`)

- `service.Service` (Mongo `_id` = name) holds `Endpoint map[string]string`,
  `Username/Password`, `Teams`, `OwnerTeams`, `IsMultiCluster`, `Encoding`.
  **There is no manifest field today** — this is the main storage gap.
- `endpointClient.Proxy` (`service/endpoint.go`) is a plain
  `httputil.ReverseProxy`; it rebuilds the URL from `endpoint + path + query`.
  It does **not** inspect method/path for authorization.
- Providers already expose discovery-style endpoints (`GET /resources/plans`,
  instance `Info`), so a **manifest fetch endpoint** fits the existing pattern.

## The Central Design Tension: static registry vs. per-service actions

The manifest's `action` strings (e.g. `acl.rules.sync`, `acl.instances.get-info`)
are **provider-defined and dynamic**, but tsuru's permission registry is
**static and compile-time**. Roles can only be granted permissions that exist in
the registry. This is the single most important decision to resolve. Options:

1. **Generic proxy sub-permissions (recommended for step 1).**
   Register a small fixed set of coarse verbs under the existing service-instance
   tree, e.g. `service-instance.update.proxy.<verb>` where `<verb>` is derived
   from the manifest action's category (read / write / bind / admin). The manifest
   maps `Method+Path → action → verb`, and tsuru checks the verb permission with
   `CtxServiceInstance`/`CtxTeam`. Backwards compatible: a holder of the parent
   `service-instance.update.proxy` still passes (parent-chain check), so existing
   roles keep working.

2. **Dynamic registration from manifests.**
   On manifest upload, register `service.<service>.<action>` schemes into the
   registry so they become role-assignable. More faithful to V3, but the registry
   was not designed for runtime mutation (thread-safety, persistence across
   restarts, role validation, removal on manifest change all need work).

3. **Manifest-driven check outside the registry.**
   Keep actions purely in the manifest and evaluate them against a per-team/per-
   instance action-grant list stored alongside the service, bypassing the global
   role system. Most flexible, but forks the authorization model and loses the
   unified role UX.

Recommendation: **start with option 1** (static coarse verbs mapped from manifest
actions) to ship value quickly and stay compatible, and record option 2 as the
migration target aligned with full V3.

## Proposed V2 Implementation (step 1)

### 1. Manifest ingestion & storage

- Reuse the V3 manifest shape but only consume the fields needed for auth:
  per-operation `path`, `method`, `action`, and (optionally) `scope`. Entity
  nesting can be flattened into effective path patterns.
- Add a `Manifest` (or `Operations []OperationRule`) field to `service.Service`,
  persisted in Mongo. Populate it either:
  - via an admin upload endpoint (`PUT /services/{service}/manifest`), or
  - by fetching from the provider (new `GET /resources/manifest`, mirroring
    `GET /resources/plans`), cached on the `Service` document.
- Validate on ingest: unique operation names, valid method enum, non-empty
  action, well-formed path patterns.

### 2. Method+Path → action matching

- Build a matcher from the manifest: an ordered list of
  `(method, pathPattern) → action`. Path patterns must support the entity-id and
  parent-id path parameters (`{instanceId}`, `{ruleId}`, ...). Use tsuru's
  existing router primitives or `path.Match`/a small trie; prefer longest/most-
  specific match, deterministic ordering.
- The proxied path in V2 is the `callback` query param (`serviceProxy`,
  `serviceInstanceProxy`) or the `:path` catch-all (`serviceInstanceProxyV2`).
  Normalize both into a single "provider path" before matching.
- Define fallback behavior for **unmatched** `Method+Path`:
  - default-deny (safer, but risks breaking existing free-form proxy usage), or
  - default to the legacy coarse `service-instance.update.proxy` (compatible).
  Make this configurable per service (e.g. `strictActions: bool` in the manifest)
  and default to compatible mode initially.

### 3. Authorization enforcement

- In each proxy handler, after resolving service/instance and the provider path:
  1. Match `Method+Path` → `action` via the manifest matcher.
  2. Map `action` → permission scheme (option 1: coarse verb; option 2: dynamic
     `service.<svc>.<action>`).
  3. `permission.Check(ctx, t, scheme, contextsForServiceInstance(...)...)`.
  4. On failure return `permission.ErrUnauthorized`; on unmatched apply the
     configured fallback.
- Keep the parent-chain semantics so `service-instance.update.proxy` and global
  grants continue to authorize (no breakage for current roles).

### 4. Events / auditing

- Reuse the existing `event.New` block in the proxy handlers, but set `Kind` to
  the resolved action (and add the action/manifest-operation name to
  `CustomData`). This gives per-action audit trails "for free" and aligns with
  the V3 goal of mapping operations to tsuru events.

## Requirements Summary

Functional:

- FR1 — A service MAY register a manifest mapping `Method + Path → action`.
- FR2 — Tsuru MUST resolve the action for each proxied request from method+path.
- FR3 — Tsuru MUST perform a permission check for the resolved action using the
  existing contextual model (service-instance / team / global).
- FR4 — Unmatched requests MUST follow a well-defined, per-service fallback
  (compatible-by-default; strict-deny opt-in).
- FR5 — Existing roles/tokens holding `service-instance.update.proxy` (or a
  parent/global) MUST continue to work (backwards compatibility).
- FR6 — Each authorized write action SHOULD emit a tsuru event keyed by the
  action for auditing.

Non-functional / constraints:

- NFR1 — No new entity persistence, status tracking, or async flows (out of
  scope for step 1 per the RFC plan).
- NFR2 — Manifest validation MUST be shallow (tsuru does not validate provider
  payloads), consistent with the RFC Validation Model.
- NFR3 — Matching MUST be deterministic and O(operations) per request; cache the
  compiled matcher on the `Service`.
- NFR4 — The design MUST leave a clear migration path to full V3 dynamic actions
  (option 2) without another breaking change.

## Open Questions

- Registry strategy: coarse static verbs (option 1) vs. dynamic per-service
  action registration (option 2) — which do we commit to for the first release?
- How are provider actions surfaced to role administrators (CLI/UI) if they are
  dynamic and not in the static registry?
- Default fallback for unmatched paths: deny vs. legacy-proxy — per service or
  global config?
- Manifest source of truth: provider-served (`/resources/manifest`) vs.
  tsuru-side admin upload vs. both with caching + refresh.
- Path pattern semantics for parented entities (V3 caps nesting at one level and
  flattens deeper relations via query params) — how much of that do we honor in
  the V2 matcher?
- Do we need a `CtxService` (provider-scoped) action check for the
  `serviceProxy` route in addition to the instance-scoped routes?
