package model

// ShapeParticipantsBlock is the result shape produced by sourcing actor
// entries terminated by `as-participants-block`. Mirrors the Participants
// section in `sdd status` (one group per active actor canonical, with
// derived-active roles bound to that chain) so view-side rendering reuses
// the same visual contract.
const ShapeParticipantsBlock RenderShape = "participants-block"

// ParticipantsBlock is the as-participants-block-bound SectionData
// variant. Groups appear in source order — the finder's actor-head walk
// preserves the active-actor ordering used by `sdd status`.
type ParticipantsBlock struct {
	Groups []ParticipantsGroup
}

// ParticipantsGroup couples one active actor head with the derived-active
// roles bound to its chain. Mirrors query.ParticipantGroup for status —
// kept as a separate model type so view-layer rendering doesn't depend on
// query types and the SectionData contract holds at the model boundary.
type ParticipantsGroup struct {
	Actor *Entry
	Roles []*Entry
}

// Shape implements SectionData.
func (ParticipantsBlock) Shape() RenderShape { return ShapeParticipantsBlock }
