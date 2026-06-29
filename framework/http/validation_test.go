/*
Purpose:
This file contains the unit test suite for the GoStack HTTP validation rule engine.
It covers the declarative Rules() composer and all built-in Rule functions.

Philosophy:
The rule engine is the first line of defence against malformed user input. Every
built-in rule must be tested against both valid and invalid inputs — including
edge cases like empty strings, Unicode characters, and boundary lengths — so that
developers can trust the rules they compose will behave exactly as documented.

Architecture:
Each test constructs a minimal anonymous struct, invokes Rules() or the rule
function directly, and asserts the presence or absence of error entries. Tests
are entirely self-contained with no shared mutable state.

Choice:
We test Rule functions directly (not via ValidateRequest middleware) to isolate
the rule logic from the HTTP layer, keeping tests fast and free of net/http
setup overhead.

Implementation:
- TestRules_Required: asserts Required rejects blank/whitespace-only values.
- TestRules_IsEmail: asserts IsEmail accepts valid and rejects malformed addresses.
- TestRules_IsNumeric: asserts IsNumeric accepts digit strings and rejects mixed input.
- TestRules_MinLength: asserts MinLength rejects short values and accepts long enough ones.
- TestRules_MaxLength: asserts MaxLength rejects long values and accepts short enough ones.
- TestRules_Matches: asserts Matches rejects values not matching the supplied pattern.
- TestRules_FirstFailOnly: asserts only the first failing rule per field is reported.
- TestRules_UnrecognisedField: asserts an error is returned for unknown struct fields.
- TestRules_PassAll: asserts an empty error map when all rules pass.
*/
package http

