# Kōgen Coffee — A Story

A practical exploration of the Signal → Dialogue → Decision framework, told through a small team building a product.

## Cast

- **Mara** — co-founder, business and brand
- **Jun** — head roaster, coffee domain expert
- **Priya** — developer
- **Customers** — real people, early adopters

## Background

Priya built Kōgen's website a few months ago using a system called SDD — Signal, Dialogue, Decision. It's a framework for working with agents where decisions are captured as small immutable documents in a Git-based graph, instead of specs or tickets. It was a bit of a trend in the dev community and Priya wanted to try it. The website turned out well. Mara made brand decisions when Priya asked her — colors, tone of voice, what photos to use — but she interacted with Priya, not with the system directly.

---

## Scene 1: Morning Coffee

Every morning the Kōgen team gathers at the bar before opening. It's a ritual — Jun pulls espresso shots, they talk about the day, about coffee, about whatever.

**Jun**: "So that guy from Portland — the one who loved the Sidama — he asked again if he could get beans shipped. Third person this week asking something like that."

**Mara**: "I keep hearing this too. People want our stuff beyond the three shops. I've been thinking about it — what if we did some kind of subscription? Like, ship people a discovery every month."

**Jun**: "Not just beans in a bag though. When I hand someone a cup of the Sidama, I tell them the story — where it's from, why I roasted it this way, what to listen for in the flavor. That's what makes it special."

**Mara**: "Absolutely. I've seen some roasteries doing this — like Nomad in Barcelona, they ship these curated boxes with cards and tasting notes..."

**Jun**: "Yeah, but theirs feels generic. Like a marketing thing. I want people to feel like I'm right there talking to them."

**Priya**: "Have you looked at Kaffebox? They do a subscription where the roaster writes a personal note for each batch. It's pretty popular."

**Mara**: "Right, I've seen that. But they're basically a marketplace — lots of roasters, one platform. That's not us. We'd be doing our own thing, which means we'd need ordering, shipping, payments, content for every release... that's a lot more than a website."

**Mara**: "So... should we do this? Priya, do you think you'd have time?"

**Priya**: "Honestly — I've got the loyalty card system and the point-of-sale integration coming up. I could carve out some time, but I'd need to know what exactly we're building. Like, is this a webshop? An app? A subscription platform with its own logistics?"

**Mara**: "I think it's a subscription. Monthly, maybe? People sign up, they get a curated box..."

**Jun**: "But what's in the box? Just beans? Beans and a card? A whole experience kit?"

**Mara**: "I don't know yet. And what do we charge? Shipping fresh beans is expensive. Do we include shipping or charge extra?"

**Jun**: "And do people choose their beans or do I choose for them? Because if I'm curating, I can only do so many per month. I'm also roasting, remember."

**Priya**: "See, this is the thing. We could spend weeks figuring all of this out before I write a single line of code. And then half of it will be wrong anyway because we don't know what customers actually want."

**Mara**: "But we can't just start building without knowing what we're building."

**Priya**: "Can't we, though? Not build the whole thing — but start with what we know and figure out the rest along the way?"

**Mara**: "What does that even mean practically?"

**Priya**: "Okay — so for the website, I used this approach called SDD. Signal, Dialogue, Decision. The idea is you don't write a big plan upfront. Instead you capture small decisions as you make them, and they link together into a picture of what you're building. When a question comes up — like 'what's in the box?' — you explore it, decide, and record that. The decisions build on each other."

**Jun**: "That sounds like what we're doing right now. Talking."

**Priya**: "It kind of is! But right now this conversation disappears after we walk away. The point is to capture these things so we don't go in circles. And I can set it up so you don't have to go through me every time. You'd each have a chat interface — Mara, you could explore pricing questions, talk through the business model. Jun, you could describe what a discovery should feel like, what goes into a box. The system captures what you figure out as decisions."

**Mara**: "And then what? You build what's in there?"

**Priya**: "I build what we've decided, yes. But I also make decisions in my area — the technical stuff. Which payment system to use, how the pages work. You wouldn't need to care about those unless they affect the product. And if a question comes up that's not mine to answer, I flag it and it finds its way to whoever knows."

**Jun**: "So I'd just... talk to it? About coffee?"

**Priya**: "Pretty much. Like you're talking to me now, but it remembers and connects things."

**Jun**: "Hmm. I'll try it. But if it starts asking me about databases I'm out."

**Mara**: "And I can see what everyone's decided? So I'm not guessing what Priya's building?"

**Priya**: "Exactly. Everything's visible. What's decided, what's still open, who decided what."

**Mara**: "Okay. Let's try it. Worst case we're back here next week talking about it again anyway."

That evening, Priya creates a new SDD project.

**System**: "Welcome to SDD. I'll help you set up this project so we can start capturing decisions and making progress. Let's begin with the basics — what's the situation? What are you trying to do, and what triggered it?"

**Priya**: "We're a small specialty coffee roastery called Kōgen — three shops in the city. Customers keep asking if they can get beans shipped, especially to share with friends in other cities. We talked about it this morning and there's energy around building some kind of subscription or discovery box."

**System**: "Interesting. So there's real demand signals from customers, and the team is motivated. What makes Kōgen's offering different from other coffee subscriptions? There are quite a few out there."

**Priya**: "Jun, our head roaster — he has this way of presenting coffee. It's not just beans. He tells you the story, why he chose this origin, what he was going for with the roast. That personal touch is what people love. The question is whether we can put that in a box."

