package gate

import (
	"errors"
	"testing"
)

type user struct {
	ID    int
	Role  string
}

type post struct {
	ID       int
	AuthorID int
}

func TestGate_AllowsAndDenies(t *testing.T) {
	g := New()

	g.Define("update-post", func(u any, target any) bool {
		usr := u.(user)
		pst := target.(post)
		return usr.Role == "admin" || usr.ID == pst.AuthorID
	})

	author := user{ID: 1, Role: "author"}
	stranger := user{ID: 2, Role: "author"}
	admin := user{ID: 99, Role: "admin"}
	p := post{ID: 10, AuthorID: 1}

	if !g.Allows("update-post", author, p) {
		t.Error("author should be allowed to update their own post")
	}

	if g.Allows("update-post", stranger, p) {
		t.Error("stranger should NOT be allowed to update post")
	}

	if !g.Allows("update-post", admin, p) {
		t.Error("admin should be allowed to update any post")
	}

	// Authorize helper
	if err := g.Authorize("update-post", stranger, p); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}

	if err := g.Authorize("update-post", author, p); err != nil {
		t.Errorf("expected nil error for authorized user, got: %v", err)
	}
}

