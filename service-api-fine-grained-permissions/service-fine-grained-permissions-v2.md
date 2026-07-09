# Service Fine-Grained Permissions on Tsuru Service API V2 — Implementation Spec

| Field       | Value                                                        |
|-------------|--------------------------------------------------------------|
| **Status**  | **Final — source of truth for implementation**               |
| **Scope**   | Backport V3 action-based authorization to the current V2 API |
| **Design**  | Dual permission registry + dual-field role storage (option 2b) |
| **Audience**| An engineer/agent implementing this in the tsuru repo        |
| **Rationale docs** | `service-fine-grained-permissions-v2-00-analysis.md`, `...-option2-dynamic-registration.md`, `...-option2b-dual-registry.md` |

> This document is self-contained. It consolidates all locked decisions and is the
> authoritative spec. The rationale docs above record how we got here and may be
> read for background, but this file governs the implementation.

---

## 1. Goal

Give service providers **fine-grained, per-action authorization** on the existing
tsuru Service API V2 **without** re-implementing the V3 entity/operation model.

Concretely: a provider ships a manifest that maps `Method + Path → action`. Tsuru
enforces a permission check for the resolved **action** on the existing service
proxy routes, replacing today's single coarse `service-instance.update.proxy`
check. Provider actions become first-class, role-grantable tsuru permissions.

This is **step 1** of the V3 plan (action-based authorization only). It explicitly
does **not** implement entity persistence, status tracking, or async operations.

## 2. Design in one paragraph

Add a **parallel, self-contained dynamic authorization subsystem** that only
activates for service proxy calls on feature-enabled services. It consists of (a) a
**second, dynamic permission registry** separate from the static one, mutable,
thread-safe, and evaluated by scheme **name**; (b) a **second role grant field**
`Role.DynamicSchemeNames` so dynamic grants never flow through the static
resolution chain; and (c) a **non-optional extension** of the `Token` interface
with `DynamicPermissions(ctx)`. The static registry, static grant list, and the
entire `expandRolePermissions → PermissionsFor → filterValidSchemes →
permission.Check` hot path stay **byte-for-byte unchanged**. Proxy handlers pick
the dynamic path only when the request is a proxy call **and** the target service
has the feature enabled.

## 3. Personas (authorization subjects)

- **Service provider / owner** — `Service.OwnerTeams`. Registers the service and
  its manifest; runs the backend tsuru proxies to. Not the day-to-day caller.
- **Instance owner / consumer** — `ServiceInstance.Teams`. App teams that create
  instances and **actually invoke proxy operations**. Proxy authorization uses
  `contextsForServiceInstance` → these teams. This set is **dynamic** and grows as
  instances are created.
- **Tsuru admin** — holds `role.*` permissions; assigns `service-action.*` grants
  to teams.

Implication used throughout: we cannot know the eventual instance-owner teams at
manifest-registration time, so onboarding is handled by a migration bridge
(`legacyCompat`), not by auto-provisioning roles.

---

## 4. Current V2 state (relevant code facts)

All paths are in the tsuru repo (`github.com/tsuru/tsuru`).

### 4.1 Proxy entry points (`api/server.go:245-248`)

| Route | Handler | Today's auth check |
|-------|---------|--------------------|
| `/services/{service}/proxy/{instance}` | `serviceInstanceProxy` (`api/service_instance.go:699`) | `PermServiceInstanceUpdateProxy` |
| `/services/{service}/resources/{instance}/{path:.*}` | `serviceInstanceProxyV2` (`api/service_instance.go:743`) | `PermServiceInstanceUpdateProxy` |
| `/services/proxy/service/{service}` | `serviceProxy` (`api/service.go:291`) | `PermServiceUpdateProxy` |
| `/services/{service}/authenticated-resources/{path:.*}` | `serviceAuthenticatedResourcesProxy` (`api/service.go:333`) | none (passthrough) |

The proxied provider path is available in the handler: the `callback` query param
(`serviceInstanceProxy`, `serviceProxy`) or the `:path` catch-all
(`serviceInstanceProxyV2`). `r.Method` gives the method. This `Method + Path` pair
is exactly the manifest key.

