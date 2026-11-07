export type EntryType = "signal" | "decision";

export interface Kind {
  id: string;
  name: string;
  type: EntryType;
  // Verbatim from kindQuestions in internal/basefacts/overview.go.
  question: string;
}

export const kinds: Kind[] = [
  { id: "gap", name: "Gap", type: "signal", question: "What needs attention?" },
  { id: "fact", name: "Fact", type: "signal", question: "What do we know?" },
  { id: "question", name: "Question", type: "signal", question: "What do we not know?" },
  { id: "insight", name: "Insight", type: "signal", question: "What have we synthesized?" },
  { id: "done", name: "Done", type: "signal", question: "What was accomplished?" },
  { id: "actor", name: "Actor", type: "signal", question: "Who is participating?" },
  { id: "annotation", name: "Annotation", type: "signal", question: "What structure lies over other entries?" },
  { id: "directive", name: "Directive", type: "decision", question: "Which way do we go?" },
  { id: "activity", name: "Activity", type: "decision", question: "What's next to do?" },
  { id: "plan", name: "Plan", type: "decision", question: "What must be true when done?" },
  { id: "aspiration", name: "Aspiration", type: "decision", question: "What are we pulling toward?" },
  { id: "focus", name: "Focus", type: "decision", question: "What are we attending to in this period, and who is engaged?" },
  { id: "role", name: "Role", type: "decision", question: "How does an actor participate here?" },
  { id: "procedure", name: "Procedure", type: "decision", question: "How does a playbook move run?" },
];

export const kindById = (id: string): Kind => {
  const kind = kinds.find((k) => k.id === id);
  if (!kind) throw new Error(`unknown entry kind: ${id}`);
  return kind;
};
