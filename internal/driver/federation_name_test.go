package driver_test

import (
	"errors"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func namedSpoke(project, name string) model.FederationSpoke {
	s := healthySpoke(project)
	s.SpokeName = name
	return s
}

func TestThePreviewOffersAnOptionalSpokeNameThatTheRunCarries(t *testing.T) {
	s := previewed(t, atHubProjectStep(t, enrollPane(t), rowReplica, hubBeta))
	wantShown(t, "the preview", s, "spoke name: -")

	card := step(t, s, key("shift+n"))
	if card.State.Model.TopCard() != model.CardFederationName || card.State.Model.CurrentMode() != model.ModeFederationName || len(card.Cmds) != 0 {
		t.Fatalf("N gave card %v, mode %q and %#v, want the name card and no daemon call", card.State.Model.TopCard(), card.State.Model.CurrentMode(), card.Cmds)
	}
	wantShown(t, "the name card", card.State, "Name Spoke", "name for the spoke (empty for none):")

	// esc leaves the draft unnamed.
	if back := step(t, typeText(t, card.State, "ph"), key("escape")).State; back.Model.TopCard() != model.NoCard || back.Model.Federation.Draft.SpokeName != "" {
		t.Errorf("esc left card %v and name %q", back.Model.TopCard(), back.Model.Federation.Draft.SpokeName)
	}

	named := step(t, typeText(t, step(t, s, key("shift+n")).State, "phone"), key("enter"))
	if len(named.Cmds) != 0 || named.State.Model.TopCard() != model.NoCard || named.State.Model.Federation.Draft.SpokeName != "phone" {
		t.Fatalf("Enter on the card gave card %v, name %q and %#v, want the draft named and no daemon call", named.State.Model.TopCard(), named.State.Model.Federation.Draft.SpokeName, named.Cmds)
	}
	wantShown(t, "the named preview", named.State, "spoke name: phone")

	// The card opens on the name given and an empty name takes it back.
	again := step(t, named.State, key("shift+n")).State
	if again.Model.Federation.Name.Input != "phone" {
		t.Errorf("the card reopened on %q, want the name given", again.Model.Federation.Name.Input)
	}
	cleared := step(t, step(t, again, key("ctrl+u")).State, key("enter")).State
	if cleared.Model.Federation.Draft.SpokeName != "" {
		t.Errorf("an empty name left %q", cleared.Model.Federation.Draft.SpokeName)
	}

	run, ok := asked[model.EnrollFederation](step(t, named.State, key("enter")).Cmds)
	if !ok || run.Draft.SpokeName != "phone" {
		t.Errorf("the run asked %#v, want an enrollment naming the spoke phone", run)
	}
}

func TestTheSpokeNameShowsOnTheCardAndTheDetail(t *testing.T) {
	s := federationListed(t, namedSpoke("acme", "laptop"), healthySpoke("zeta"))
	wantShown(t, "the list", s, "acme  Name laptop  Hub hub.example:7474  Actor alice", "zeta  Hub hub.example:7474  Actor alice")
	wantNotShown(t, "the unnamed spoke", s, "zeta  Name")

	detail := step(t, s, key("enter")).State
	wantShown(t, "the detail", detail, "spoke name: laptop")
	other := step(t, step(t, s, key("j")).State, key("enter")).State
	wantShown(t, "the unnamed detail", other, "spoke name: -")
}

func TestNOnASpokeRenamesItOnItsHubAndAnEmptyNameClearsIt(t *testing.T) {
	for _, screen := range []string{"list", "detail"} {
		t.Run(screen, func(t *testing.T) {
			s := federationListed(t, namedSpoke("acme", "laptop"), healthySpoke("zeta"))
			if screen == "detail" {
				s = step(t, s, key("enter")).State
			}
			card := step(t, s, key("shift+n"))
			if card.State.Model.TopCard() != model.CardFederationName || len(card.Cmds) != 0 || card.State.Model.Federation.Name.Input != "laptop" {
				t.Fatalf("N gave card %v, input %q and %#v, want the card on the current name", card.State.Model.TopCard(), card.State.Model.Federation.Name.Input, card.Cmds)
			}
			wantShown(t, "the card", card.State, "Rename Spoke", "name for this spoke on its hub (empty clears it):")

			saved := step(t, typeText(t, card.State, "2"), key("enter"))
			cmd, ok := asked[model.SetFederationSpokeName](saved.Cmds)
			if !ok || cmd.Name != "laptop2" || cmd.HubURL != "https://hub.example:7474" || cmd.InstanceUID != "INSTANCE-1" {
				t.Fatalf("Enter asked %#v, want the rename of INSTANCE-1 on its hub", saved.Cmds)
			}
			if !saved.State.Model.Federation.Name.Saving {
				t.Error("the card is not saving")
			}
			if again := step(t, saved.State, key("enter")); len(again.Cmds) != 0 {
				t.Errorf("Enter while it runs asked %#v", again.Cmds)
			}

			done := step(t, saved.State, model.FederationSpokeNamed{Attempt: cmd.Attempt, Hub: cmd.HubURL, Name: cmd.Name}).State
			if done.Model.TopCard() != model.NoCard {
				t.Error("the card stayed open after the hub took the name")
			}
			for _, spoke := range done.Model.Federation.Spokes {
				if spoke.SpokeName != "laptop2" {
					t.Errorf("spoke %s is named %q, want every spoke of that hub to show the new name", spoke.ProjectName, spoke.SpokeName)
				}
			}
		})
	}

	s := federationListed(t, namedSpoke("acme", "laptop"))
	clear := step(t, step(t, step(t, s, key("shift+n")).State, key("ctrl+u")).State, key("enter"))
	cmd, ok := asked[model.SetFederationSpokeName](clear.Cmds)
	if !ok || cmd.Name != "" {
		t.Fatalf("an empty name asked %#v, want a clear", clear.Cmds)
	}
	cleared := step(t, clear.State, model.FederationSpokeNamed{Attempt: cmd.Attempt, Hub: cmd.HubURL}).State
	if cleared.Model.SelectedSpoke().SpokeName != "" {
		t.Errorf("the spoke is still named %q", cleared.Model.SelectedSpoke().SpokeName)
	}
}

func TestTheCardRefusesABadNameItselfAndSendsNothing(t *testing.T) {
	s := federationListed(t, healthySpoke("acme"))
	saved := step(t, typeText(t, step(t, s, key("shift+n")).State, "Bad Name"), key("enter"))
	if len(saved.Cmds) != 0 || saved.State.Model.Federation.Name.Saving || saved.State.Model.TopCard() != model.CardFederationName || saved.State.Model.Federation.Name.Input != "Bad Name" {
		t.Fatalf("a bad name left card %v, saving %v, input %q and asked %#v", saved.State.Model.TopCard(), saved.State.Model.Federation.Name.Saving, saved.State.Model.Federation.Name.Input, saved.Cmds)
	}
	wantShown(t, "the card", saved.State, `invalid federation spoke name: "Bad Name" must match [a-z][a-z0-9-]{0,31}`)

	preview := previewed(t, atHubProjectStep(t, enrollPane(t), rowReplica, hubBeta))
	named := step(t, typeText(t, step(t, preview, key("shift+n")).State, "Phone"), key("enter")).State
	if named.Model.TopCard() != model.CardFederationName || named.Model.Federation.Draft.SpokeName != "" {
		t.Errorf("the preview took a bad name: card %v, name %q", named.Model.TopCard(), named.Model.Federation.Draft.SpokeName)
	}
}

func TestAHubsRefusalOfANameShowsItsReasonAndKeepsTheCard(t *testing.T) {
	s := federationListed(t, healthySpoke("acme"))
	saved := step(t, typeText(t, step(t, s, key("shift+n")).State, "taken"), key("enter"))
	cmd, ok := asked[model.SetFederationSpokeName](saved.Cmds)
	if !ok {
		t.Fatalf("Enter asked %#v", saved.Cmds)
	}
	refused := step(t, saved.State, model.FederationSpokeNamed{Attempt: cmd.Attempt, Hub: cmd.HubURL, Err: errors.New("federation spoke name already held")}).State
	if refused.Model.TopCard() != model.CardFederationName || refused.Model.Federation.Name.Saving || refused.Model.Federation.Name.Input != "taken" {
		t.Fatalf("a refusal left card %v, saving %v, input %q", refused.Model.TopCard(), refused.Model.Federation.Name.Saving, refused.Model.Federation.Name.Input)
	}
	wantShown(t, "the card", refused, "federation spoke name already held")
	if refused.Model.SelectedSpoke().SpokeName != "" {
		t.Error("a refused name was kept")
	}

	// Editing dismisses the reason, and an answer to an earlier try is dropped.
	if edited := step(t, refused, key("x")).State; edited.Model.Federation.Name.Err != nil {
		t.Error("typing kept the refusal")
	}
	retry := step(t, refused, key("enter"))
	next, _ := asked[model.SetFederationSpokeName](retry.Cmds)
	stale := step(t, retry.State, model.FederationSpokeNamed{Attempt: next.Attempt - 1, Hub: next.HubURL, Name: "old"}).State
	if !stale.Model.Federation.Name.Saving || stale.Model.SelectedSpoke().SpokeName != "" {
		t.Error("an answer to an earlier try was applied")
	}
}

func TestNOnAnEmptyListOrADetailLessScreenOpensNoNameCard(t *testing.T) {
	empty := federationListed(t)
	if out := step(t, empty, key("shift+n")); out.State.Model.TopCard() != model.NoCard || len(out.Cmds) != 0 {
		t.Errorf("N with no spoke opened card %v and asked %#v", out.State.Model.TopCard(), out.Cmds)
	}
}

func TestARefreshStartedBeforeARenameCannotRestoreTheOldName(t *testing.T) {
	s := federationListed(t, namedSpoke("acme", "laptop"))
	// A refresh goes out, then a rename starts; the refresh's status read happens before the hub has the new name.
	refresh := step(t, s, key("r"))
	old, ok := askedFederation(refresh.Cmds)
	if !ok {
		t.Fatalf("r asked %#v, want a status read", refresh.Cmds)
	}
	card := step(t, refresh.State, key("shift+n")).State
	saved := step(t, typeText(t, card, "2"), key("enter"))
	cmd, ok := asked[model.SetFederationSpokeName](saved.Cmds)
	if !ok {
		t.Fatalf("Enter asked %#v", saved.Cmds)
	}
	done := step(t, saved.State, model.FederationSpokeNamed{Attempt: cmd.Attempt, Hub: cmd.HubURL, Name: cmd.Name})
	fresh, ok := askedFederation(done.Cmds)
	if !ok || fresh.Request == old.Request {
		t.Fatalf("the rename's answer asked %#v, want a new status read", done.Cmds)
	}
	late := step(t, done.State, federationAnswer(old, namedSpoke("acme", "laptop"))).State
	if got := late.Model.SelectedSpoke().SpokeName; got != "laptop2" {
		t.Errorf("the spoke is named %q after the older read landed, want laptop2", got)
	}
	current := step(t, late, federationAnswer(fresh, namedSpoke("acme", "laptop2"))).State
	if got := current.Model.SelectedSpoke().SpokeName; got != "laptop2" {
		t.Errorf("the spoke is named %q after the new read landed, want laptop2", got)
	}
}