**System**: "That's a strong starting signal. So the core hypothesis is that Kōgen can deliver Jun's personal curation experience beyond the physical shops, through a shipped product. A few things are open: the format (subscription vs. one-off), what's actually in the package, and whether Jun's voice can scale. Want me to capture this as the initial direction?"

**Priya**: "Yes. But mark it as exploratory — we don't know the shape yet. Mara's interested in the business model, Jun cares about the experience staying authentic."

**System**: "Okay, I think I have a picture of the starting point. Let me play back what I'd capture as the initial signals and direction. Tell me if this looks right:

Signals I'm seeing:
- Customers are repeatedly asking about shipping beans, especially to friends in other cities — this is a recurring pattern, not a one-off
- There's an opportunity to reach beyond your three shops with a direct-to-consumer product
- Jun's personal curation — the stories, the roasting intent, the tasting guidance — is what makes Kōgen different. Any product only works if that comes through

And from all of that, I'd capture this as your first strategic decision:

*We will explore a direct-to-consumer offering. The core idea is sharing coffee discoveries — not a commodity subscription box, but something that carries Jun's curation and story. Format, pricing, and content are all open.*

I'd mark confidence as medium since there's conviction about the direction but a lot still to figure out. Does that capture it?"

**Priya**: "Yeah, that's pretty much it. Change 'opportunity' to something more concrete — it's Mara who's been thinking about this for a while, not just a generic opportunity."

**System**: "Got it — I'll note that Mara has been considering this direction and the customer requests are confirming her instinct. Captured. I've set up access for the project — want to invite Mara and Jun so they can start their own conversations?"

Priya shares access with Mara and Jun. The underlying entries are structured with IDs and references — `20260405-084500-s-stg-j3n`, `20260405-090000-d-stg-f3a` — but everyone interacts through conversation. The graph builds itself.

---

## Scene 2: Finding the Shape

The next morning, Mara opens the chat for the first time. The system greets her and catches her up.

**System**: "Hi Mara. Priya set up this project last night based on your morning conversation. Here's where things stand: you've decided to explore a direct-to-consumer discovery offering built around Jun's curation. Confidence is medium — the direction feels right but the shape is wide open.

There are three big open questions I'd suggest exploring:
1. **Business model** — subscription vs. one-off gifts vs. something hybrid. What works economically with shipping fresh beans?
2. **The discovery experience** — what actually goes in the box? How does Jun's voice come through in a shipped product?
3. **Technical feasibility** — how simple can the first version be? What's the fastest path to getting something real in front of people?

You probably have the best perspective on #1. Jun would own #2. Priya is already thinking about #3. Want to start exploring the business model?"

**Mara**: "Yes, let's do that. But not now — I need to open the shop in twenty minutes."

**System**: "No rush. Pick it up whenever you have time — I'll be here with the context ready."

She closes the chat and goes about her day. Supplier call at noon, inventory in the afternoon, a problem with the espresso machine at the second location. The subscription idea stays in the back of her mind — she notices a regular customer buying beans "for a friend in Munich" and makes a mental note.

That evening after closing, she sits down with her phone and picks up the thread. "Okay, I'm back. Let's talk about shipping. What would it actually cost to ship fresh-roasted beans? What do other specialty subscriptions charge? And what if someone just wants to send a one-time gift?"

**System**: "I can look into that. For shipping costs, I'd want to check actual rates from DHL and DPD for small package weights — probably 250g to 500g. For subscription pricing, I can research what Kaffebox, Nomad, and a few others charge and what's included. Want me to do that?"

**Mara**: "Yes, do it."

The system researches and comes back with specifics — actual carrier rates with sources, competitor pricing from their public pages, and a rough margin model. Mara pushes back on some of it ("DPD is cheaper but their tracking is terrible, our customers would complain"), asks follow-ups ("what if we roast to order instead of keeping inventory?"), and challenges an assumption about packaging costs ("that doesn't include the branded box Jun wants").

Through this back and forth, a picture forms:

> **20260406-211500-s-cpt-r4w**: Shipping economics strongly favor subscription over one-off gifts. A single shipment costs €4.50-5.80 (DHL, 500g package) — viable in a €25+ subscription but painful for a €15 one-off gift. Fresh-roasting to order reduces waste but adds 2-3 days to fulfillment.

**System**: "I want to flag something. A review of the decision graph found a tension between this research and your existing strategic direction. The strategic decision says the core idea is *sharing discoveries* — sending something to a friend, which is naturally a one-off act. But your research shows the economics strongly favor subscriptions. The thing that makes financial sense and the thing that fits your identity aren't obviously the same. I'd recommend bringing this to the whole team — it's a conceptual question that touches both the business model and the product identity."

**Mara**: "Hmm. You're right, I can't resolve that on my own. Let's bring it up tomorrow morning."

Behind the scenes, this is a separate review agent at work — not the same one that helped Mara research shipping costs. Its job is to continuously check new signals against existing decisions for coherence. When Mara's shipping signal landed in the graph, it noticed the conflict with the strategic decision and surfaced it through Mara's chat, since she was the one whose work triggered it. This is the coverage mechanism in action: not just tracking what's been decided, but watching for contradictions as new information arrives.

**Jun** is cleaning up the roastery that evening — wiping down the Probat, sweeping chaff off the floor. He's got his AirPods in. Normally he'd listen to a podcast, but today he's been thinking about what a discovery box would actually contain. He activates the chat interface in voice mode.

