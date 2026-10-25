# C-11 Processing, Presentation, and User Configuration

[Contract index](../domain-contracts.md) | [Shared conventions](shared-conventions.md) | [Reference cases](reference-cases.md)

Referenced contracts (follow only when relevant): [C-03](c-03-message-grouping.md), [C-06](c-06-identity-authorization.md), [C-07](c-07-query-live.md), [C-08](c-08-events-jobs.md), [C-09](c-09-delivery.md), [C-10](c-10-plugin-host.md).

Targeted sections: [Workflow schema/evaluation](#workflow-definition-and-results), [publication/activation/reprocessing](#ingest-publication-activation-and-reprocessing), [presentation/configuration](#presentation-and-configuration), [English key mapping](#text-key-mapping-and-remaining-technical-evidence). Concrete checks: [F-37/F-38](ap-02-boundary-cases.md).

**Traceability:** [REQ-091..097/099..100](../requirements-catalog.md#j-processing-presentation-and-usability). Issue reference: [P-16](../authoritative-plan.md#open-issues-and-risks); status is maintained there.

**Version/status:** 0.1-draft. **Owner:** DOM-08; frontend consumes permitted configuration/results. **Producers:** authorized administrative rules and sample/Transmission Payload. **Consumers:** presentation and explicit Domain event reactions.

Draft interfaces: `PreviewRules(Principal, draft, sample) -> derivedFields, explanation, errors` without live effects; `SaveRuleVersion(Principal, expectedRevision, definition) -> savedVersion`; `ActivateRuleVersion` / `SelectPreviousVersion` -> traceable active state; `EvaluateRules(context, version, Payload) -> boundedDerivedResult`; `RenderAuthorizedPayload(projectedFields, presentationConfig) -> safeViewModel`.

**Defined:** Parsing, bounded regex extraction, administratively structured data, conditions/branches, preview/errors. Flexible composition preferred; basic branches are an effort fallback. No arbitrary inline scripts, loops, or expanded permissions; trusted installed code steps use C-10's explicit capabilities. Saving and taking effect are distinct without a mandated multistage approval. Concurrent edits produce conflicts rather than silent overwrites.

Preserve original Payload; processing/presentation version changes create no Transmission/regrouping. Derived fields retain required classification; regex extraction does not make hidden Payload text public. Rendering/templates allow permitted arrangement/highlighting without mandatory HTML/scripts. Preview silently changes neither real Messages nor targets and sends nothing.

**D-29/D-30 source selection:** GUI rendering consumes C-03/C-07's readable exact Representative Transmission, not an unspecified global text or concatenated originals. Processing results retain source Transmission/Payload and Workflow context. A later better representative must have required Processing completed before publishing its new content; do not mix another member's fields into it. A restricted view's fallback cannot inherit fields derived only from hidden evidence. Different canonical fuller text belongs to a different Message.

Installation title/notices, dismissal rule, locale/time format/gaps, and preferences have clear persistence/revision/permissions. Text/touch/keyboard, readable states independent of color, and context-preserving copy/details are frontend acceptance goals.

**Internationalization preparation:** Under REQ-097/D-20, English is the initial UI language and text catalog. Isolate all product-owned user-facing text, including editor validation, statuses, accessibility labels, and built-in presentation Localized text, behind stable text keys. Support parameters/plurals instead of concatenated sentences, and define English fallback. Keep UI language separate from persisted formatting preferences and locale-aware dates/numbers/time zones. Do not translate received Payload, Unit names, operator templates, or extracted values automatically. Text keys and parameters do not grant data access; fallback/error rendering must retain field permissions and redaction. No additional translation catalog, language pack, or selector is required yet. Select a suitable Vue i18n facility during AP-02/AP-13; none is chosen or implemented here.

## Workflow Definition and Results

| Record | Required logical information |
| --- | --- |
| Workflow | Stable ID, mutable revision, name, enabled state, active immutable definition-version reference. Changes require `expectedRevision`. |
| Definition version | Workflow/version reference, selector, ordered structured steps, output-field schema, creation time/actor; saved definitions remain selectable while referenced. Saving does not activate. |
| Selector | C-09 typed predicate over declared Protocol/Payload form/subtype, resolved Network and permitted target context; original Payload text only where readable. Selection is per Transmission, not per Unit or Message. |
| Step | Stable local step ID, kind, typed input/output references, configuration; kinds: extract, map, condition/branch, and declared installed processing capability. Branches are structured, references point only to prior outputs, cycles are invalid. |
| Output field | Workflow-namespaced name, type (string, boolean, integer, finite number, timestamp, bounded object/list), optionality, source-field classification. Absent differs from null/empty. No implicit overwrite of another Workflow's field. |
| Processing result | Source Transmission ID, Workflow/definition version, evaluated configuration/target-resolution revision, result revision, pending/running/completed/failed/superseded status, bounded typed fields, safe error/step reference and completion time. No second editable original. |

Multiple matching Workflows evaluate independently on the same immutable input and current derived context. Namespace isolation removes scheduling-order dependence; one Workflow cannot silently consume another's result. Presentation/Subscription mappings explicitly select a namespace/field. No duplicate or ambiguous unqualified field lookup. Processing fan-out is once per `(Transmission, Workflow, definition version, relevant input revision)` with C-08 idempotency/fencing; a retry does not duplicate a result/fact.

Extraction uses Go RE2-compatible regular expressions, without backreferences/lookbehind or runtime-specific backtracking. Pattern validation rejects unsupported syntax; named captures map explicitly to declared typed fields. No match is an explained absent optional value, not an execution failure; invalid required conversion/schema, resource exhaustion or plugin failure is an explicit failed result. Shared limits apply (128 steps, nesting 32, 1000 collection elements, 4096-byte pattern, 1 MiB input, 1 s built-in budget). C-10's 30 s bound applies to each installed code invocation; the finite step count also bounds the number of invocations. No unbounded loops, recursion or implicit external Delivery step.

Classification propagates from every input used to compute a field, including branch/selector dependencies, not just copied strings. A derived field requires its declared `derived` permission and the necessary source-field/context rights; extraction or a boolean condition does not declassify hidden text. Administrators cannot make a hidden field public by renaming it. A user-initiated preview needs `rule.preview` and read permission for all stored sample data; a synthetic sample is request data, not permission to fetch hidden Messages.

Core-owned built-in ingest derivation is internal processing, not a user grant or outbound capability. Installed code receives only its declared authorized input and current configured execution Principal; any further SDK read or external/OS access obeys C-10. Activation validates that capability/scope binding; it never captures the editor's temporary admin rights. Missing execution authority is an explicit failed/pending state, not an unrestricted worker fallback.

## Ingest Publication, Activation and Reprocessing

1. Capture active Workflow versions and relevant current context for the accepted Transmission. No matching Workflow means the barrier is complete, not failed. Each selected Workflow must complete before its new evidence/results contribute to normal GUI/search/Delivery publication.
2. Original acceptance and internal facts can commit while Processing is pending. Managers see safe processing status; normal views do not receive the new unprocessed state. A failed Workflow leaves that state failed/pending publication, with authorized retry, not a silently successful raw-text substitute.
3. Compute without a long SQL transaction; publish results, ready target/contribution state and `processing.completed` facts with current revisions and C-08 fencing. Changed relevant input/configuration supersedes stale work. A pending new reception must not remove previously valid published evidence from an existing Message; select representatives only from ready readable supports.
4. `ActivateRuleVersion`/`SelectPreviousVersion` atomically change the active pointer with revision/audit. New inputs use it; pending work on the old pointer is superseded/reselected before publication. Already published results stay source/version-bound until explicit reprocessing; no automatic historical sweep or regrouping.
5. `Reprocess` requires `rule.reprocess`, explicit permitted source scope and chosen version. It uses current access/target state and preserves original Payload/membership. Replacement results publish atomically after success; invalidated source authority is never kept visible merely because a prior result exists. Historical external reactions are suppressed by default through C-08 `replayMode`; redelivery is C-09's separately confirmed operation.

Preview runs the same validator/evaluator against the supplied draft/sample and returns typed fields, selected branches, absent-field explanations and safe step errors. It neither saves/activates nor writes live results, events or Deliveries. External/OS-effecting installed steps are not invoked in preview: return an explicit capability-unavailable explanation unless a declared pure preview implementation exists. Never quietly execute live integration code.

## Presentation and Configuration

Presentation template: ID/revision, immutable definition version, applicable permitted scope, typed field bindings and output kind (GUI card or Delivery text/structured input). Use an established engine's constrained condition/iteration/fallback syntax for both contexts; no second handwritten expression parser. Concrete library/version, Vue editor and optional visual builder are implementation selections in AP-12/AP-13, subject to these semantics, not missing product decisions or promised parity. Rendering is bounded by shared output/nesting/collection limits and escapes interpolation for its output context.

GUI card models support text, labeled structured sections, icons, links/buttons and configured map references. Missing permitted structured location/destination fields fall back to permitted original text according to explicit template conditions, not hidden alternate evidence. No arbitrary browser script or raw untrusted HTML; SVG/icons are sanitized, unsafe URL schemes/event handlers/executable references rejected. Admin-defined external links/maps may deliberately transmit permitted parameters (D-25); their external origin is visible. No arbitrary outbound requests just to preview a template; provider-specific output escaping is C-09/C-10.

Precedence is fieldwise: installation presentation default, effective Organization/Unit settings under C-04, then explicit view/Subscription template selection. If several Units prescribe conflicting whole templates, the view/Subscription explicitly chooses one or renders separately scoped sections; never select by map iteration order or combine hidden Unit settings. Personal formatting preferences cannot widen field rights or modify source results.

Installation title/notices have ID/revision, plain or sanitized content, publication state and audience. Dismissal records `(Principal, notice ID, published revision)`; publishing a changed revision shows it again. Guest dismissal is local browser state, not a mutation of shared Guest preferences. Account preferences store formatting locale, IANA time zone, time-gap display and Delivery defaults with ordinary revision checks; Delivery configuration still belongs to C-09. Invalid locale/time-zone input is explained, not silently replaced. UI language remains English initially.

## Text-key Mapping and Remaining Technical Evidence

C-07 owns the complete API error-code/fallback table. C-11 owns `status.<status>`, `validation.<constraint>`, `processing.<reason>` and component/action/accessibility keys. Registry entries require an English base string, typed named parameters and plural forms when count-dependent; CI in AP-13 checks every referenced built-in key/parameter against the catalog. Validation constraints reuse shared names (`required`, `type`, `range`, `maxLength`, `pattern`, `reference`, `cycle`); processing reasons include `no-match`, `conversion-failed`, `capability-unavailable`, `resource-limit` and `evaluation-failed`. Missing optional output is not displayed as failure.

A missing translation uses the English entry; a missing key/invalid parameter emits a redacted diagnostic and safe generic English text, never the confidential raw argument or an untranslated key. New built-in status/constraint/reason values require their English entry in the same change. Trusted operator templates and received data are not automatically translated. Exact component wording, the Vue i18n library and editor controls are AP-13 implementation artifacts, not a requirement for another locale.

| Text keys (one entry per listed suffix) | English base entries, in the same order |
| --- | --- |
| `status.pending`, `.running`, `.completed`, `.failed`, `.superseded` | Pending; Running; Completed; Failed; Superseded |
| `status.prepared`, `.started`, `.retry-pending`, `.accepted`, `.expired`, `.cancelled`, `.uncertain`, `.dependency-unavailable` | Prepared; Started; Retry pending; Accepted by provider; Expired; Cancelled; External outcome uncertain; Required data unavailable |
| `status.never-seen`, `.active`, `.degraded`, `.stopped`, `.contact-lost` | Never contacted; Active; Degraded; Deliberately stopped; Contact lost |
| `status.installed`, `.validated`, `.configured`, `.incompatible`, `.disabled` | Installed; Validated; Configured; Incompatible; Disabled |
| `status.deleting`, `.cleanup-failed`, `.retrying`, `.fresh`, `.processing-pending`, `.resync-required` | Deleting account; Account cleanup failed; Retrying; Up to date; Processing pending; Reload required |
| `validation.required`, `.type`, `.range`, `.maxLength`, `.pattern`, `.reference`, `.cycle` | Required value missing; Invalid value type; Value outside allowed range; Value exceeds length limit; Unsupported or invalid pattern; Invalid reference; Cyclic references are not permitted |
| `processing.no-match`, `.conversion-failed`, `.capability-unavailable`, `.resource-limit`, `.evaluation-failed` | No matching value; Value conversion failed; Capability unavailable; Processing limit exceeded; Processing failed |
| `account.deleted`, `delivery.retryPending`, `history.retainedPartially`, `workflow.failed` | Deleted user; Retry pending; Earlier history may have been removed; Workflow failed |

`account.deleted` carries the distinguishing suffix separately; add it accessibly without turning an ID into a name. `status.accepted` is scoped to Delivery; intake acknowledgement uses its own component wording, never claims provider acceptance. Validation parameters are typed `field`, `constraint`, `limit` as applicable. Count-dependent component keys declare an integer `count` and English singular/other forms (for example `{count} item` / `{count} items`), not concatenated fragments. Component/action labels are populated with their implementing UI, not a speculative catalog of screens.

AP-03 validates runtime budgets, regex cancellation and installed-code execution. AP-12 compares established template/builder libraries against these semantics before implementation; the inspected legacy parser/cards are functional provenance, not adopted code or a parity goal. No product runtime or preview was executed by AP-02.