### 4.2 Permission model (`permission/`)

- Hierarchical dotted schemes with context types; static registry built once at
  init in `permission/permlist.go`. **No locking**; assumes read-only after boot.
- `permission.Check(ctx, token, scheme, contexts...)` → `CheckFromPermList`
  (`permission/permission.go:86`) walks the parent chain. Parent matching uses
  **pointer identity** (`PermissionScheme.IsParent`, `types/permission/permission.go:77`,
  via `reflect.ValueOf(...).Pointer()`).
- Roles persist only `SchemeNames []string`; resolved lazily. `AddPermissions`
  (`permission/role.go:186`) rejects names not in the static registry.
  `filterValidSchemes` (`permission/role.go:245`) **silently drops** names that no
  longer resolve. `PermissionsFor` (`permission/role.go:266`) turns schemes into
  `permTypes.Permission`.
- All token types resolve permissions through one shared function
  `expandRolePermissions` (`auth/user.go:172`) → `Role.PermissionsFor`.
  `BaseTokenPermission` (`auth/token.go:56`) delegates to `User.Permissions`.
  `Permissions()` is **not cached** — each check re-expands roles.
- `listPermissions` (`api/permission.go:429`) walks `PermissionRegistry.Permissions()`.

### 4.3 Service model (`service/`)

- `service.Service` (`service/service.go:35`) has no manifest field today.
  Persisted in Mongo (`_id` = name). Service names validated to
  `^[a-z][a-z0-9-]{0,39}$` (`service/service.go:364` → `validation.ValidateName`)
  — **no dots**, safe as a single scheme segment.
- `service.GetServices(ctx)` (`service/service.go:160`) returns all services in one
  query — used for startup re-population.
- `ServiceClient` interface (`service/service.go:68`) already exposes `Plans`
  (`GET /resources/plans`, `service/endpoint.go:371`) — the manifest-fetch method
  mirrors this pattern.

---

## 5. Data model changes

### 5.1 `Role` (`permission/role.go:19`) — add one field

```go
type Role struct {
    Name               string                `bson:"_id" json:"name"`
    ContextType        permTypes.ContextType `json:"context"`
    Description        string
    SchemeNames        []string `json:"scheme_names,omitempty"`         // static — UNCHANGED
    DynamicSchemeNames []string `json:"dynamic_scheme_names,omitempty"` // NEW — dynamic grants
    Events             []string `json:"events,omitempty"`
}
```

- Additive & backwards-compatible: absent BSON field decodes to `nil`.
- The static chain (`filterValidSchemes`, `PermissionsFor`) MUST continue to read
  only `SchemeNames`.

### 5.2 `Service` (`service/service.go:35`) — add manifest

```go
type Service struct {
    // ... existing fields ...
    Manifest *ServiceManifest `bson:"manifest,omitempty"`
}

type ServiceManifest struct {
    Enabled        bool              `bson:"enabled"`         // fine-grained feature on/off
    StrictActions  bool              `bson:"strict_actions"`  // default true; governs unmatched paths
    LegacyCompat   bool              `bson:"legacy_compat"`   // migration bridge
    LegacyEnabledAt *time.Time       `bson:"legacy_enabled_at,omitempty"` // when legacyCompat was turned on
    Operations     []ManifestOperation `bson:"operations"`
}

type ManifestOperation struct {
    Method     string `bson:"method"`      // GET|POST|PUT|PATCH|DELETE
    Path       string `bson:"path"`        // provider path / pattern, e.g. "/rules/{ruleId}/sync"
    Action     string `bson:"action"`      // authorization action, e.g. "rules.sync"
}
```

Notes:
- `StrictActions` defaults to `true` (deny). When decoding a manifest that omits
  it, set it to `true` explicitly during ingest.
- The V3 manifest's nested entity/operation structure is **flattened** into
  `Operations` at ingest: entity nesting becomes effective `Method + Path` patterns
  and an `Action` string.

---

## 6. Permission scheme naming (locked)