**System**: "Hey Jun, welcome. A few things happened since yesterday morning — want me to catch you up?"

**Jun**: "Yeah, go ahead."

**System**: "Priya captured the direction from your morning conversation — you're exploring a direct-to-consumer offering built around your curation. She flagged that the discovery experience is your area to shape. Mara hasn't started on the business model yet. The big open question for you is: what would a discovery actually contain? What makes it feel like you're there talking to the customer?"

"Yeah, so I've been thinking about exactly that all day. If I were putting together a discovery for the Sidama... when I roast this one, I'm going for that bright berry thing but with enough body that it works as a filter too. And the story — this lot comes from a washing station where..." He narrates the way he'd tell a customer at the counter, hands busy with the cleaning. The system listens and helps him see the structure in what he already knows: origin story, roasting approach, flavor notes, brewing recommendation.

> **20260407-073000-s-cpt-a5x**: A discovery package is four elements: origin story, Jun's roasting intent, tasting notes, and a specific brewing recommendation. Jun can produce these naturally by narrating — the bottleneck isn't creation, it's that each one is specific to the bean and needs his voice.

**Priya** opens a session that same evening. The system catches her up — Jun's been in, Mara's started on the business side.

**Priya**: "Okay, I want to figure out the technical shape. How simple can the first version be?"

**System**: "Based on what's in the graph so far: Jun's defined the discovery as four content elements, the strategic direction is sharing-first. The big open questions on your side are: what's the platform, how do payments work, and how does the share mechanic work technically. You could research these or — given that the components are pretty standard — you might learn more by building a rough prototype. A real thing to look at would also give Jun and Mara something concrete to react to, instead of discussing in the abstract."

**Priya**: "Yeah, let's build something. Let me think about the technical decisions first."

She works through the choices in dialogue with the system — not a long planning session, but small decisions with reasoning:

**Priya**: "Plain web pages, not an app. Server-rendered, fast, no JavaScript framework needed for this."

**System**: "Makes sense for the scope. Captured as a technical decision?"

**Priya**: "Yes. And Stripe for payments — it handles subscriptions natively, and the API is straightforward. For sharing, the simplest thing is a unique link per share. Recipient gets a page with the discovery content and can order a tasting package."

> **20260407-143000-d-tac-t4n**: Web-based, server-rendered pages. No app, no frontend framework. Fast and simple for the current scope. Refs: 20260405-090000-d-stg-f3a.

> **20260407-143500-d-tac-r6b**: Stripe for subscription billing. Handles recurring payments and supports shipping to different addresses for gift shares. Refs: 20260405-090000-d-stg-f3a.

> **20260407-144000-d-tac-w2m**: Share mechanic via unique links. Each subscriber gets a shareable URL per discovery. Recipient sees the discovery page and can order a tasting package. Refs: 20260405-090000-d-stg-f3a.

**System**: "That's a clear enough scope to build against. Want to start the prototype? I'd suggest: one discovery page with placeholder content standing in for Jun's Sidama, a Stripe checkout for subscriptions, and the share link flow. Minimal, but enough to see if the pieces fit."

> **20260407-145000-d-tac-k8p**: Build a prototype: one discovery page, Stripe subscription checkout, share-via-link flow. Goal is to test feasibility and have something real for the team to react to. Refs: 20260407-143000-d-tac-t4n, 20260407-143500-d-tac-r6b, 20260407-144000-d-tac-w2m.

The prototype decision has sub-decisions at the operational layer — the individual steps to build it:

> **20260407-150000-d-ops-a3n**: Build discovery page with placeholder content standing in for Jun's Sidama. Refs: 20260407-145000-d-tac-k8p.

> **20260407-150100-d-ops-b7q**: Integrate Stripe subscription checkout. Refs: 20260407-145000-d-tac-k8p, 20260407-143500-d-tac-r6b.

> **20260407-150200-d-ops-c4s**: Build share-via-link flow: generate unique link per subscriber, recipient sees discovery page. Refs: 20260407-145000-d-tac-k8p, 20260407-144000-d-tac-w2m.

> **20260407-150300-d-ops-d9t**: Deploy to staging. Refs: 20260407-145000-d-tac-k8p.

The implementation agent starts working from the sub-decisions. Each one references the scope decisions — the agent has everything it needs without Priya directing it step by step.

The first step completes without issues:

> **20260407-163000-a-ops-f3v**: Built discovery page. Placeholder content for Jun's four elements (origin story, roasting intent, tasting notes, brewing recommendation), basic layout, responsive. Refs: 20260407-150000-d-ops-a3n.

The second step hits a snag. The implementation can't fulfill the decision as specified — the system stops the build and surfaces the issue:

> **20260407-170000-a-ops-g6w**: Started Stripe integration. Stripe's embedded checkout doesn't support the subscription-with-gift-to-different-address flow described in 20260407-143500-d-tac-r6b. Stopped by system — decision cannot be fulfilled as specified. Refs: 20260407-150100-d-ops-b7q.

> **20260407-170500-s-ops-x3p**: Stripe's embedded checkout can't handle shipping to a different address for gift shares. Stripe's hosted checkout can — but it redirects the customer away from our page. This is a trade-off between integration depth and simplicity. Refs: 20260407-150100-d-ops-b7q, 20260407-143500-d-tac-r6b.

The system surfaces this to Priya:

