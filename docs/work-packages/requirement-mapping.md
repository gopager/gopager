# Requirement Mapping and Planned Evidence

[Assignments](../work-packages.md#revised-execution-path) | [Requirements](../requirements-catalog.md) | [Delivery scope](../authoritative-plan.md#delivery-scope)

This table owns requirement-to-producer coverage, not requirement definitions, delivery approval or results. Linked assignments own exact planning, paths and cases. AP-17 maps approved claims to actual producer evidence.

| REQ / capability | Producer | Evidence category |
| --- | --- | --- |
| REQ-001..019/027/029/055..060/070/098 query/live/detail | [AP-10 T063..T065](ap-10-query-live.md), [AP-15 T078..T080](ap-15-frontend.md) | Contribution-first projection/filter/page, Unicode, HTTP/SSE, browser freshness/revocation/navigation |
| REQ-020/031..032/037..038/041..046/113..114/121..122 protocol/intake/Grouping | [AP-07](../execution/ap-07.md), [AP-08 T060..T062](ap-08-master-data-correction-after-intake.md) | Protocol/target/Grouping, acceptance/receipt/commit uncertainty and correction |
| REQ-021..026/028/030/045/095/132 master data/defaults/templates | [AP-08 T060/T062](ap-08-master-data-correction-after-intake.md), AP-15 | Snapshot/default detachment/bulk conflict/authorization, PostgreSQL/HTTP/browser |
| REQ-031..040/123..124 assignments/suppression | [AP-08 T061/T062](ap-08-master-data-correction-after-intake.md), AP-10 | Original-preserving correction, revision/claim/race/restart, safe visible derived state |
| REQ-041..050 provisioning/diagnostics/contact state | [AP-09 T086..T088](ap-09-gateway-network.md), AP-15 | Production provisioning, HTTP diagnostics, timed contact races and atomic state facts |
| REQ-049/081..082/084..087/105..106/129..130 events/Delivery | [AP-11 T066..T068](ap-11-events-delivery.md), [AP-12 T069..T071](ap-12-plugin-host.md), AP-15 | Claims, input/rendering, split/replay/idempotency/current rights, controlled versus real providers |
| REQ-051..052/054..064/069..079/085/109..110 identity/lifecycle | [Identity core](../execution/source-ap-07.md#handover), [AP-14 T075..T077](ap-14-identity-lifecycle.md), AP-10/AP-11/AP-15 | Additive scopes/fields/age, safe management/recovery/rescue, immediate invalidation and participant cleanup |
| REQ-089..090/106..107/129 host/initial targets | [AP-12 T069..T071](ap-12-plugin-host.md) | Manifest/configuration/Secrets/ABI/lifecycle, target method/signature matrix, actual external Delivery |
| REQ-091..095 shared Delivery rendering | [AP-11 T066/T068](ap-11-events-delivery.md) | Established bounded engine, declared inputs/output contexts, hidden defaults/conflicts |
| REQ-091..100 Processing/presentation/preferences/notices | [AP-06](../execution/ap-06.md), [AP-13 T072..T074](ap-13-processing-presentation-integration.md), AP-15 | Evaluator/preview/activation, historical Reprocess without sending, source-bound safe presentation and browser accessibility |
| REQ-097/099..100 frontend assets/i18n/usability | [AP-15 T078..T080](ap-15-frontend.md) | Build/asset registration, English keys/typed parameters, independent locale, desktop/mobile/keyboard/touch |
| REQ-107/114 foundation redaction/authority | [AP-05](../execution/ap-05.md), AP-07/AP-12/AP-16 | Safe errors/logs, transaction uncertainty, bounded shutdown/current authority |
| REQ-108 throttling | Identity core/AP-14 login; [AP-10 T063/T065](ap-10-query-live.md) search; [AP-11 T067/T068](ap-11-events-delivery.md) sending; AP-12 provider limits | Explained admission and shared-IP fairness, distinct from page/deadline/retry bounds |
| REQ-111..112/117..118/120/127 operations | [AP-16 T081..T083](ap-16-operations.md) | Actual retention/status/setup/update/shutdown, disposable Full topology/fault/recovery, measured environment without SLA |
| REQ-115..116/119 profile preparation only | AP-05, [AP-16 T083](ap-16-operations.md) | Adapter/topology/transition boundary; no Lite runtime, parity or executable transfer claim |
| REQ-133/138 saved views | [AP-10 T063/T065](ap-10-query-live.md), AP-15 | Own product clarification, then approved server ownership/delegation/visibility and private saved-view tests |
| REQ-134 single-Gateway delegation | [AP-09 T086/T088](ap-09-gateway-network.md), AP-15 | Own delivery clarification, then approved detail-versus-rotation scope tests |
| REQ-135..137 field/query refinements | AP-10/AP-15 | Selected-Unit, Payload-only and hidden-filter/option scenarios |
| REQ-139 plugin reactions to any Domain event | Unassigned; needs the plugin-model planning revision under [D-130](../authoritative-plan.md#plugin-trust-clarification) (AP-11/AP-12 compatibility context) | Not in the initial release; planning must keep the event catalog, reaction Subscriptions and plugin capabilities open for it |
| REQ-053/065..068/080/083/101..104/125/128/131 later capabilities | No current implementation producer | Delivery scope determines future work; no present completion claim |
| REQ-088/126 removed identifiers | None | Reserved, not implementation tasks |

Foundation evidence is an input, not proof of later product behavior. Each consumer verifies its exact required versions. Missing producer evidence remains with that producer; AP-17 does not backfill it.