- Dynamic action scheme: **`service-action.<serviceName>.<action>`**
  - Example: manifest `service: acl`, operation `action: rules.sync` →
    scheme `service-action.acl.rules.sync`.
- `<serviceName>` is dot-free by validation, so it is a single safe segment.
- `<action>` MAY be dotted to form sub-grants: granting
  `service-action.acl.rules` covers `service-action.acl.rules.sync` (prefix
  parenthood). Validate `<action>` to `[a-z0-9-]` segments separated by `.`;
  reject empty segments and leading/trailing dots.
- Dedicated `service-action` root avoids any collision with the fixed built-in
  `service.*` verbs (`service.update.proxy`, `service.delete`, ...).
- Contexts for every dynamic action scheme: **`[CtxServiceInstance, CtxService,
  CtxTeam]`** (plus implicit `CtxGlobal`), matching what proxy handlers already
  pass via `contextsForServiceInstance`.

---

## 7. Pillar A — dynamic registry (`permission/dynamic.go`, new file)

A separate registry, independent of the static `PermissionRegistry`.

### 7.1 Requirements

- Guarded by a `sync.RWMutex`. Mutations happen at manifest ingest and startup;
  reads happen on proxy checks and role management.
- **Evaluated by name, not pointer identity.** Parenthood is dotted-prefix on
  `FullName()`: `service-action.acl.rules` is-parent-of
  `service-action.acl.rules.sync`. This is safe under concurrent mutation and boot
  rebuilds (no pointer stability needed) — deliberately different from the static
  registry's `reflect`-pointer approach.
- Precompute parent pointers at insert time (or avoid them entirely, since
  matching is name-based) so reads never mutate.

### 7.2 Public API (package `permission`)

```go
// RegisterDynamic registers (idempotently) an action scheme with the given
// contexts. Safe for concurrent use.
func RegisterDynamic(name string, ctxs []permTypes.ContextType) error

// UnregisterDynamic removes an action scheme (used on explicit manifest changes /
// service deletion). Idempotent.
func UnregisterDynamic(name string) error

// LookupDynamic returns the scheme and whether it exists.
func LookupDynamic(name string) (*permTypes.PermissionScheme, bool)

// ListDynamic returns all dynamic schemes (for the /dynamic-permissions endpoint).
func ListDynamic() permTypes.PermissionSchemeList

// CheckDynamic returns true if any granted name is an ancestor-or-equal of the
// requested name (dotted-prefix) AND the granted permission's context matches one
// of the provided contexts (or is global). This is the dynamic analogue of
// CheckFromPermList.
func CheckDynamic(granted []permTypes.Permission, requested string, contexts ...permTypes.PermissionContext) bool
```

`CheckDynamic` operates on `[]permTypes.Permission` (scheme + context) produced by
`Token.DynamicPermissions`, so it can honor context values just like the static
`CheckFromPermList`. Name-prefix parenthood replaces `IsParent`.

---

## 8. Pillar B — dynamic role methods (`permission/role.go`)

New methods; existing ones untouched.

```go
// AddDynamicPermissions validates each name against the DYNAMIC registry and its
// AllowedContexts vs the role's ContextType, then $addToSet into dynamicschemenames.
// Mirrors AddPermissions (role.go:186) but against RegisterDynamic's registry.
func (r *Role) AddDynamicPermissions(ctx context.Context, permNames ...string) error

// RemoveDynamicPermissions $pullAll from dynamicschemenames.
func (r *Role) RemoveDynamicPermissions(ctx context.Context, permNames ...string) error

// DynamicPermissionsFor mirrors PermissionsFor (role.go:266) but reads
// DynamicSchemeNames and resolves against the dynamic registry.
func (r *Role) DynamicPermissionsFor(contextValue string) []permTypes.Permission
```

- `AddDynamicPermissions` MUST reject a name that is not registered in the dynamic
  registry (`LookupDynamic`), and MUST enforce the role's context is in the
  scheme's allowed contexts (same rule as `AddPermissions`).