**System**: "The Stripe integration hit an issue — embedded checkout can't handle the gift shipping case. Hosted checkout works but redirects customers off-site. This is a trade-off between a seamless experience and getting something working now. The share link flow depends on this, so it's blocked until this is resolved."

Priya doesn't need long to think. It's a prototype — polish comes later.

> **20260407-171500-d-ops-n4q**: Use Stripe hosted checkout for now. The redirect is acceptable for a prototype. Revisit if it becomes a problem with real users. Supersedes: 20260407-150100-d-ops-b7q. Refs: 20260407-170500-s-ops-x3p.

The system checks downstream: the share link flow referenced the Stripe decision, but it only needs a subscription ID to generate share links — the checkout method doesn't affect it. Downstream steps are unblocked.

The agent continues:

> **20260407-174000-a-ops-g7w**: Integrated Stripe hosted checkout. Subscription flow works, redirects to Stripe then back. Refs: 20260407-171500-d-ops-n4q.

> **20260407-183000-a-ops-h2x**: Built share link flow. Unique URL generated per subscriber per discovery. Recipient sees the discovery page. Refs: 20260407-150200-d-ops-c4s.

> **20260407-190000-a-ops-j8y**: Deployed to staging. All pieces running. Refs: 20260407-150300-d-ops-d9t.

**System**: "Prototype is live on staging. All four steps are done. Want to evaluate? You could check the technical side yourself, but the strategic direction says this should feel personal and story-driven — that's probably worth getting Mara's and Jun's eyes on too."

**Priya**: "Good call. Let me do a technical check first, then let's get the others to look at the product side."

> **20260407-191000-d-tac-e5h**: Evaluate prototype. Technical feasibility check by Priya. Brand and product experience review needed from Mara and Jun. Refs: 20260407-145000-d-tac-k8p, 20260405-090000-d-stg-f3a.

Priya clicks through the whole flow — landing page, discovery, checkout, share link, recipient view.

**Priya**: "Technically this works. The pieces fit together, Stripe hosted checkout is fine, the share links generate correctly. But the share recipient page is a dead end — you can see the coffee but there's no way to subscribe or order. That needs solving."

> **20260407-193000-a-tac-n7w**: Priya reviewed the prototype end-to-end. Refs: 20260407-191000-d-tac-e5h.

> **20260407-193500-s-tac-h8c**: Technically the pieces fit. Complexity is low, no app needed, Stripe hosted checkout works well enough. The foundation is solid. Refs: 20260407-191000-d-tac-e5h.

> **20260407-194500-s-tac-v6k**: The share link works but the recipient lands on a dead end — they can see the discovery but can't subscribe or order their own. Refs: 20260407-191000-d-tac-e5h.

Since the evaluation decision names Mara and Jun as needed reviewers, the system notifies them through their chat interfaces:

**System** (to Mara): "Priya built a first prototype and reviewed the technical side — it works. But the evaluation also calls for a brand and product experience review, which is your area. Here's the staging link: [link]. Have a look when you get a chance and let me know what you think."

**System** (to Jun): "Priya has a first prototype up — it uses placeholder content where your discovery would go. Worth a look to see if the structure works for the kind of content you've been describing: [link]."

Mara checks it on her phone that evening:

**Mara**: "I see what Priya built. The technology works, okay. But this doesn't feel like us at all. It looks like any generic subscription box site. The placeholder content doesn't help, but even the layout — it's too transactional. Where's the warmth? Where's the story? When you walk into our shop, the first thing you see is Jun's chalkboard with tasting notes. This should feel like that, not like an online checkout."

> **20260407-212500-a-cpt-v8n**: Mara reviewed the prototype from a brand and product experience perspective. Refs: 20260407-191000-d-tac-e5h.

> **20260407-213000-s-cpt-q2r**: The prototype feels like a generic webshop, not a personal discovery experience. The layout is too transactional — lacks the warmth and story-first feel that defines Kōgen in person. Placeholder content contributes to this but isn't the only issue — the visual hierarchy prioritizes buying over discovering. Refs: 20260407-212500-a-cpt-v8n, 20260405-090000-d-stg-f3a.

Jun checks the staging link the next morning before the others arrive, while preheating the Probat.

**Jun**: "Okay, I looked at the prototype. The structure actually makes sense to me — origin story at the top, then roasting notes, tasting notes, brewing recommendation. That's exactly how I'd walk someone through a discovery. The problem is the placeholder text. 'Lorem ipsum' where the story should be — of course it feels generic. Put my real Sidama narration in there and I think it'll feel completely different. The bones are right."

> **20260408-064500-a-cpt-t3k**: Jun reviewed the prototype from a content and discovery experience perspective. Refs: 20260407-191000-d-tac-e5h.

> **20260408-065000-s-cpt-w7m**: The page structure matches how Jun naturally presents a discovery — the four content sections are in the right order. The generic feel is primarily a content problem, not a layout problem. Real narrated content should transform the experience. Refs: 20260408-064500-a-cpt-t3k, 20260407-073000-s-cpt-a5x.

Now there's a conflict in the graph. Mara's signal says the layout itself is too transactional — the visual hierarchy prioritizes buying over discovering. Jun's signal says the structure is right and the content will fix the feel. They're both looking at the same prototype, both evaluating against the same strategic direction, and reaching different conclusions.

The system surfaces this to all three:

**System** (to all): "Two conflicting signals from the prototype evaluation. Mara sees a layout problem — the page feels like a shop, not a discovery. Jun sees a content problem — the structure is right but placeholder text makes it feel generic. These point to different fixes: redesign the layout vs. add real content first. This probably needs a conversation."

