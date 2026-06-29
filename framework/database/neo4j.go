/*
Nexus — GoStack Neo4j Graph Database Integration Subsystem.

Purpose:
This file implements the Nexus Neo4j initialization helper for the GoStack framework.
It establishes and verifies a Neo4j driver connection, returning the official
neo4j.DriverWithContext bound to the global gostack.Neo4j facade.

Philosophy:
Nexus surfaces the native Neo4j Go driver directly, giving developers full access to
Cypher query execution, graph traversals, transactions, and bookmark management without
any wrapper limitations. Graph databases have fundamentally different query patterns
(Cypher vs SQL) and forcing them behind a SQL-shaped interface would severely restrict
their capabilities.

Architecture:
Nexus sits in the database package and is only compiled into the binary when the
developer explicitly selects Neo4j during project scaffolding, upholding GoStack's
Modular Self-Containment principle.
*/
package database

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// InitNexus initializes and verifies a Neo4j connection using the provided URI and credentials.
// It returns the connected neo4j.DriverWithContext ready for use as the global gostack.Neo4j facade.
//
// Parameters:
//   - uri:      The Neo4j connection URI (e.g. "neo4j://localhost:7687" or "neo4j+s://..." for AuraDB).
//   - username: The Neo4j authentication username (typically "neo4j").
//   - password: The Neo4j authentication password.
//
// Returns:
//   - A live neo4j.DriverWithContext, or an error if connection or verification fails.
func InitNexus(uri, username, password string) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, fmt.Errorf("[Nexus] Failed to create Neo4j driver: %w", err)
	}

	// Verify the connection is live by sending a connectivity check.
	ctx := context.Background()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("[Nexus] Neo4j connectivity check failed: %w", err)
	}

	return driver, nil
}