- `DynamicPermissionsFor` MUST resolve names against the dynamic registry; unknown
  names are skipped (inert-by-construction — see §11.3). It MUST NOT touch
  `SchemeNames`.

---

## 9. Pillar C — `Token` interface extension (non-optional)

`Token` is defined in `types/auth/token.go:18`, aliased as `auth.Token`
(`auth/token.go:19`). Add the method to the interface itself:

```go
type Token interface {
    GetValue() string
    GetUserName() string
    User(ctx context.Context) (*User, error)
    Engine() string
    Permissions(ctx context.Context) ([]permission.Permission, error)
    DynamicPermissions(ctx context.Context) ([]permission.Permission, error) // NEW
}
```

Non-optional by decision: all seven implementers are in-tree and share role
expansion, so a missing implementation is a compile error, not a silent auth gap.

Implementations (thin mirrors of the static path):

- `expandDynamicRolePermissions(ctx, roleInstances)` — sibling of
  `expandRolePermissions` (`auth/user.go:172`) calling `role.DynamicPermissionsFor`.
- `BaseTokenDynamicPermission(ctx, t)` — sibling of `BaseTokenPermission`
  (`auth/token.go:56`); resolves the user, returns `User.DynamicPermissions`.
- Per implementer:
  - `User.DynamicPermissions` → `expandDynamicRolePermissions(ctx, u.Roles + group roles)`.
    MUST NOT prepend the implicit `PermUser` self-permission (static-only concept).
  - `teamToken.DynamicPermissions` → `expandDynamicRolePermissions(ctx, t.Roles)`.
  - `APIToken`, `native`, `oauth`, `oidc`, `peer` → `BaseTokenDynamicPermission(ctx, t)`.

`Permissions(ctx)` MUST remain unchanged so dynamic grants never leak into the
generic per-request permission list. No caching (mirror the uncached `Permissions`).

---

## 10. Pillar D — manifest ingest, validation, startup

### 10.1 Sources of a manifest

- **Provider fetch:** add `Manifest(ctx, requestID) (*ServiceManifest, error)` to
  the `ServiceClient` interface (`service/service.go:68`) and `endpointClient`
  (`service/endpoint.go`), calling `GET /resources/manifest`, mirroring `Plans`
  (`service/endpoint.go:371`).
- **Admin set:** `PUT /services/{service}/manifest` (see §13), accepting
  `?force=true`.

Either path runs the same ingest routine.

### 10.2 Validation (shallow — tsuru does not validate provider payloads)

- Service name matches an existing service.
- Each operation: non-empty `action` (unique within service), `method` in the
  allowed enum, non-empty `action` matching the charset in §6, compilable
  `path` pattern.
- Reject duplicate operation actions and duplicate `(method, path)` pairs.

### 10.3 Guarded rename/removal policy (locked)

The `action` string is the **stability contract**. Providers MAY freely change an
operation's `path` or `method` as long as `action` is unchanged — existing
grants keep working.

On ingest, compute the set of `action`s being **removed or renamed** (present in the
stored manifest, absent in the new one). For each such action, check whether any
role lists `service-action.<svc>.<action>` in `DynamicSchemeNames`:

- If any active grant exists and the request does **not** carry `?force=true` →
  **reject** with `409 Conflict`, body listing the orphaning actions and affected
  roles. (Prevents silent denial of authorized subjects.)
- If `?force=true`, or no active grants exist → apply. Log the removed actions and
  affected roles. `UnregisterDynamic` the removed action schemes.

> **Documented pivot-back — "Minimal".** If Guarded proves too strict, switch the
> ingest-time *reject* to a *warn*: never block; emit a log/event listing removed
> actions and the roles that grant them; leave orphaned grants inert. Storage,
> registry, and check paths are identical — the pivot is a one-line policy swap plus
> surfacing the diff.

### 10.4 Registry updates on ingest

For each distinct `action` in the accepted manifest:
`RegisterDynamic("service-action."+svc+"."+action, []ContextType{CtxServiceInstance, CtxService, CtxTeam})`.
Then persist the manifest on the `Service` document.

