# Evidence: process-detail leakage in agent-authored narration

## Context

In a sibling SDD-instrumented project, an agent was asked to generate a meeting agenda document for a workshop where open questions previously captured in the graph would be discussed. The questions section read fine. The trailing section, intended to describe what happens after the meeting, drifted into SDD-internal mechanics.

## Verbatim excerpt

```
---

## Nach dem Meeting — Rückführung in den Graph

Pro geklärter Frage:

1. **Done-Signal** mit `--closes <frage-id>` und der konkreten Antwort als Beschreibung.
2. Falls die Antwort eine **neue Anforderung** etabliert (z. B. „Englisch ist MUST"): zusätzliche Direktive oder Update von `workshop-ergebnisse.md` + ggf. Supersede-Kette zu `d-cpt-oye`, falls die MoSCoW-Liste sich verschiebt.
3. Falls die Antwort eine **Bewertungsachse stärker gewichtet** (z. B. „Hand-off = Rückruf primär"): Update des Stufe-2-Templates und der bereits angelegten Anbieter-Profile (Capability D).
4. Stufe-2-Bewertung kann auf Basis der geklärten Achsen zur Top-3-Synthese verdichtet werden.

| Frage | Done-Signal schließt | Mögliche Folge-Captures |
|---|---|---|
| 1 — Volumen | `s-tac-og9` | Fakt-Signal mit Volumen-Annahme; ggf. Direktive zur Bewertungs-Methode |
| 2 — Hand-off | `s-tac-70o` | Direktive zum Phase-1-Hand-off; Update Capability D |
| 3 — Englisch | `s-tac-e05` | MoSCoW-Update; ggf. Direktive die `d-cpt-oye` ergänzt/ablöst |
```

## Two failure modes

**1. Jargon-heavy register.** "Done-Signal mit `--closes <frage-id>`", "Supersede-Kette", "Direktive", "Fakt-Signal", "Folge-Captures" — these are SDD-internal terms. They are precise for the agent and for SDD-fluent readers, but for a meeting recipient they read as cluttered jargon when plain wording would carry the same meaning.

**2. Playbook-move duplication.** The numbered steps and the "possible follow-up captures" column reproduce procedure that already lives in the base skill — when to capture a done signal that closes a question, when a directive is warranted, when a supersede chain might apply. The agent does not need this in the artifact to perform the work after the meeting; the meeting attendees do not need it to participate. The artifact is restating how the agent operates rather than narrating the substance of the meeting.

## Plain-language sketch

The same back-fill intent reads cleanly without leaking the playbook:

> ## Nach dem Meeting
>
> Die Antworten auf die offenen Fragen werden im Anschluss strukturiert festgehalten und mit den ursprünglichen Fragen verknüpft (Fragen 1–3, IDs `s-tac-og9`, `s-tac-70o`, `s-tac-e05`). Falls eine Antwort eine neue Anforderung etabliert oder ein vorhandenes Bewertungsraster verschiebt, wird das ebenfalls dokumentiert und führt zu einer angepassten Top-3-Auswahl.

Entry IDs are still present for precise referencing — that is internal-consistency hygiene, not jargon. What is gone: the procedural how-to that belongs to the agent's behavior.

## Pattern across observations

This is the second concrete instance of register leakage from agents working in SDD-instrumented projects. The first was captured as `s-prc-yg0` — bootstrap onboarding, where framework vocabulary surfaced in user-facing prose during dialogue. Together they suggest a single root: the skill teaches the agent the framework's working model, and that working model bleeds through into agent-authored narration unless the skill specifies that outer narration stays in plain language.
