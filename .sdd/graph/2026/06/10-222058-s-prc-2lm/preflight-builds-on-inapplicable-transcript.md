# Pre-flight transcript: `builds-on` on terminal done blocked as inapplicable (redacted)

Single-run `[high] ref-kind-inapplicable` finding on a `builds-on` ref to a terminal done signal,
observed 2026-06-10 in an external project graph (German-language entry bodies). Redactions:
person, company, and product names, ticket number, and domain content replaced with placeholders
(`<vendor>`, `<contact>`, `<ticket-no>`, `<org-X>`, …). Entry IDs, the failing ref, the finding
text's reasoning, and the body framing that triggered it are kept verbatim.

## Context

The proposed entry was a fact signal (conceptual layer, confidence high) recording that an external
vendor answered a set of integration questions. The answer arrived in response to a support ticket
whose creation was recorded earlier the same day as a terminal done signal
(`20260610-184443-s-ops-oq5`). The proposed entry referenced that done signal with kind
`builds-on` — narrative: ticket step finished, the answer takes up the follow-up point the done
flagged ("question stays open until the vendor's answer arrives").

## Failing invocation (shape)

```bash
DESC=$(cat <<'EOF'
<vendor> hat die Fragen zur übergreifenden Identifikation beantwortet (<contact>, Ticket
#<ticket-no>; vollständige Antwort im [Anhang]({{attachments}}/<vendor>-antwort-<ticket-no>.md)).
Kernaussagen: **(1)** Bei <org-A> werden „Abgleich-<entitäten>" definiert — [redigierte
Domänendetails: Zuordnung von Kennungen desselben <entität>s über mehrere <organisationen>,
existiert nur auf der <org-A>-Datenbank]. **(2)** [redigiert: ein zweites Identifikationsmerkmal
ist über alle <organisationen> gleich, wodurch übergreifende Identifikation kombiniert möglich
ist]. **(3)** [redigiert: Aussage zu Einheiten-Codes, mit Querbezug auf
20260610-181145-s-cpt-wdb]. **(4)** [redigiert: Aussage zu Duplikaten innerhalb einer
<organisation>]. Damit ist die übergreifende Identifikation grundsätzlich möglich
(20260610-180615-s-cpt-k6a); offen bleibt, ob die Zuordnung über die Schnittstellen geliefert
werden kann. Die Antwort ging aus dem angelegten Ticket hervor (20260610-184443-s-ops-oq5).
EOF
) && sdd new s cpt --kind fact --confidence high \
  --participants Christopher,Claude \
  --refs '{"id":"20260610-180615-s-cpt-k6a","kind":"addresses","desc":"beantwortet die offenen Fragen; Identitätsschema noch zu entscheiden"}' \
  --refs '{"id":"20260610-184443-s-ops-oq5","kind":"builds-on","desc":"greift den im Ticket-Schritt offen gebliebenen Punkt auf"}' \
  --refs '{"id":"20260610-181145-s-cpt-wdb","kind":"related","desc":"Einheiten-Aussage betrifft die Wertelisten-Frage"}' \
  --topics <topic-1>,<topic-2> \
  --attach <path>/<vendor>-antwort-<ticket-no>.md:<vendor>-antwort-<ticket-no>.md \
  "$DESC"
```

Note: an earlier draft of the body framed the relationship as provenance — *"Die Antwort kam auf
das angelegte Ticket"* — which is the phrasing the finding quotes below.

## Pre-flight output (run 1 of 1 — capture was stopped, no retry run)

```
Error: Exit code 1
  [high] ref-kind-inapplicable: Ref to 20260610-184443-s-ops-oq5 uses kind `builds-on`, but that
  target is a terminal `done` signal. The body frames the relationship as 'Die Antwort kam auf das
  angelegte Ticket' — the proposed entry is not extending a finished chain but rather reasoning
  from the done as empirical context (the ticket produced the answer). The applicable kinds here
  are `grounded-in` (reasoning from the done as context/evidence) or potentially `surfaces` if the
  ticket work is what produced this fact, but `builds-on` on a terminal done whose body is used as
  provenance/context maps most precisely to `grounded-in`.
pre-flight validation blocked: 1 high-severity finding(s)
stdin attachment saved (pre-flight rejected): <path>/.sdd/tmp/<uuid>-<vendor>-antwort-<ticket-no>.md
  retry: sdd new ... --attach <path>/.sdd/tmp/<uuid>-<vendor>-antwort-<ticket-no>.md:<vendor>-antwort-<ticket-no>.md
  ✕ pre-flight rejected entry
```

## Why this is a calibration failure

- On a terminal done target, only `addresses` is inapplicable; `builds-on` vs `grounded-in` is
  documented as a defensible choice, not an error ("Choosing between `builds-on` and `grounded-in`
  here is a defensible-choice question, not an error" — framework-concepts, ref kinds).
- The finding's own prose performs the tie-break analysis (names `grounded-in` and `surfaces` as
  applicable, argues `grounded-in` "maps most precisely") — a sharpness judgment — yet labels the
  non-preferred applicable kind *inapplicable* and blocks at `[high]`. Under the applicable-never-
  high rule (d-prc-v0h), the ceiling for this finding is `[low]`.
- Unlike the self-retracting (s-prc-lbv) and cross-run-oscillating (s-prc-pex) instances, the
  finding is internally coherent and confident — no retraction, no contradiction to detect
  mechanically. The failure is purely the severity-class assignment: defensible-choice-between-
  applicables categorized as precondition violation.
- The done signal's own body had explicitly flagged the follow-up ("question stays open until the
  answer arrives"), which is the textbook `builds-on` case from the vocabulary — so the blocked
  kind was arguably the *better* narrative fit, not merely a defensible one.