Overnight, a review agent also checks the prototype against existing decisions:

> **20260407-220000-s-tac-m4c**: Review: the share flow decision (20260407-144000-d-tac-w2m) specified "recipient sees discovery page and can order a tasting package." The implementation delivers the first part but not the second — no ordering path for recipients. This is a gap between decision and action. Refs: 20260407-191000-d-tac-e5h, 20260407-144000-d-tac-w2m, 20260407-183000-a-ops-h2x.

None of these signals require anyone to update a plan or reassign a task. They're in the graph — honest, specific, and waiting for the morning.

---

## Scene 3: Coffee Round Decisions

The system catches everyone up as they start their day. Priya's session opens with:

**System**: "Morning. The graph has moved since yesterday. Here's the picture:

**Prototype** is live on staging. Technically solid, but there are conflicting signals about the experience:
- Mara says it feels like a generic webshop — the layout prioritizes buying over discovering.
- Jun says the structure is right — it matches how he naturally presents a discovery. He thinks real content will transform it.
- A review check found a gap: recipients can see the discovery but can't subscribe or order.

**Business model**: Still an unresolved tension — sharing feels one-off but subscriptions make financial sense.

Three things to discuss: the layout conflict, the business model tension, and the share flow gap."

At the bar, the conversation covers all three.

First, the business model tension — Mara has an idea that's been forming overnight:

**Mara**: "What if the subscription IS the sharing? You subscribe, you get a discovery every month, and part of your subscription is you can send one to a friend."

**Jun**: "Oh, I like that. You're not buying a gift. You're sharing from your own thing. 'I got this, you have to try it.' That's how people actually talk about coffee."

**Priya**: "That's basically what the prototype already does, just with the framing flipped. Subscription with a share feature built in."

Mara records it:

> **20260408-091200-d-cpt-b2k**: The product is a discovery subscription with sharing built in. Each delivery includes one "share" — a link the subscriber can send to someone, giving them a tasting package of the same discovery. This resolves shipping economics (subscription covers base cost) while preserving the sharing-first identity. Refs: 20260405-090000-d-stg-f3a, 20260406-211500-s-cpt-r4w.

Then the layout conflict — the conflicting signals from Mara and Jun that the system flagged last night:

**Mara**: "I'm not saying the structure is wrong. I'm saying the whole page feels like a checkout. Even with great content, if the first thing you see is a price and an 'Add to cart' button..."

**Jun**: "But that's not what I mean either. I'm saying the story IS the product. If the content is strong enough, people won't even notice the buy button. It's like in the shop — I don't 'sell' them the coffee. I tell them about it and they want it."

**Mara**: "Okay, so we actually agree? The story needs to dominate. The question is whether the current layout lets it, or whether it fights against it."

**Jun**: "I honestly don't know. I can't tell from placeholder text."

**Priya**: "So let's not guess. We can branch this — build two versions in parallel. One with Jun's real content on the current layout. Another where I redesign the layout to lead with the story — bigger text, no sidebar, the checkout pushed below the fold. Both get Jun's content. We look at them side by side."

**Mara**: "How long would that take?"

**Priya**: "A few hours each. The agents do the building, I just define the direction."

**Jun**: "Let's do it. I'd rather look at two real options than keep talking about it. I already narrated the Sidama the other night — the system has it structured, I just need to clean it up."

> **20260408-093000-d-tac-h6n**: Branch experiment: test two approaches to resolve conflicting layout signals. Branch A: current layout + Jun's real Sidama content. Branch B: story-first layout redesign + Jun's real content. Evaluate both against brand feel and strategic direction. Refs: 20260407-213000-s-cpt-q2r, 20260408-065000-s-cpt-w7m.

And the share flow gap:

> **20260408-093500-d-tac-r5f**: Fix the share recipient dead end — add a path for recipients to subscribe or order their own discovery. This is independent of the layout experiment and goes straight into main. Refs: 20260407-194500-s-tac-v6k, 20260407-220000-s-tac-m4c.

Over the next two days, things move in parallel. Jun cleans up his Sidama content:

> **20260408-180000-a-cpt-j2w**: Jun finalized the Sidama discovery content — origin story, roasting intent, tasting notes, and a specific pour-over recommendation. Reviewed and cleaned up from his earlier voice narration. Refs: 20260408-091200-d-cpt-b2k, 20260407-073000-s-cpt-a5x.

Priya's agents build both branches:

> **20260409-140000-a-tac-k4a**: Built Branch A: current layout with Jun's real Sidama content replacing placeholders. Deployed to staging. Refs: 20260408-093000-d-tac-h6n, 20260408-180000-a-cpt-j2w.

> **20260409-180000-a-tac-n7b**: Built Branch B: story-first layout redesign — origin story as full-width hero, Jun's narrative in large readable text, tasting notes as a visual section, checkout below the fold. Jun's real content. Deployed to staging. Refs: 20260408-093000-d-tac-h6n, 20260408-180000-a-cpt-j2w.

The team looks at them side by side the next morning.

**Jun**: "Okay... Branch A is better than I expected. The real content does change the feel. But Branch B — that's it. That's what it should feel like. The story pulls you in."

**Mara**: "Agreed. Branch B. It's not even close when you see them next to each other. The layout matters — Jun, you were right that the content transforms it, but I was right that the structure was fighting it."