import (
	"testing"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func assertError(t *testing.T, errors map[string]string, field string) {
	t.Helper()
	if _, ok := errors[field]; !ok {
		t.Errorf("expected error for field %q, got none", field)
	}
}

func assertNoError(t *testing.T, errors map[string]string, field string) {
	t.Helper()
	if msg, ok := errors[field]; ok {
		t.Errorf("unexpected error for field %q: %s", field, msg)
	}
}

// ─── Required ─────────────────────────────────────────────────────────────────

func TestRules_Required(t *testing.T) {
	type req struct{ Name string }

	cases := []struct {
		value   string
		wantErr bool
	}{
		{"", true},
		{"   ", true},
		{"Alice", false},
	}

	for _, c := range cases {
		r := req{Name: c.value}
		errors := Rules(&r, RuleSet{"Name": {Required}})
		if c.wantErr {
			assertError(t, errors, "Name")
		} else {
			assertNoError(t, errors, "Name")
		}
	}
}

// ─── IsEmail ──────────────────────────────────────────────────────────────────

func TestRules_IsEmail(t *testing.T) {
	type req struct{ Email string }

	valid := []string{"user@example.com", "user+tag@sub.domain.io"}
	invalid := []string{"notanemail", "missing@", "@nodomain", "no-at-sign"}

	for _, v := range valid {
		r := req{Email: v}
		if errs := Rules(&r, RuleSet{"Email": {IsEmail}}); len(errs) > 0 {
			t.Errorf("IsEmail(%q) unexpectedly failed: %v", v, errs)
		}
	}
	for _, v := range invalid {
		r := req{Email: v}
		if errs := Rules(&r, RuleSet{"Email": {IsEmail}}); len(errs) == 0 {
			t.Errorf("IsEmail(%q) should have failed but passed", v)
		}
	}
}

// ─── IsNumeric ────────────────────────────────────────────────────────────────

func TestRules_IsNumeric(t *testing.T) {
	type req struct{ Age string }

	valid := []string{"0", "42", "1000"}
	invalid := []string{"", "abc", "12.5", "1e3", " 5"}

	for _, v := range valid {
		r := req{Age: v}
		if errs := Rules(&r, RuleSet{"Age": {IsNumeric}}); len(errs) > 0 {
			t.Errorf("IsNumeric(%q) unexpectedly failed", v)
		}
	}
	for _, v := range invalid {
		r := req{Age: v}
		if errs := Rules(&r, RuleSet{"Age": {IsNumeric}}); len(errs) == 0 {
			t.Errorf("IsNumeric(%q) should have failed but passed", v)
		}
	}
}

// ─── MinLength ────────────────────────────────────────────────────────────────

func TestRules_MinLength(t *testing.T) {
	type req struct{ Password string }

	min8 := MinLength(8)

	r1 := req{Password: "short"}
	if errs := Rules(&r1, RuleSet{"Password": {min8}}); len(errs) == 0 {
		t.Error("MinLength(8): expected error for value 'short'")
	}

	r2 := req{Password: "longenough"}
	if errs := Rules(&r2, RuleSet{"Password": {min8}}); len(errs) > 0 {
		t.Errorf("MinLength(8): unexpected error for value 'longenough': %v", errs)
	}

	// Unicode: "héllo" is 5 runes — should fail MinLength(6).
	r3 := req{Password: "héllo"}
	if errs := Rules(&r3, RuleSet{"Password": {MinLength(6)}}); len(errs) == 0 {
		t.Error("MinLength(6): expected error for 5-rune unicode string")
	}
}

// ─── MaxLength ────────────────────────────────────────────────────────────────

func TestRules_MaxLength(t *testing.T) {
	type req struct{ Bio string }

	max10 := MaxLength(10)

	r1 := req{Bio: "this is way too long for the limit"}
	if errs := Rules(&r1, RuleSet{"Bio": {max10}}); len(errs) == 0 {
		t.Error("MaxLength(10): expected error for long value")
	}

	r2 := req{Bio: "short"}
	if errs := Rules(&r2, RuleSet{"Bio": {max10}}); len(errs) > 0 {
		t.Errorf("MaxLength(10): unexpected error for short value: %v", errs)
	}
}

// ─── Matches ──────────────────────────────────────────────────────────────────

func TestRules_Matches(t *testing.T) {
	type req struct{ Slug string }

	alphaOnly := Matches(`^[a-z\-]+$`)

	r1 := req{Slug: "hello-world"}
	if errs := Rules(&r1, RuleSet{"Slug": {alphaOnly}}); len(errs) > 0 {
		t.Errorf("Matches: unexpected error for 'hello-world': %v", errs)
	}

	r2 := req{Slug: "INVALID_123"}
	if errs := Rules(&r2, RuleSet{"Slug": {alphaOnly}}); len(errs) == 0 {
		t.Error("Matches: expected error for 'INVALID_123'")
	}
}

// ─── First-fail-only ──────────────────────────────────────────────────────────

func TestRules_FirstFailOnly(t *testing.T) {
	type req struct{ Email string }

	// Both Required and IsEmail would fail on an empty string,
	// but only Required's message should appear.
	r := req{Email: ""}
	errs := Rules(&r, RuleSet{"Email": {Required, IsEmail}})

	msg, ok := errs["Email"]
	if !ok {
		t.Fatal("expected error for Email, got none")
	}
	if msg != "Email is required" {
		t.Errorf("expected Required message, got: %q", msg)
	}
}

// ─── Unrecognised field ───────────────────────────────────────────────────────

func TestRules_UnrecognisedField(t *testing.T) {
	type req struct{ Name string }
	r := req{Name: "Alice"}
	errs := Rules(&r, RuleSet{"NonExistentField": {Required}})
	if _, ok := errs["NonExistentField"]; !ok {
		t.Error("expected error for unrecognised field")
	}
}

// ─── All pass ─────────────────────────────────────────────────────────────────

func TestRules_PassAll(t *testing.T) {
	type req struct {
		Email    string
		Password string
	}

	r := req{Email: "user@example.com", Password: "securepass"}
	errs := Rules(&r, RuleSet{
		"Email":    {Required, IsEmail},
		"Password": {Required, MinLength(8)},
	})

	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}
