---
type: signal
layer: process
kind: fact
override: closed
confidence: high
topics:
    - engine/base-facts
    - type-system/kinds
summary: >-
    The actor kind records who a participant is — a declared identity, human
    or not, carried by a canonical name matched to what the graph already
    uses and introduced readably in the prose, with aliases collected in the
    same capture — evolving by supersession within its chain and retired by
    a directive whose closure cascades to the identity's roles.
---

# Declaring a participant — the actor kind

An actor records who a participant is: a person, a team, a model family — any identity the graph attributes work to. It is a signal — an identity declared, not something committed to — and declaration is the point: participants are declared, never inferred, and a name with no actor entry behind it is not a participant the graph recognizes. Its reader arrives asking three things — who is this, what name does the graph know them by, and which other names resolve to them — and a good actor is the shortest record that answers all three.

**The canonical is the name already in use.** The canonical is the stable name this participant is known by across the graph — the form they would recognize as theirs, matched to the name work is already attributed under, never a more formal variant invented for the record: where the project already says "Marta", the canonical is "Marta", not the full name from her papers. Before drafting at all, check whether an identity already holds this person — resolve the active identities and their aliases first, because a known participant needs reusing, not reconfirming. And the canonical outlives any single entry: everything binds to the name, not to the entry that introduced it, and once a canonical has named one identity it never names another — attributions written years ago must still resolve to the right participant, so even a departed participant's name is not free for reuse.

**Introduce the canonical in the prose.** One sentence is enough — "Yusuf is a master baker trained in Gaziantep, twenty years at his family's bakery before this one" — but it must be there: a capture that names the canonical only in the entry's fields is an identity record with no readable identity. Bare naming with nothing behind it — "actor: Yusuf" — is the opposite defect: the prose grounds who this is for a future reader with no outside context, through affiliation, background, the expertise they bring from elsewhere. That grounding may also carry what shifts too often to be worth a fresh entry — a job title, an organization, a current focus — mutable context lives in the prose precisely because it changes without the identity changing.

**Identity here, not work here.** The body describes who the participant is independent of the work in this project; what they do inside it is a role, a separate entry bound to the actor. "Marta — trained mezzo, twelve years in the city opera chorus, teaches music at the Gymnasium" is an actor; "Marta selects the repertoire for the winter concert" is her role. People naturally describe a participant in mixed terms, and that is normal — it means two entries, not a blend: draft the actor first and let the role bind to its canonical as it currently stands, because a role never mints a name of its own. Capturing both is the default; deferring the role is an explicit call, never a silent drop of what the person volunteered.

**Aliases travel with the capture.** Collect, in the same capture, the other names this participant appears under — the name they sign work with, go by in conversation, use on outside correspondence — each listed once. An alias is for the reader, never a second canonical: the graph attributes work by the canonical alone, and aliases exist so a reader can resolve a signature or a chat name back to the identity behind it. So when aliases are present, the prose says where each variant appears — "signs invoices as J. Weber, appears in the group chat as Jojo" — making the resolution readable rather than guessed.

**The stable identity over the momentary instance.** Humans and non-humans are actors alike — a person and the model family assisting the project both participate. For anything non-individual, declare the identity that persists: the model family rather than one working session, the team rather than tonight's roster. An identity scoped to a moment forces a new actor every time the moment changes, and attribution scatters across handles that were never really different participants.

**Who participates is how the project works.** An actor describes the project's working fabric, not any piece of the work — so it lives at the process layer, whatever depth the participant's own work runs at. A person doing strategic thinking is not a strategic-layer entry.

**Evolving is superseding; leaving is a directive.** When a fact of the identity itself shifts — the canonical changes, an alias is added or removed, a spelling is corrected — the actor is superseded within its own chain, and keeping the same canonical across a supersede is normal. A first capture naturally points at nothing — the identity arrives from the world, declared; a supersede points at what it replaces. Retirement is different: when the participant is no longer relevant — left the project, will not participate further — a directive closes the actor with the why; a done is a category error, because nothing completed. And closing the identity closes its work-side shadow with it: every role ever bound to any of the identity's names retires in the same stroke, with no separate role closures to write.

**Choosing actor at all.** The neighbor tests the type-system introduction does not run, each one question. Is the description a position held in another frame — CEO of her company, member of the parish council? That is identity here even though it is a role there: what someone is elsewhere grounds who they are, and only what they do within this project is a role. Is the name just someone who took part once — a guest quoted in one meeting's minutes? Listing them among that entry's participants records the taking part; the actor entry is for a name the graph will attribute to from now on — and even then, routine participation never warrants pointing an entry at the actor itself, because the listed name already carries the identity reference. For the outside-versus-here test itself, pull the type-system introduction.

{{ .Mechanics }}