> **20260410-085000-d-cpt-p8m**: The story-first layout (Branch B) is the direction. Leads with origin story, Jun's narrative prominent, checkout below the fold. This resolves the layout conflict — both real content AND story-first layout are needed. Refs: 20260408-093000-d-tac-h6n, 20260409-140000-a-tac-k4a, 20260409-180000-a-tac-n7b, 20260407-213000-s-cpt-q2r, 20260408-065000-s-cpt-w7m.

> **20260409-200000-a-tac-q3c**: Built subscribe-from-share path on main — recipients can now subscribe or order their own discovery from the share page. Refs: 20260408-093500-d-tac-r5f.

> **20260410-160000-a-tac-p5r**: Priya merged Branch B into main. Story-first layout combined with the share flow fix already there. Deployed to staging. Refs: 20260410-085000-d-cpt-p8m.

**Mara**: "Now I want real people to see this. I have a list of 30 customers who'd try anything we put out. But wait — how does this actually go live? Who decides when it's ready?"

**Priya**: "Right now, main is live. Anything merged to main is what customers see. The branches held the layout experiments until we evaluated them — that way they didn't block the share flow fix, which went straight to main."

**Mara**: "So if Jun updates his discovery content for the next batch...?"

**Priya**: "That can go live immediately. Jun owns the content — if he's happy with it, it goes in. For functionality changes — new features, changes to the checkout flow, things that could break — I review before it merges. The agents build on branches, we evaluate, then merge."

**Jun**: "So I don't need to ask permission to update tasting notes?"

**Priya**: "No. It's your domain. The system knows that from the contracts we've been building up."

