package dbs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

const (
	postgreSQLTypeName = "postgres"
	cassandraTypeName  = "cassandra"
)

var (
	ErrUnsupportedDatabaseType = errors.New("Unsupported database type")
)

func Factory(conf *DatabaseConfig) (DatabaseType, error) {
	switch conf.DatabaseType {
	case postgreSQLTypeName:
		var details *sqlConnectionDetails
		err := mapstructure.Decode(conf.ConnectionDetails, &details)
		if err != nil {
			return nil, err
		}

		connProducer := &sqlConnectionProducer{
			config:      conf,
			connDetails: details,
		}

		credsProducer := &sqlCredentialsProducer{
			displayNameLen: 23,
			usernameLen:    63,
		}

		return &PostgreSQL{
			ConnectionProducer:  connProducer,
			CredentialsProducer: credsProducer,
		}, nil

	case cassandraTypeName:
		var details *cassandraConnectionDetails
		err := mapstructure.Decode(conf.ConnectionDetails, &details)
		if err != nil {
			return nil, err
		}

		connProducer := &cassandraConnectionProducer{
			config:      conf,
			connDetails: details,
		}

		credsProducer := &cassandraCredentialsProducer{}

		return &Cassandra{
			ConnectionProducer:  connProducer,
			CredentialsProducer: credsProducer,
		}, nil
	}

	return nil, ErrUnsupportedDatabaseType
}

type DatabaseType interface {
	Type() string
	CreateUser(createStmt, rollbackStmt, username, password, expiration string) error
	RenewUser(username, expiration string) error
	RevokeUser(username, revocationStmt string) error

	ConnectionProducer
	CredentialsProducer
}

type DatabaseConfig struct {
	DatabaseType          string                 `json:"type" structs:"type" mapstructure:"type"`
	ConnectionDetails     map[string]interface{} `json:"connection_details" structs:"connection_details" mapstructure:"connection_details"`
	MaxOpenConnections    int                    `json:"max_open_connections" structs:"max_open_connections" mapstructure:"max_open_connections"`
	MaxIdleConnections    int                    `json:"max_idle_connections" structs:"max_idle_connections" mapstructure:"max_idle_connections"`
	MaxConnectionLifetime time.Duration          `json:"max_connection_lifetime" structs:"max_connection_lifetime" mapstructure:"max_connection_lifetime"`
}

// Query templates a query for us.
func queryHelper(tpl string, data map[string]string) string {
	for k, v := range data {
		tpl = strings.Replace(tpl, fmt.Sprintf("{{%s}}", k), v, -1)
	}

	return tpl
}