### 10.5 Startup re-population (mandatory)

The dynamic registry is in-memory and rebuilt each boot; grants persist as names.
On API startup, **before serving traffic**, iterate `service.GetServices(ctx)` and
`RegisterDynamic` every enabled manifest's actions. This makes role grants resolve
after a restart. Failure of this step MUST NOT affect static authorization
(isolated failure domain) — log and continue; only dynamic service-proxy auth for
the failed service is degraded.

---

## 11. Pillar E — enforcement in proxy handlers

Apply in `serviceInstanceProxy` (`api/service_instance.go:699`) and
`serviceInstanceProxyV2` (`api/service_instance.go:743`). `serviceProxy`
(`api/service.go:291`) MAY be included using `CtxService` contexts; decide during
implementation (instance routes are the primary target).

### 11.1 Algorithm

```
1. Resolve service (+ instance) as today.
2. If svc.Manifest == nil || !svc.Manifest.Enabled:
       // unchanged legacy behavior
       return permission.Check(ctx, t, PermServiceInstanceUpdateProxy, ctxs...) ? proceed : 403
3. Feature enabled:
   a. Normalize proxied path (callback query for v1 proxy, :path catch-all for v2).
   b. action, matched := svc.Manifest.Match(r.Method, normalizedPath)
   c. allowed := false
      if matched:
          dyn, _ := t.DynamicPermissions(ctx)
          allowed = permission.CheckDynamic(dyn,
                        "service-action."+svc.Name+"."+action, ctxs...)
   d. if !allowed && svc.Manifest.LegacyCompat &&
         (matched || !svc.Manifest.StrictActions):
          allowed = permission.Check(ctx, t, PermServiceInstanceUpdateProxy, ctxs...)
   e. if !allowed && !matched && !svc.Manifest.StrictActions && !svc.Manifest.LegacyCompat:
          // strictActions=false without legacyCompat: fall back to coarse check
          allowed = permission.Check(ctx, t, PermServiceInstanceUpdateProxy, ctxs...)
   f. if !allowed: return 403
4. Emit event keyed by the resolved action (see §14); proxy the request as today.
```

`ctxs` = `contextsForServiceInstance(serviceInstance, serviceName)` (unchanged
helper, `api/service_instance.go:863`).

### 11.2 Decision truth table (feature enabled)

| Request | Has `service-action.<action>` | `legacyCompat` | `strictActions` | Result |
|---------|-------------------------------|----------------|-----------------|--------|
| Matched action | yes | any | any | **allow** |
| Matched action | no | true (+ holds `update.proxy`) | any | **allow** (bridge) |
| Matched action | no | false | any | **deny** |
| Unmatched path | — | true (+ holds `update.proxy`) | any | **allow** (bridge) |
| Unmatched path | — | false | true (default) | **deny** |
| Unmatched path | — | false | false (+ holds `update.proxy`) | **allow** (legacy) |

`strictActions` governs **unmatched** paths; `legacyCompat` governs
**matched-but-ungranted** (and, when `strictActions=false`, also unmatched). Both
ultimately ask "is the coarse `service-instance.update.proxy` still honored?".

### 11.3 Inert-by-construction guarantee

Because the requested action is resolved from the manifest, a grant name that no
longer corresponds to a declared action can never be the requested action → it
authorizes nothing. This is why manifest-only source-of-truth is safe and no
tombstoning/ledger is required.

---

## 12. Migration behavior

- `legacyCompat: true` makes a feature-enabled service a **strict superset** of
  today: instance-owner teams holding `service-instance.update.proxy` keep working
  while admins roll out `service-action.*` grants.
- While `legacyCompat` is enabled, tsuru emits a **recurring warning** (log/event)
  that the service runs in backwards-compat mode. **No hard expiry/TTL** — set
  `LegacyEnabledAt` when it is turned on so a TTL can be layered on later.
- Flipping `legacyCompat: false` removes the bridge → pure least-privilege.
- **No role auto-provisioning** (personas rationale, §3).

---

## 13. API endpoints

All additive; existing handlers unchanged.

