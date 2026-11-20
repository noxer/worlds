package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/save"
	"github.com/noxer/worlds/internal/world"
)

// speakWith stands the party next to the NPC called name and talks to
// it, picking the choices whose text starts with one of picks in turn
// (and the last choice offered when none does), until the dialog is over.
func (h *harness) speakWith(name string, picks ...string) {
	h.t.Helper()
	for _, n := range h.e.npcs {
		if n.Name != name {
			continue
		}
		for _, d := range world.Dirs {
			p, ok := h.w.Neighbor(n.Pos, d)
			if !ok || h.w.Solid(p) || h.w.Terrain(p) != world.Land || h.e.npcAt(p) != nil {
				continue
			}
			h.e.party.PlaceAll(p, d.Opposite())
			h.press(input.Confirm)
			for k := 0; k < 600 && h.top() != Scene(h.e); k++ {
				box, ok := h.top().(*DialogBox)
				if !ok {
					if _, shop := h.top().(*Shop); shop && len(picks) == 0 {
						return // the dialog opened a shop
					}
					h.t.Fatalf("talking to %s: top is %T", name, h.top())
				}
				if cs := box.choices(); len(cs) > 0 && box.typed() {
					box.cursor = len(cs) - 1
					if len(picks) > 0 {
						for i, c := range cs {
							if strings.HasPrefix(c.Text, picks[0]) {
								box.cursor, picks = i, picks[1:]
								break
							}
						}
					}
				}
				h.press(input.Confirm)
			}
			if h.top() != Scene(h.e) {
				h.t.Fatalf("the dialog with %s never ended", name)
			}
			if len(picks) > 0 {
				h.t.Fatalf("talking to %s: never offered %q", name, picks[0])
			}
			return
		}
		h.t.Fatalf("nowhere to stand next to %s", name)
	}
	h.t.Fatalf("no NPC called %s", name)
}

// toastTitles returns the titles of the banners queued.
func (h *harness) toastTitles() []string {
	var out []string
	for _, t := range h.e.toasts {
		out = append(out, t.title)
	}
	return out
}

// TestMeadowQuest: the elder gives the job, the slimes are beaten, the
// elder pays; the journal follows, banners tell, a save keeps it.
func TestMeadowQuest(t *testing.T) {
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Quests) == 0 {
		t.Fatal("no quests")
	}
	h := newBattleHarness(t)
	h.e.quests = c.Quests
	h.e.combat = nil // the fight itself is tested elsewhere
	store := save.Memory{}
	h.e.saves = store
	journal := func() []menuItem {
		h.press(input.Menu)
		h.menuTo("Quests")
		its := h.top().(*Menu).items()
		h.press(input.Cancel)
		h.press(input.Cancel)
		return its
	}
	if its := journal(); len(its) != 2 || !its[0].off {
		t.Fatalf("the journal at the start: %v", its)
	}

	// The elder offers the job.
	h.speakWith("elder", "Is there work")
	if !h.e.story.Flag("slime_job") || !h.e.story.Flag("quest.meadow_slimes") {
		t.Fatalf("after asking for work: flags %v", h.e.story.Flags())
	}
	if got := h.toastTitles(); len(got) != 1 || got[0] != "New quest: Slimes in the meadow" {
		t.Fatalf("banners %v", got)
	}
	if its := journal(); its[0].text != "Slimes in the meadow" || !strings.Contains(its[0].about, "east field") {
		t.Fatalf("the journal with the job: %+v", its[0])
	}
	// Reporting before the work is done gets nothing.
	h.speakWith("elder")
	if h.e.story.Flag("slime_job_done") || h.e.story.Count("coin") != 0 {
		t.Fatal("paid before the field was clear")
	}

	// The field is cleared (as a won fight leaves it).
	for _, n := range h.enemies() {
		n.active, n.defeated = false, true
		h.e.story.SetFlag(defeatedFlag(n), true)
	}
	h.e.toasts = nil
	h.e.updateQuests()
	if got := h.toastTitles(); len(got) != 1 || got[0] != "Slimes in the meadow" || !strings.Contains(h.e.toasts[0].text, "Elder Maren") {
		t.Fatalf("banners after the fight: %v", h.e.toasts)
	}
	// Save here; then report.
	h.press(input.Menu)
	h.menuTo("Save")
	h.menuTo("1")
	h.e.toasts = nil
	h.speakWith("elder")
	if !h.e.story.Flag("quest.meadow_slimes.done") || h.e.story.Count("coin") != 10 {
		t.Fatalf("after reporting: %d coins, flags %v", h.e.story.Count("coin"), h.e.story.Flags())
	}
	if got := h.e.toasts; len(got) != 1 || got[0].title != "Quest completed: Slimes in the meadow" || got[0].text != "Received: Coin x10." {
		t.Fatalf("banners after reporting: %+v", got)
	}
	if its := journal(); its[0].text != "Slimes in the meadow (done)" {
		t.Fatalf("the journal when done: %+v", its[0])
	}
	// The elder doesn't pay twice.
	h.speakWith("elder")
	if h.e.story.Count("coin") != 10 {
		t.Fatalf("%d coins after talking again", h.e.story.Count("coin"))
	}
	// Banners pass with time.
	for k := 0; k < toastTicks+5; k++ {
		h.tick(input.Frame{})
	}
	if len(h.e.toasts) != 0 {
		t.Errorf("the banner is still up: %+v", h.e.toasts)
	}
	// The save from before reporting: the quest under way again, unpaid.
	h.press(input.Menu)
	h.menuTo("Load")
	h.menuTo("1")
	h.runUntil("loading", 200, func() bool { return h.e.fade == 0 })
	if h.e.story.Flag("quest.meadow_slimes.done") || !h.e.story.Flag("quest.meadow_slimes.clear") || h.e.story.Count("coin") != 0 {
		t.Fatalf("after loading: %d coins, flags %v", h.e.story.Count("coin"), h.e.story.Flags())
	}
}
