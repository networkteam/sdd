package model

// ShapeParticipantsBlock is the result shape produced by sourcing actor
// entries terminated by `as-participants-block` — the Participants block
// rendered by `sdd view --layout='participants'` (one group per active
// actor canonical, with derived-active roles bound to that chain).
const ShapeParticipantsBlock RenderShape = "participants-block"

// ParticipantsBlock is the as-participants-block-bound SectionData
// variant. Groups appear in source order — the finder's actor-head walk
// preserves active-actor ordering.
type ParticipantsBlock struct {
	Groups []ParticipantsGroup
}

// ParticipantsGroup couples one active actor head with the derived-active
// roles bound to its chain. Kept as a separate model type so view-layer
// rendering doesn't depend on query types and the SectionData contract
// holds at the model boundary.
type ParticipantsGroup struct {
	Actor *Entry
	Roles []*Entry
}

// Shape implements SectionData.
func (ParticipantsBlock) Shape() RenderShape { return ShapeParticipantsBlock }

// Count implements SectionData: the number of participant groups produced.
func (p ParticipantsBlock) Count() int { return len(p.Groups) }