### 13.1 Manifest management

- `PUT /services/{service}/manifest` — admin set/replace a manifest.
  - Query: `?force=true` to override the Guarded rejection (§10.3).
  - Permission: gate on a service-provider permission (e.g.
    `PermServiceUpdate` / `PermServiceUpdateProxy` on the service context — pick the
    existing appropriate `service.*` update permission during implementation).
  - Responses: `200` applied; `409` orphaned grants without force (body lists
    affected actions + roles); `400` invalid manifest.
- `GET /services/{service}/manifest` — return the stored manifest.

(Provider-served `GET /resources/manifest` is the backend contract used by the
optional provider-fetch path, §10.1.)

### 13.2 Dynamic permission discovery & role grants

- `GET /dynamic-permissions` — grouped by service, each action enriched with
  manifest metadata. Gate on `PermRoleUpdate` (same as `listPermissions`). Shape:

  ```json
  [
    {
      "service": "acl",
      "actions": [
        {
          "name": "service-action.acl.rules.sync",
          "action": "acl.rules.sync",
          "method": "POST",
          "path": "/sync",
          "contexts": ["service-instance", "service", "team"]
        }
      ]
    }
  ]
  ```

  `name` is the grantable scheme (what goes into `DynamicSchemeNames`); the rest are
  carried from the manifest for human context. Implementation joins `ListDynamic()`
  back to each service's manifest operation.

- `POST /roles/{name}/dynamic-permissions` — body: permission names →
  `Role.AddDynamicPermissions`.
- `DELETE /roles/{name}/dynamic-permissions/{permission}` →
  `Role.RemoveDynamicPermissions`.

Existing `/permissions` and `/roles/{name}/permissions` endpoints remain static-only
and unchanged.

---

## 14. Events / audit

Reuse the existing `event.New` block in the proxy handlers (non-GET/HEAD), but for
feature-enabled **matched** requests the event MUST be created with the resolved
dynamic permission scheme as `Kind`:

```go
kind, ok := permission.LookupDynamic("service-action." + svc.Name + "." + op.Action)
if !ok {
    // Registry/startup/ingest bug: fail closed instead of emitting an event with
    // an action that cannot be granted or audited consistently.
    return permission.ErrUnauthorized
}
evt, err = event.New(ctx, &event.Opts{
    Target:     serviceInstanceTarget(serviceName, serviceInstance.Name),
    Kind:       kind,
    Owner:      t,
    RemoteAddr: r.RemoteAddr,
    CustomData: serviceInstanceProxyEventData(r, authResult),
    Allowed: event.Allowed(permission.PermServiceInstanceReadEvents,
        contextsForServiceInstance(serviceInstance, serviceName)...),
})
```

Keep `Allowed` as the existing static `service-instance.read.events` permission
with the service-instance contexts. Event read access (`api/event.go:eventInfo` and
`eventList`) authorizes through `Allowed.Scheme`, not through `Kind`, so dynamic
event kinds do not require dynamic read-event permissions and do not leak through
the static `Permissions(ctx)` path.

Current event code supports this without storage changes:

- `event.Opts.Kind` is a `*permTypes.PermissionScheme`; `event.New` stores
  `kind.type="permission"` and `kind.name=opts.Kind.FullName()` without resolving
  the kind through the static permission registry.
- `types/event.EventData.Kind` is persisted as plain `{type, name}` data.
- The `events` collection already indexes `kind.name`, `starttime`,
  `allowed.scheme`, and the compound
  `{target.value, kind.name, starttime}` in `db/storagev2/indexes.go`; dynamic
  action names therefore use the same event-list/kind-filter indexes as static
  event kinds.
- `/events/kinds` uses Mongo `Distinct("kind")`, so dynamic action kinds will show
  up automatically after events are emitted.
- Webhooks match exact `KindNames` from `evt.Kind.Name`; event blocks use prefix
  matching on `KindName`. Dynamic names therefore enable per-action webhook and
  block behavior such as `service-action.acl.rules.sync` or the broader prefix
  `service-action.acl.rules`.

