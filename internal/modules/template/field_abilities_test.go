package template

import "testing"

// abilitySnapshot pins which types are allowed each gated ability.
// A type missing from a set must have that ability=false; a type
// present must have it=true. Editing the matrix to flip a bool that
// changes the allowed-set will fail this test until the set below is
// updated to match - that's the point. Drive-by changes are caught.

func TestAbilities_ExpressionItem_OnlyOnScalarValueTypes(t *testing.T) {
	allowed := stringSet(
		"text", "number", "range", "date",
		"boolean", "dropdown", "radio",
	)
	assertAbilityMatchesSet(t, "ExpressionItem", allowed, func(a Abilities) bool { return a.ExpressionItem })
}

func TestAbilities_Options_OnlyOnChoiceAndCollectionTypes(t *testing.T) {
	allowed := stringSet(
		"boolean", "range",
		"dropdown", "multioption", "radio",
		"list", "table",
		"file-path",
	)
	assertAbilityMatchesSet(t, "Options", allowed, func(a Abilities) bool { return a.Options })
}

func TestAbilities_Collapsible_OnlyOnListTableAndLink(t *testing.T) {
	allowed := stringSet("list", "table", "link")
	assertAbilityMatchesSet(t, "Collapsible", allowed, func(a Abilities) bool { return a.Collapsible })
}

func TestAbilities_SummaryField_OnlyOnLoopstart(t *testing.T) {
	allowed := stringSet("loopstart")
	assertAbilityMatchesSet(t, "SummaryField", allowed, func(a Abilities) bool { return a.SummaryField })
}

func TestAbilities_Format_OnlyOnTextarea(t *testing.T) {
	allowed := stringSet("textarea")
	assertAbilityMatchesSet(t, "Format", allowed, func(a Abilities) bool { return a.Format })
}

func TestAbilities_Readonly_OnlyOnTextLikeAndPathTypes(t *testing.T) {
	allowed := stringSet("text", "textarea", "file-path", "folder-path")
	assertAbilityMatchesSet(t, "Readonly", allowed, func(a Abilities) bool { return a.Readonly })
}

func TestAbilities_Label_DisabledOnGuidAndLoopstop(t *testing.T) {
	disallowed := stringSet("guid")
	assertAbilityDisabledOn(t, "Label", disallowed, func(a Abilities) bool { return a.Label })
}

func TestAbilities_Description_DisabledOnGuidAndLoopstop(t *testing.T) {
	disallowed := stringSet("guid", "loopstop")
	assertAbilityDisabledOn(t, "Description", disallowed, func(a Abilities) bool { return a.Description })
}

func TestAbilities_Default_DisabledOnGuidApiAndLoopMeta(t *testing.T) {
	disallowed := stringSet("guid", "api", "looper", "loopstart", "loopstop")
	assertAbilityDisabledOn(t, "Default", disallowed, func(a Abilities) bool { return a.Default })
}

func TestAbilities_PrimaryKey_DisabledOnGuidAndLoopMeta(t *testing.T) {
	disallowed := stringSet("guid", "looper", "loopstart", "loopstop")
	assertAbilityDisabledOn(t, "PrimaryKey", disallowed, func(a Abilities) bool { return a.PrimaryKey })
}

func TestAbilities_TwoColumn_DisabledOnGuidApiAndLoopMeta(t *testing.T) {
	disallowed := stringSet("guid", "api", "looper", "loopstart", "loopstop")
	assertAbilityDisabledOn(t, "TwoColumn", disallowed, func(a Abilities) bool { return a.TwoColumn })
}

// Every type in orderedTypes must be present in fieldDescriptors and
// vice versa - keeps the dropdown order and the matrix in sync.
func TestOrderedTypes_MatchesFieldDescriptors(t *testing.T) {
	if len(orderedTypes) != len(fieldDescriptors) {
		t.Fatalf("orderedTypes has %d entries; fieldDescriptors has %d",
			len(orderedTypes), len(fieldDescriptors))
	}
	seen := map[string]bool{}
	for _, id := range orderedTypes {
		if seen[id] {
			t.Errorf("orderedTypes lists %q twice", id)
		}
		seen[id] = true
		if _, ok := fieldDescriptors[id]; !ok {
			t.Errorf("orderedTypes lists %q but fieldDescriptors has no entry", id)
		}
	}
	for id := range fieldDescriptors {
		if !seen[id] {
			t.Errorf("fieldDescriptors has %q but orderedTypes does not list it", id)
		}
	}
}

// Every descriptor must have Key=true and Type=true - those are the
// two abilities that are structurally always enabled (modal always
// renders the Key + Field Type rows).
func TestAbilities_KeyAndTypeAlwaysEnabled(t *testing.T) {
	for id, def := range fieldDescriptors {
		if !def.Abilities.Key {
			t.Errorf("type %q has Key=false; every type must have Key=true", id)
		}
		if !def.Abilities.Type {
			t.Errorf("type %q has Type=false; every type must have Type=true", id)
		}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────

func stringSet(ids ...string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// assertAbilityMatchesSet enforces: ability=true ↔ id in allowed.
// Catches additions to the matrix that flip a forbidden type, AND
// removals that demote an allowed type, in one assertion.
func assertAbilityMatchesSet(t *testing.T, name string, allowed map[string]bool, get func(Abilities) bool) {
	t.Helper()
	for id, def := range fieldDescriptors {
		got := get(def.Abilities)
		want := allowed[id]
		if got != want {
			t.Errorf("type %q: %s = %v, want %v", id, name, got, want)
		}
	}
}

// assertAbilityDisabledOn enforces ability=false for every id in the
// disallowed set. Other types are left unchecked - used for abilities
// that are broadly enabled with a small forbidden list.
func assertAbilityDisabledOn(t *testing.T, name string, disallowed map[string]bool, get func(Abilities) bool) {
	t.Helper()
	for id := range disallowed {
		def, ok := fieldDescriptors[id]
		if !ok {
			t.Errorf("disallowed list mentions unknown type %q", id)
			continue
		}
		if get(def.Abilities) {
			t.Errorf("type %q must have %s=false", id, name)
		}
	}
}
