# Integration Coverage Plan: service + fine-grained service permissions

  ## Summary

  Add two new integration test files and fixtures to cover:

  1. general service manifest behavior through integration flow patterns.
  2. service fine-grained permission proxy enforcement, including role creation/assignment via curl (CLI role-management-free path) and `/permissions` dynamic action checks.

  ## Key changes / additions

  ### 1) New service-general integration flow

  - Add integration/service_test.go with an ExecFlow function named serviceGeneralFlow (or equivalent).
  - Keep style consistent with install_test.go:
      - use ExecFlow, ResultOk, ResultMatches, retry, and environment interpolation ({{.servicename}}, {{.targetaddr}}, etc.).

  - Flow behavior:
      - Requires servicename (reuse service from existing serviceCreate flow).
      - Verify base service behavior:
          - tsuru service list includes created service.
          - tsuru service info {{.servicename}} succeeds.
          - GET /1.31/services/{{.servicename}}/manifest returns JSON.

      - Push a baseline manifest from fixture and confirm readback:
          - PUT /1.31/services/{{.servicename}}/manifest with fixture payload.
          - GET again and assert returned enabled, strict_actions, and operation count match fixture.

  - Backward cleanup:
      - optional manifest reset to disabled/empty manifest fixture to keep subsequent tests deterministic.

  ### 2) New fine-grained permissions integration flow

  - Add integration/service_fine_grained_permissions_test.go with an ExecFlow named serviceFineGrainedPermissionsFlow.
  - Use curl for role-management API calls (as requested):
      - POST /1.0/roles to create role (name, context=team).
      - POST /1.0/roles/{role}/user to assign admin user (email={{.adminuser}}, context={{.team}}).
- Grant/revoke via CLI: `tsuru role permission add <role> <permission>` and
  `tsuru role permission remove <role> <permission>`.
      - GET /1.0/roles/{role} and `/permissions` assertions for `service-action.<service>.<action>` entries.

  - Authorization checks:
      - The integration code should obtain the current tsuru token by running `tsuru token show`.
      - Use the returned token as `Authorization: Bearer <token>` for curl/API calls.
      - Set service manifest with FG fixture containing known action routes (e.g. rules.sync + strict mode).
      - For a request to /services/{service}/proxy/{instance}?callback=/resources/{instance}/rules/123/sync:
          - before dynamic grant → expect 403.
          - after dynamic grant → expect non-403 and successful proxy authorization behavior.

      - Include at least one negative unmatched/fallback case depending on strictness in manifest fixture (strict_actions true keeps behavior deterministic).

  - Backward cleanup:
      - remove dynamic permission from role.
      - dissociate role from user.
      - delete role.
      - restore manifest to safe value (if flow changed it).

  ### 3) New fixture files under integration/fixtures

  - Add:
      - integration/fixtures/service/manifest-general.json
      - integration/fixtures/service/manifest-fine-grained.json

  - Keep payloads in API format (application/json) because FG tests write via curl directly.
  - Include operation action/path/method combinations that are explicit and stable for assertions.

  ### 4) Flow registration

  - Register both new flows in integration/install_test.go:
      - service general flow near existing service flow.
      - FG flow after serviceBind() (so a service instance is available for proxy path checks).

  ## Test plan

  - Run:
      - make test-ci-integration
      - make local.test-ci-integration

  - Validate at least:
      - service manifest round-trip and readback.
      - role create/update/revoke via curl.
      - permissions list includes `service-action.<service>.<action>`.
      - FG proxy pre-grant denied and post-grant allowed path behavior.
      - cleanup runs without lingering roles.

  ## Assumptions and defaults

  - Chosen FG scope: include proxy authorization behavior (user-selected).
  - The test runner must be logged in with an admin-capable tsuru user; integration code should call `tsuru token show` to retrieve the token required for role endpoints and manifest write.
- Use API versioned path 1.0 for base roles endpoints and `/permissions`, and 1.31 for services/{name}/manifest calls.
