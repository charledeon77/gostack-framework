package gostack

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/auth"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/charledeon77/gostack-framework/framework/database"
	"github.com/charledeon77/gostack-framework/framework/mail"
	"github.com/gocql/gocql"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.mongodb.org/mongo-driver/mongo"
)

// DB holds the initialized global state application database driver adapter connection.
// This is populated for relational SQL databases (MySQL, PostgreSQL, CockroachDB, SQLite).
var DB contract.Database

// Mongo is the global GoMon facade exposing the official MongoDB client.
// This is populated when the application is initialized with a MongoDB connection.
var Mongo *mongo.Client

// Neo4j is the global Nexus facade exposing the official Neo4j graph database driver.
// This is populated when the application is initialized with a Neo4j connection.
var Neo4j neo4j.DriverWithContext

// Cassandra is the global Aether facade exposing the official Cassandra session.
// This is populated when the application is initialized with a Cassandra connection.
var Cassandra *gocql.Session

// Auth coordinates authentication strategies globally.
var Auth *auth.AuthManager

// Mail is the global mailer facade. Initialise it with InitMail before sending.
var Mail contract.Mailer

// InitDatabase provisions and activates the target relational database driver infrastructure.
//
// Supported drivers: "mysql", "postgres", "sqlite", "sqlite3", "cockroach", "cockroachdb".
// For MongoDB, Neo4j, and Cassandra use InitMongo, InitNexus, and InitAether instead.
//
// Parameters:
//   - driver: The database identifier string (e.g. "mysql", "postgres", "sqlite").
//   - dsn: The Data Source Name string configuration mapping host, credentials, and target database.
//
// Returns:
//   - An error if the driver registration is missing or connection handshake protocols fail.
func InitDatabase(driver, dsn string) error {
	var db contract.Database
	var err error
	switch driver {
	case "postgres", "mysql", "sqlite", "sqlite3", "cockroach", "cockroachdb":
		// cockroach/cockroachdb are PostgreSQL wire-compatible — use the postgres driver under the hood.
		effectiveDriver := driver
		if driver == "cockroach" || driver == "cockroachdb" {
			effectiveDriver = "postgres"
		}
		db, err = database.NewSQLAdapter(effectiveDriver, dsn)
	default:
		return fmt.Errorf("database driver [%s] is not supported by InitDatabase. Use InitMongo, InitNexus, or InitAether for NoSQL/Graph databases", driver)
	}

	if err != nil {
		return err
	}
	if db == nil {
		return fmt.Errorf("database adapter failed critical initialization sequence")
	}

	err = db.Connect()
	if err != nil {
		return err
	}

	DB = db
	return nil
}

// InitMongo provisions and activates the GoMon MongoDB integration subsystem.
// It connects to MongoDB using the provided URI and sets the global gostack.Mongo facade.
//
// Parameters:
//   - uri: The MongoDB connection string (e.g. "mongodb://127.0.0.1:27017").
func InitMongo(uri string) error {
	client, err := database.InitGoMon(uri)
	if err != nil {
		return err
	}
	Mongo = client
	return nil
}

// InitNexus provisions and activates the Nexus Neo4j graph database integration subsystem.
// It connects using the official Neo4j driver and sets the global gostack.Neo4j facade.
//
// Parameters:
//   - uri:      The Neo4j connection URI (e.g. "neo4j://localhost:7687").
//   - username: The Neo4j username (typically "neo4j").
//   - password: The Neo4j password.
func InitNexus(uri, username, password string) error {
	driver, err := database.InitNexus(uri, username, password)
	if err != nil {
		return err
	}
	Neo4j = driver
	return nil
}

// InitAether provisions and activates the Aether Cassandra integration subsystem.
// It connects to the Cassandra cluster and sets the global gostack.Cassandra facade.
//
// Parameters:
//   - hosts:    A comma-separated string of Cassandra contact points (e.g. "127.0.0.1:9042").
//   - keyspace: The Cassandra keyspace to bind the session to.
func InitAether(hosts, keyspace string) error {
	session, err := database.InitAether(hosts, keyspace)
	if err != nil {
		return err
	}
	Cassandra = session
	return nil
}

// InitAuth provisions and initializes the authentication system,
// registering default session and token guards.
func InitAuth(defaultGuard string, userProvider contract.UserProvider) {
	Auth = auth.NewAuthManager(defaultGuard)
	Auth.Register("session", auth.NewSessionGuard("session", userProvider))
	Auth.Register("token", auth.NewTokenGuard(userProvider))
}

// InitMail provisions the global Mail facade from the given SMTP configuration.
//
// Call this once at application boot, typically after InitConfig:
//
//	gostack.InitMail(mail.Config{
//	    Host:        gostack.Config.Get("MAIL_HOST"),
//	    Port:        587,
//	    Username:    gostack.Config.Get("MAIL_USERNAME"),
//	    Password:    gostack.Config.Get("MAIL_PASSWORD"),
//	    FromAddress: gostack.Config.Get("MAIL_FROM_ADDRESS"),
//	    FromName:    gostack.Config.Get("MAIL_FROM_NAME"),
//	})
func InitMail(cfg mail.Config) {
	Mail = mail.NewMailer(cfg)
}


// Table provisions a fluent, decoupled instance of the QueryBuilder pipeline mapping a target table.
//
// Why this exists:
// It hooks the global active database connection context into the builder framework, providing 
// developers with an expressive data management interface from anywhere in the application scope.
//
// Parameters:
//   - name: The target schema table name string to initiate query expressions against.
func Table(name string) *database.QueryBuilder {
	return database.New(DB, name)
}