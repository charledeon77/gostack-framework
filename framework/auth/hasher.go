/*
Purpose:
This file implements standard, secure password hashing for the GoStack framework using Bcrypt.

Philosophy:
Password security is non-negotiable. We favor the standard, battle-tested bcrypt algorithm
with configurable hashing cost (work factor). We abstract this behind the contract.Hasher
interface, permitting mock hashing under tests and supporting future algorithm upgrades
without modifying controller credentials logic.

Architecture:
Part of the auth package. Implements contract.Hasher.

Choice:
We chose bcrypt for password hashing because of its built-in resistance to brute-force
hardware attacks via adaptive work factor scaling.
*/
package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// BcryptHasher implements contract.Hasher using the standard bcrypt package.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher constructs a new BcryptHasher with a specified cost factor.
// If cost <= 0, it defaults to bcrypt.DefaultCost.
func NewBcryptHasher(cost int) *BcryptHasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	return &BcryptHasher{cost: cost}
}

// Hash generates a secure bcrypt hash of a plain text string.
func (h *BcryptHasher) Hash(plain string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Verify compares a plain text password against a hash in constant time.
func (h *BcryptHasher) Verify(plain, hashed string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
	return err == nil
}