For feature-disabled services and feature-enabled requests that are allowed only
through an unmatched-path legacy fallback, keep the legacy
`service-instance.update.proxy` event kind because there is no resolved dynamic
action. For matched requests allowed by `legacyCompat`, still use the resolved
dynamic kind; `legacyCompat` changes only the authorization fallback, not the audit
identity. Keep `CustomData` with `method`, resolved `action`, and manifest
operation `action` for display and migration diagnostics.

---

## 15. Invariants — MUST NOT change

- `permission/permlist.go`, `permission/registry.go` static tree.
- `permission.Check`, `CheckFromPermList`, `IsParent`, `SafeGet`.
- `Role.SchemeNames`, `AddPermissions`, `PermissionsFor`, `filterValidSchemes`.
- `expandRolePermissions`, `BaseTokenPermission`, every token's `Permissions()`.
- The static registry stays immutable and lock-free; only the dynamic registry is
  mutable/locked.
- The `Token` interface gains exactly one new method; no existing method changes.

---

## 16. Normative requirements

- **R1** — The dynamic registry MUST be a separate, thread-safe (`RWMutex`)
  structure; the static registry MUST remain immutable and lock-free.
- **R2** — Dynamic authorization MUST be evaluated by scheme name (dotted-prefix
  parenthood), not pointer identity.
- **R3** — Role storage MUST carry dynamic grants in a distinct `DynamicSchemeNames`
  field; the static resolution chain MUST NOT read it.
- **R4** — The `Token` interface MUST declare `DynamicPermissions(ctx)`, implemented
  by all token types; `Permissions(ctx)` MUST remain static-only and uncached.
- **R5** — The dynamic path MUST be gated to feature-enabled service proxy calls;
  all other requests MUST follow the unchanged static path.
- **R6** — On startup, tsuru MUST re-register all enabled manifest actions into the
  dynamic registry before serving traffic; failure MUST NOT affect static auth.
- **R7** — The manifest is the sole source of truth for dynamic schemes (no ledger).
  Action removal/rename MUST follow the Guarded ingest policy: reject on orphaned
  active grants unless `?force=true`, with logging.
- **R8** — A feature-enabled service MUST support a per-service `legacyCompat` flag
  that additionally honors `service-instance.update.proxy` for matched-but-ungranted
  requests (and, when `strictActions=false`, for unmatched paths). While enabled it
  MUST emit a recurring warning and record `LegacyEnabledAt`; there is no TTL.
- **R9** — Unmatched paths on feature-enabled services MUST be governed by a
  per-service `strictActions` flag, defaulting to `true` (deny).
- **R10** — Scheme names MUST be namespaced `service-action.<service>.<action>` with
  validated charset; `<action>` MAY be dotted for hierarchical grants.
- **R11** — For feature-enabled matched proxy requests, tsuru events MUST use the
  resolved dynamic permission scheme as `Kind`; event visibility MUST remain gated
  by the static `service-instance.read.events` `Allowed` permission.

---

## 17. Implementation task breakdown (suggested order)

1. **Dynamic registry** — `permission/dynamic.go`: registry type, `RWMutex`,
   name-based parenthood, `RegisterDynamic`/`UnregisterDynamic`/`LookupDynamic`/
   `ListDynamic`/`CheckDynamic`. Unit tests for concurrency and prefix matching.
2. **Role dual-field** — add `DynamicSchemeNames` to `Role`; implement
   `AddDynamicPermissions`/`RemoveDynamicPermissions`/`DynamicPermissionsFor`. Tests.
3. **Token interface** — add `DynamicPermissions` to `types/auth/token.go`;
   implement `expandDynamicRolePermissions`, `BaseTokenDynamicPermission`, and the
   method on all seven token types. Tests per token type.
4. **Service manifest model & storage** — add `ServiceManifest`/`ManifestOperation`
   and `Service.Manifest`; `Match(method, path)` matcher (compiled, deterministic,
   longest/most-specific first). Tests for matcher.
5. **Manifest ingest** — validation, Guarded policy with `?force`, registry
   updates, persistence; provider-fetch `ServiceClient.Manifest` + `endpointClient`.
