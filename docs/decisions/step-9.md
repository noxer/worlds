# Step 9: decisions made while working alone

Step 9 was worked on overnight without anyone to ask. These are the calls
I made, why, and what would be easy to change. Each is open for discussion.

## Working arrangement

- **Wake-up time:** "3:15 AM" was read as Europe/Berlin time, so the
  scheduled check-in fires at 02:15 UTC.
- **No pull request** is opened. Each part is committed and pushed to
  `claude/clever-bohr-ot0l3g` with its tests, as before.

## 9.0: the fight's feel (done before the night)

You found the first fight clunky: it was hard to reach the enemies, and
only one member could attack from a distance.

- **A role for each member:**
  - Ari and Dagny can *throw* (2–4 tiles, weaker than a strike);
  - Cato's spear *thrusts* 1–2 tiles;
  - Bryn stays the archer.

  This gives everyone a way to hit something that isn't adjacent. The
  alternative was a ranged attack for everyone, but that would make them
  all alike.
- **Movement:** one more step for all (Dagny, in heavy armour, stays one
  behind).
- **The cellar's back room:** it now opens through three tiles instead of
  one, so the party isn't fed through a single doorway.
- **Gathering:** when a fight starts, the party gathers round its leader
  instead of staying strung out in its marching line.
- **Result:** measured over 20 seeds with the AI playing both sides, the
  cellar fight went from about 6 rounds to 3, and the party ends it with
  31 of 52 hit points instead of 23.

## 9.1: health, the inn, camping, defeat

- **Hit points carry over** between fights, and walking doesn't heal.
  Otherwise healing items (9.2) and the inn would be pointless.
- **What is saved** is the hit points each member is *missing*, not what
  they have. A member at full health is left out of the save, and a higher
  maximum (levels in 9.4) needs no change to old saves. The save version
  stays 2: the new field is optional.
- **After a won fight,** a member who went down gets up with 1 hit point.
  I chose this over staying down until healed: there are no revive items
  yet, and a party member you can't use would only be frustrating.
- **The inn, the Sleeping Bee:**
  - It is a new building east of Brookhaven's square, a copy of the blue
    house, with a sign. Inside are the innkeeper Marta, beds and a
    counter (`tools/art/inn.py`, `assets/maps/detail/inn.tmj`).
  - A night costs **5 coins**. The two quests so far pay 10 and 20, so
    that is cheap but not free.
  - The night heals everyone fully, and the party wakes at **07:00**, in
    the room, by the beds.
  - The dialog asks for the action `rest` (`do: rest`), so any later
    innkeeper can reuse it.
- **Camping:** a new **Camp** entry in the pause menu.
  - It takes 8 hours and brings everyone up to at least half their hit
    points, never down.
  - It is refused:
    - indoors;
    - in a vehicle;
    - with an active enemy within 20 tiles;
    - when nobody is hurt.

    The menu says why.
  - It is the free but slow and partial option, against the inn's paid
    full heal.
- **Defeat:**
  - The party wakes at the inn the next morning, fully healed.
  - It loses a **quarter of its coins**, rounded down: 10 coins become 8.
  - The enemies of that fight are back, as before.
  - The inn costs nothing on that night ("someone carried you here").
  - I picked a quarter so that losing hurts without wiping out what you
    have saved up.
- **The party screen** shows each member's HP (in red when hurt).

Easy to change: the price (`innkeeper` in `assets/dialog/town.yaml`), the
morning hour, the camping hours and the share of coins lost (constants at
the top of `internal/scene/health.go`).

## 9.2: usable items

- **Items say what they do** in `assets/items.yaml` (`use: {heal, revive}`),
  so new items need no code. There are three so far:
  - **Honey cake** gives back 5 HP (the wanderer's gift).
  - **Herb tonic** gives back 12 HP.
  - **Smelling salts** revive a fallen member with 4 HP, and only in a
    fight.

  Outside a fight nobody stays down, because the fallen get up after a
  win, so there is nothing to revive there.
- **From the menu:**
  1. Choose an item on the Items page.
  2. Then choose a member, whose HP is listed.

  Using one on someone at full health does nothing and costs nothing.
- **In a fight:**
  - An **Items** entry on the action bar (shown only when there is
    something to use) swaps the abilities for the items.
  - Using one costs **1 AP** and reaches the user or a friend next to
    them.
  - I picked the reach so that a healer has to step close, which counts
    for something on the grid. The cost of 1 AP leaves room to move and
    strike in the same turn.
  - Allies that can be helped are highlighted in green, and a green "+N"
    rises over them.
- **Who uses items:** the AI doesn't use items for the enemies. Nothing
  they carry would make sense yet.
- **The action bar wraps** onto a second row on narrow screens. On a phone
  in portrait it was already too wide for Ari before items, and that is
  fixed too.
