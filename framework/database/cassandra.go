/*
Aether — GoStack Cassandra Wide-Column Database Integration Subsystem.

Purpose:
This file implements the Aether Cassandra initialization helper for the GoStack framework.
It establishes a gocql cluster session and verifies connectivity, returning the active
*gocql.Session bound to the global gostack.Cassandra facade.

Philosophy:
Aether surfaces the native gocql session directly. Cassandra operates on a distributed
ring topology using the Cassandra Query Language (CQL), which maps naturally to Go's
standard library patterns but differs fundamentally from both SQL and document paradigms.
Wrapping it behind a SQL-shaped interface would prevent developers from using keyspace-aware
queries, lightweight transactions (LWT), and Cassandra-specific consistency level controls.

Architecture:
Aether sits in the database package and is only compiled into the binary when the
developer explicitly selects Cassandra during project scaffolding, upholding GoStack's
Modular Self-Containment principle.
*/
package database

import (
	"fmt"
	"strings"

	"github.com/gocql/gocql"
)

// InitAether initializes and verifies a Cassandra connection using the provided cluster hosts and keyspace.
// It returns the connected *gocql.Session ready for use as the global gostack.Cassandra facade.
//
// Parameters:
//   - hosts:    A comma-separated string of Cassandra cluster contact points (e.g. "127.0.0.1:9042").
//   - keyspace: The Cassandra keyspace to bind the session to (e.g. "myapp").
//
// Returns:
//   - A live *gocql.Session, or an error if the cluster connection or session creation fails.
func InitAether(hosts string, keyspace string) (*gocql.Session, error) {
	// Split comma-separated hosts into a slice.
	hostList := strings.Split(hosts, ",")
	for i, h := range hostList {
		hostList[i] = strings.TrimSpace(h)
	}

	cluster := gocql.NewCluster(hostList...)
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.Quorum

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("[Aether] Failed to connect to Cassandra cluster: %w", err)
	}

	return session, nil
}
