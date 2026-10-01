package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-190: no path leaves a workspace without an owner.
func TestLastOwnerGuard(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	sole := newUser(t, s)
	if err := s.AddMember(ctx, ws, sole, models.RoleOwner); err != nil {
		t.Fatal(err)
	}
	// Demoting the only owner is refused and the role is unchanged.
	if err := s.AddMember(ctx, ws, sole, models.RoleAdmin); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("demote sole owner: err = %v, want ErrLastOwner", err)
	}
	if role, _ := s.RoleIn(ctx, ws, sole); role != models.RoleOwner {
		t.Fatalf("role after refused demotion = %q, want owner", role)
	}
	if err := s.RemoveMember(ctx, ws, sole); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("remove sole owner: err = %v, want ErrLastOwner", err)
	}
	// Re-adding as owner is a no-op, not a demotion.
	if err := s.AddMember(ctx, ws, sole, models.RoleOwner); err != nil {
		t.Fatalf("re-add owner: %v", err)
	}
	// With a second owner, demoting one is fine.
	second := newUser(t, s)
	if err := s.AddMember(ctx, ws, second, models.RoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(ctx, ws, second, models.RoleMember); err != nil {
		t.Fatalf("demote with two owners: %v", err)
	}
}

// Two owners removing each other at the same moment: exactly one succeeds.
func TestLastOwnerGuardConcurrent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for round := 0; round < 10; round++ {
		ws := newWorkspace(t, s)
		a, b := newUser(t, s), newUser(t, s)
		for _, u := range []string{a, b} {
			if err := s.AddMember(ctx, ws, u, models.RoleOwner); err != nil {
				t.Fatal(err)
			}
		}
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, u := range []string{a, b} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = s.RemoveMember(ctx, ws, u)
			}()
		}
		wg.Wait()
		ok, last := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrLastOwner):
				last++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if ok != 1 || last != 1 {
			t.Fatalf("round %d: %d succeeded, %d last_owner (want 1 and 1)", round, ok, last)
		}
		members, err := s.ListMembers(ctx, ws)
		if err != nil || len(members) != 1 {
			t.Fatalf("round %d: members = %v / %v, want exactly one owner left", round, members, err)
		}
	}
}