> **20260410-170000-d-prc-x2g**: Release process: main branch is live. Content changes (Jun's domain) can merge directly. Functionality changes require Priya's review before merging to main. Feature work happens on branches and merges after evaluation. This keeps changes flowing without blocking each other. Refs: 20260410-085000-d-cpt-p8m.

This wasn't planned upfront. It emerged from the question "how does this go live?" — and the answer fell naturally out of the contracts already forming. The release decision references the domain contracts, and the system can now enforce them: content merges flow through if Jun approved, functionality merges require Priya's sign-off.

> **20260412-140000-a-stg-m8k**: Mara sent personal invitations to 28 customers from her list, offering early access to "something new we're trying." Refs: 20260408-091200-d-cpt-b2k.

Five days from "should we do this?" to real customers testing a real product. The graph has 20-some entries — signals, decisions, actions — each small, each linked. Nobody wrote a spec or planned a sprint. The prototype's initial "failure" was the most useful thing that happened.

---

## Scene 4: Questions That Find the Right Person

With the product taking shape, the remaining work generates questions that find their way to whoever can answer them.

Now that the subscribe-from-share path exists, the implementation agent hits a conceptual question: when a subscriber sends a share, should they be able to add a personal message? And if so, how prominent should it be relative to the coffee story? The agent flags this — it can't make a product experience decision.

The system routes it to Jun, since his domain covers the discovery experience:

**System** (to Jun): "A question came up while refining the share flow. When a subscriber shares a discovery, should they be able to add a personal note to the recipient? And how should it relate to the coffee story? This is about the experience, not the tech — your area."

Jun thinks about it the way he thinks about handing someone a cup at the counter.

**Jun**: "The coffee story is always the main thing. That's what you're sharing. But the sender should be able to add a short note — like a handwritten card. 'You have to try this one.' Keep it optional, keep it short."

> **20260411-193000-d-cpt-n6v**: The share experience leads with the full discovery story (origin, notes, brewing recommendation). The sender can optionally add a short personal note — secondary to the coffee story, like a handwritten card. Refs: 20260408-091200-d-cpt-b2k.

Jun recorded this himself. It's his domain. The implementation agent picks it up and continues.

A few hours later, Mara is working on launch messaging and runs into her own question: what should shipping cost the customer?

**Mara**: "Has anyone decided what we're charging for shipping?"

**System**: "No decision on shipping pricing yet. You have research from earlier — DHL at €4.50-5.80 for a 500g package. The subscription is €29/month based on your margin modeling. Want to explore the options?"

She works through it with her agent, modeling different price points:

> **20260411-214000-d-cpt-t6j**: Shipping is included in the subscription price (€29/month covers beans + shipping). No separate shipping line item — it feels like a gift, not an e-commerce transaction. One-off gift shares have a flat €4.50 shipping fee. Refs: 20260408-091200-d-cpt-b2k, 20260406-211500-s-cpt-r4w.

Nobody assigned Mara this decision. She hit it because she was doing her work and recognized it as hers to make.

Meanwhile, a review agent examining the implementation flags something:

> **20260411-161000-s-tac-f2a**: The customer login flow uses email-based magic links. No password. This is simple but means customers can't manage their subscription without access to their email. Worth a decision.

Priya sees it. She could decide this herself — it's tactical, it's her domain:

> **20260411-170000-d-tac-c8w**: Keep magic link login. No passwords. At our scale, email access is a safe assumption. Revisit if customer support signals indicate problems. Refs: 20260411-161000-s-tac-f2a.

This is how contracts form naturally. Through working, a pattern emerges. And at this point, the system notices it:

**System** (to all): "Looking at the decisions so far, a clear pattern has formed in who decides what. Jun has made all content and experience decisions. Mara has made all business model and pricing decisions. Priya has made all technical decisions. When a question crosses domains, it's been routed to the person with knowledge. Should I capture this as an explicit contract? It would help me route future questions and enforce review requirements."

**Mara**: "Makes sense. That is how we've been working."

**Jun**: "As long as it's not set in stone. If I have an opinion about pricing, I want to say so."

**Priya**: "That's the point — it's about who *decides*, not who *participates*. Anyone can weigh in. The contract just says who has the final call."

> **20260412-090000-d-prc-y5d**: Domain contracts: Jun has authority over content, curation, and discovery experience. Mara has authority over business model, pricing, and brand. Priya has authority over technical decisions. All participants can contribute signals and participate in dialogue across domains. These contracts define decision authority, not participation boundaries. Refs: 20260405-090000-d-stg-f3a.

The contract is now explicit and in the graph — referenceable by the system for routing and review enforcement, and challengeable by anyone through a new signal if circumstances change.

---

## Scene 5: The First Signals from Reality

Two weeks in, they soft-launch to 28 customers. Real people, real money, real beans.

The good news comes fast. Share feature adoption is high — most subscribers use it within days. The share-to-subscribe path works; a few recipients convert. Jun's tasting notes are the most-read section on every discovery page. The core hypothesis — people want to share discoveries — is confirmed.

But the interesting signals are the ones nobody anticipated.

Jun talks to customers at the shop:

> **20260420-173000-s-ops-c2j**: Talked to Lena today. She loved the Sidama, drank through it in a week, and wants to order more of the same batch. There's no way to do that — the subscription just sends whatever the next discovery is. She asked "can I just buy more of this one?"

> **20260422-114000-s-ops-n4t**: Marco wants to pause his subscription for a month — he's traveling and still has beans from last time. No pause option exists. He asked if he should just cancel and re-subscribe later.

Mara gets a phone call:

> **20260421-161000-s-ops-j7w**: A subscriber (Thomas) switched email providers and can't log in anymore — magic links go to his old address. He called the shop directly to ask for help. Mara had to manually look up his subscription in Stripe.

An analytics review agent, running on its contract to monitor user engagement patterns, surfaces a content signal:

> **20260422-090000-s-ops-k4w**: Jun's tasting notes are the most-viewed section on the discovery page. Average time on page: 3 minutes. The brewing guide gets only 20% of views. Source: analytics review.

Next morning at the bar, the signals come together.

**Jun**: "Lena wants to order more Sidama. Makes sense — when you fall in love with a bean, you want more. But we don't have a way to do that."

**Mara**: "And Marco wants to pause. If we force him to cancel, we might lose him. Also — Thomas called me directly because he couldn't log in. He switched email and the magic links don't work anymore."

**Priya**: "The email thing — I made a decision about that. Magic links, no passwords. The system actually flagged it as a risk at the time, and I said we'd revisit if customer support signals came in. Well... here's the signal."

She pulls up her earlier decision:

**System**: "Decision 20260411-170000-d-tac-c8w: 'Keep magic link login. No passwords. Revisit if customer support signals indicate problems.' The signal from Thomas directly challenges this."

**Priya**: "Okay, I'm not going to rebuild auth right now. But I should at least add a way for people to update their email. That's the minimal fix."

> **20260423-090000-d-tac-c9w**: Add email update flow for subscribers — they can change their login email from within an active session. Doesn't solve the locked-out case fully but prevents it going forward. Revisit auth approach if more signals accumulate. Supersedes: 20260411-170000-d-tac-c8w. Refs: 20260421-161000-s-ops-j7w.

**Mara**: "The pause and reorder things are bigger questions. Pausing is about retention — we don't want people canceling just because they're traveling. And reordering... that's almost a different product. It changes us from pure subscription to something with a shop element."

**Jun**: "I don't want a shop. That's not what this is."

**Mara**: "I agree. But Lena's request is real. Maybe it's not a shop — maybe it's just a 'get more of this discovery' button on the page you already have."

> **20260423-091500-d-cpt-b4k**: Add a subscription pause feature — subscribers can skip one month. This is a retention mechanism. Refs: 20260422-114000-s-ops-n4t.

> **20260423-092000-s-cpt-f6m**: Lena's reorder request raises a conceptual question: should subscribers be able to buy more of a specific discovery? This could be a simple add-on or it could shift the product toward a shop model. Needs careful thought — the strategic direction is discovery/curation, not e-commerce. Refs: 20260420-173000-s-ops-c2j, 20260405-090000-d-stg-f3a.

Notice what happens with the reorder question: Mara captures it as a signal, not a decision. They're not ready to decide. The signal references the original strategic direction — the system will flag it if a future decision conflicts. It sits in the graph, waiting for more data or a conversation that resolves it.

**Jun**: "One more thing — the brewing guide. People aren't reading it. I think it's because it's too generic. If I write 'this bean is best as a pour-over, here's exactly why' instead of a general guide... let me try it for the next discovery."

> **20260423-093000-d-cpt-a2m**: Experiment: replace the generic brewing guide with Jun's specific recommendation per discovery. Hypothesis: engagement increases when the guide feels as personal as the tasting notes. Refs: 20260422-090000-s-ops-k4w.

The rhythm is clear. Signals from real customers arrive in everyone's domain — Jun hears it at the counter, Mara gets the phone call, Priya sees it in the data. The morning conversation connects the dots. Some signals lead to immediate decisions. Some need more exploration. Some sit as open questions. Each person captures what's in their domain. The graph grows honestly — not everything is resolved, and that's fine.

---

## Scene 6: A Harder Decision

A month in, subscriptions are growing. The share-to-subscribe loop is working. Then Mara gets an email that changes the conversation.

> **20260510-091000-s-stg-j8k**: GreenLeaf, a specialty grocery chain (12 locations), reached out. They want to stock Kōgen beans and offer a co-branded subscription through their stores.

Mara's excited — this could be a huge growth channel. She mentions it at the morning coffee round.

**Jun** goes quiet. Then: "Co-branded? What does that mean for us?"

**Mara**: "I don't know yet. Could mean a lot of things. Let me explore it."

She spends the evening in dialogue with her agent, modeling scenarios. Revenue potential, typical partnership terms in specialty food, what GreenLeaf's size means for volume.

> **20260510-201000-s-stg-p3a**: GreenLeaf partnership could mean 80-100 new subscribers through in-store promotion within six months. Revenue roughly 3x current base. Source: modeled from GreenLeaf's store traffic and comparable specialty food partnerships.

> **20260510-203000-s-stg-r8b**: Co-branded specialty food partnerships typically require: partner branding on the experience, input on product selection, volume commitments, and shared customer data. Source: research on comparable partnerships (Kaffebox/retailer, Blue Bottle/grocery).

She also asks the system what existing decisions this might conflict with.

**System**: "This potentially conflicts with several decisions. The original strategic direction (20260405-090000-d-stg-f3a) positions the product as personal and story-driven under Kōgen's full control. The whole decision chain builds on this — Jun's voice being central, the curated experience, the personal notes on shares. A co-branded partnership where the partner influences product selection would cut across most of this."

She brings it to the next morning round with the research:

**Mara**: "Okay, here's what I found. GreenLeaf could potentially 3x our subscriber base within six months. But the standard deal in this space means they'd expect input on product selection and their own branding on the experience."

**Jun**: "So they'd tell us which beans to feature?"

**Mara**: "That's what they usually ask for."

**Jun**: "Then no. That's not what we do."

**Priya**: "What if we set the terms? Our brand, our curation, they just distribute?"

**Mara**: "I think we can try. But I want to be clear about the line — Jun, what's the line for you?"

**Jun**: "I pick the beans. I write the stories. The page looks like Kōgen, not like GreenLeaf. If someone gets our coffee through them, it should feel the same as getting it from us."

> **20260511-160000-d-stg-w4p**: We will explore the GreenLeaf partnership on the condition that Kōgen brand, curation, and voice remain fully under our control. No white-labeling, no external curation influence. GreenLeaf distributes, we create. If negotiation signals indicate they won't accept these terms, we walk away. Refs: 20260405-090000-d-stg-f3a, 20260510-091000-s-stg-j8k. Confidence: medium.

Mara records this. The confidence is medium — it's an experiment at the strategic layer. The decision enables exploration without full commitment.

**Jun** adds his own guardrail:

> **20260511-161000-d-stg-k3f**: Guardrail: any partnership or distribution deal must preserve Jun's sole authority over bean selection and discovery content. This is non-negotiable and applies to all future partnership signals, not just GreenLeaf. Refs: 20260511-160000-d-stg-w4p, 20260405-090000-d-stg-f3a.

This is the first explicit guardrail in the system. It didn't come from a governance framework or a risk assessment. It came from Jun knowing what matters and recording it as a constraint that will guide future decisions — by humans and agents — without needing to re-have this conversation.

---

## What the Story Shows

**How contracts emerged:**
- Nobody assigned roles. Through working, patterns formed: Jun decides experience and curation, Mara decides business and positioning, Priya decides technical approach.
- Questions found the right person — the system routed based on domain, not hierarchy.

**How signals flowed:**
- Everyone captured signals in their own work — Jun from customer conversations, Priya from code and analytics, Mara from business research and customer calls
- Agents contributed signals too (analytics engagement data, share flow gap review, GreenLeaf coherence check) — treated the same as human observations
- Morning coffee conversations brought signals together across domains
- Conflicting signals (Mara vs. Jun on the layout) were surfaced explicitly and resolved through a branch experiment, not debate

**How decisions found the right level:**
- Priya decided on magic link auth (tactical, her domain) without consulting anyone — then reality challenged it when Thomas got locked out
- Jun decided on the share experience (conceptual, his domain) when the system routed the question to him
- Mara decided on shipping pricing (conceptual, her domain) when she encountered it in her own work
- The GreenLeaf response required all three at the strategic level
- Some things were deliberately left as open signals rather than forced into premature decisions (the reorder question)

**How the loop closed:**
- Actions recorded what was actually done, closing the gap between decisions and reality
- Evaluations were explicit — a decision to evaluate, an action recording who reviewed, signals capturing what they found
- Previous decisions got revisited when new signals contradicted them (magic links → email update flow)
- The prototype's "failure" — feeling like a generic webshop — was the most valuable signal in the whole project
- Guardrails grew from experience — Jun's curation guardrail now guides future decisions without re-debating

**What the system did:**
- Caught up each person with context relevant to their domain
- Surfaced tensions between new signals and existing decisions
- Suggested next actions — build, evaluate, branch, bring the team together
- Routed questions to the person with domain knowledge
- Stopped implementation when a decision couldn't be fulfilled as specified

**What was absent, and not missed:**
- No spec was written
- No tickets were created or groomed
- No sprint was planned
- No standup happened
- No status report was filed
- No role was formally assigned
