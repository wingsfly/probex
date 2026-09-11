# GuideX Runtime v4 Schema Lifecycle

- Current status: NOT FINALIZED. Only explicit user confirmation finalizes the fields.
- During v4 schema iteration, clear obsolete test results and aggregates only for
  `ext_guidex-runtime-v4` in the verified environment. Check target and counts
  before clearing, then verify that unrelated records and registrations remain.
- Do not maintain temporary v4 aliases or reconstruct missing milestones.
  Legacy adapter support is separate and must remain intact.
- Never add scheduled, startup, plugin-load, or per-result automatic deletion.
- After user sign-off, mark this status and the extension's status/docs FINALIZED.
  Preserve historical results during future field changes; use versioning,
  migration or compatibility instead. Existing retention policies are separate.
- The sibling extension repository is `../probex-webrtc-guidex-extension`;
  see its `docs/guidex-runtime-v4.md` for field definitions and cleanup scope.
  Updating source or clearing data does not update already-injected pages.