6. **Startup re-population** — hook in API bootstrap before serving; iterate
   `GetServices` and register. Test restart resolves grants.
7. **Proxy enforcement** — implement §11 algorithm in the instance proxy handlers;
   wire `legacyCompat`/`strictActions`; create matched proxy events with the
   dynamic permission `Kind` and keep event custom-data.
8. **API endpoints** — `PUT/GET /services/{service}/manifest`,
   `GET /dynamic-permissions`, `POST/DELETE /roles/{name}/dynamic-permissions`.
9. **Migration nudge** — recurring warning while `legacyCompat` enabled;
   `LegacyEnabledAt`.
10. **Docs/CLI** — client commands for manifest set/get and dynamic role grants;
    update API docs.

## 18. File-by-file change map

| File | Change |
|------|--------|
| `permission/dynamic.go` (new) | Dynamic registry + `CheckDynamic` + register/list APIs |
| `permission/role.go` | `DynamicSchemeNames` field; `AddDynamicPermissions`, `RemoveDynamicPermissions`, `DynamicPermissionsFor` |
| `types/auth/token.go` | Add `DynamicPermissions` to `Token` interface |
| `auth/user.go` | `expandDynamicRolePermissions`; `User.DynamicPermissions` |
| `auth/token.go` | `BaseTokenDynamicPermission` |
| `auth/api_token.go`, `auth/native/token.go`, `auth/oauth/token.go`, `auth/oidc/token.go`, `auth/peer/peer.go`, `auth/team_token.go` | Implement `DynamicPermissions` |
| `service/service.go` | `ServiceManifest`/`ManifestOperation` types; `Service.Manifest`; `Match`; ingest/validate; `ServiceClient.Manifest` |
| `service/endpoint.go` | `endpointClient.Manifest` (`GET /resources/manifest`) |
| `api/service.go` | `PUT/GET /services/{service}/manifest`; optional `serviceProxy` enforcement |
| `api/service_instance.go` | Enforcement in `serviceInstanceProxy` / `serviceInstanceProxyV2` |
| `api/permission.go` | `GET /dynamic-permissions`; dynamic role grant/revoke handlers |
| `api/server.go` | Register new routes |
| API bootstrap (startup) | Re-populate dynamic registry from manifests before serving |

## 19. Testing plan (minimum)

- Dynamic registry: concurrent register/read; prefix parenthood; context matching.
- Role: add/remove/resolve dynamic grants; static `SchemeNames` untouched;
  `filterValidSchemes` never drops dynamic grants.
- Token: every implementer returns dynamic perms; `Permissions()` unchanged.
- Matcher: parented paths, unmatched.
- Enforcement: full truth table (§11.2) incl. `legacyCompat`/`strictActions`.
- Ingest: Guarded reject vs `?force`; validation failures; startup re-population
  after simulated restart.
- Backwards compat: non-feature services behave exactly as today.

## 20. Non-goals (out of scope for this spec)

- Entity persistence, entity envelopes, status tracking, async operations, operation
  completion callbacks (V3 step 2+).
- Dynamic-scheme ledger / tombstoning (not needed — §11.3).
- Auto-provisioning of per-service roles.
- Replacing the static registry's pointer-identity model.

## 21. Decisions log

| # | Decision |
|---|----------|
| Design | Dual registry + dual-field role storage; non-optional `Token` extension (option 2b) |
| Q1 | Reserved root `service-action.<service>.<action>` |
| Q2 | Manifest-only source of truth (no ledger); Guarded rename policy; Minimal documented as pivot-back |
| Q3 | Per-service `strictActions`, default deny |
| Q4 | No caching of `DynamicPermissions` |
| Q5 | No role auto-provisioning; `legacyCompat` migration bridge |
| force | `?force=true` query parameter on the admin manifest-set endpoint |
| legacyCompat nudge | Recurring warning, no hard TTL; record `LegacyEnabledAt` |
| `/dynamic-permissions` shape | Grouped by service, actions enriched with manifest metadata |
