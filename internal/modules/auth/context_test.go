package auth

import (
	"context"
	"testing"
)

func TestWithIdentity_RoundTrip(t *testing.T) {
	id := Identity{Kind: KindDesktop, Subject: "peter", Name: "Peter", Email: "peter@x.com"}
	ctx := WithIdentity(context.Background(), id)

	got, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("IdentityFromContext should report ok=true after WithIdentity")
	}
	if got != id {
		t.Errorf("got %+v, want %+v", got, id)
	}
}

func TestIdentityFromContext_Empty(t *testing.T) {
	got, ok := IdentityFromContext(context.Background())
	if ok {
		t.Fatal("ok should be false on bare context")
	}
	if !got.IsZero() {
		t.Errorf("expected zero Identity, got %+v", got)
	}
}

func TestIdentityFromContext_NilContext(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil context should not panic, got %v", r)
		}
	}()
	got, ok := IdentityFromContext(nil)
	if ok || !got.IsZero() {
		t.Fatalf("nil context should give zero+false, got (%+v, %v)", got, ok)
	}
}

func TestWithIdentity_DoesNotLeakAcrossUnrelatedKeys(t *testing.T) {
	type otherKey struct{}
	parent := context.WithValue(context.Background(), otherKey{}, "unrelated")

	// No identity stored - even with other ctx values present, we must
	// report ok=false rather than dragging the unrelated value through.
	if _, ok := IdentityFromContext(parent); ok {
		t.Fatal("unrelated ctx values must not satisfy IdentityFromContext")
	}

	id := Identity{Kind: KindSystem, Subject: "migrator"}
	ctx := WithIdentity(parent, id)

	got, ok := IdentityFromContext(ctx)
	if !ok || got != id {
		t.Fatalf("identity lost across unrelated keys: %+v %v", got, ok)
	}
	// Unrelated value still reachable.
	if v, _ := ctx.Value(otherKey{}).(string); v != "unrelated" {
		t.Errorf("unrelated ctx value lost: %q", v)
	}
}

func TestWithIdentity_OverwriteShadowsPrior(t *testing.T) {
	first := Identity{Kind: KindDesktop, Subject: "peter"}
	second := Identity{Kind: KindSubscription, Subject: "sub-1"}
	ctx := WithIdentity(WithIdentity(context.Background(), first), second)

	got, ok := IdentityFromContext(ctx)
	if !ok || got != second {
		t.Errorf("nested overwrite failed: got %+v, want %+v", got, second)
	}
}
