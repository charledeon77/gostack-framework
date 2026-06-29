/*
Purpose:
This file defines standard, reusable sentinel error types for GoStack authentication.

Philosophy:
Authentication flows should return clear, typed, and predictable error conditions.
By utilizing typed sentinels, caller code (such as HTTP controllers or CLI commands)
can inspect failure causes precisely (e.g. invalid password vs token expiration) and
respond with appropriate HTTP status codes or message outputs.

Architecture:
Part of the auth package. These errors are returned by Hasher, Guards, and UserProvider implementations.
*/
package auth

import "errors"

var (
	// ErrInvalidCredentials is returned when a user provides an incorrect password.
	ErrInvalidCredentials = errors.New("authentication failed: invalid credentials")

	// ErrUserNotFound is returned when no user corresponds to the provided credentials.
	ErrUserNotFound = errors.New("authentication failed: user not found")

	// ErrSessionUninitialized is returned when the session store middleware was not run before an auth guard.
	ErrSessionUninitialized = errors.New("authentication failed: session not initialized")

	// ErrInvalidToken is returned when an API token is invalid or cannot be decoded.
	ErrInvalidToken = errors.New("authentication failed: invalid bearer token")

	// ErrTokenExpired is returned when a valid API token has exceeded its expiration date.
	ErrTokenExpired = errors.New("authentication failed: token expired")
)
