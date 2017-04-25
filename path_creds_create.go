package database

import (
	"fmt"
	"time"

	"github.com/hashicorp/vault/helper/strutil"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathCredsCreate(b *databaseBackend) *framework.Path {
	return &framework.Path{
		Pattern: "creds/" + framework.GenericNameRegex("name"),
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of the role.",
			},
		},

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ReadOperation: b.pathCredsCreateRead(),
		},

		HelpSynopsis:    pathCredsCreateReadHelpSyn,
		HelpDescription: pathCredsCreateReadHelpDesc,
	}
}

func (b *databaseBackend) pathCredsCreateRead() framework.OperationFunc {
	return func(req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
		name := data.Get("name").(string)

		// Get the role
		role, err := b.Role(req.Storage, name)
		if err != nil {
			return nil, err
		}
		if role == nil {
			return logical.ErrorResponse(fmt.Sprintf("unknown role: %s", name)), nil
		}

		dbConfig, err := b.DatabaseConfig(req.Storage, role.DBName)
		if err != nil {
			return nil, err
		}

		// If role name isn't in the database's allowed roles, send back a
		// permission denied.
		if !strutil.StrListContains(dbConfig.AllowedRoles, "*") && !strutil.StrListContains(dbConfig.AllowedRoles, name) {
			return nil, logical.ErrPermissionDenied
		}

		b.Lock()
		defer b.Unlock()

		// Get the Database object
		db, err := b.getOrCreateDBObj(req.Storage, role.DBName)
		if err != nil {
			return nil, fmt.Errorf("cound not retrieve db with name: %s, got error: %s", role.DBName, err)
		}

		expiration := time.Now().Add(role.DefaultTTL)

		// Create the user
		username, password, err := db.CreateUser(role.Statements, req.DisplayName, expiration)
		if err != nil {
			return nil, err
		}

		resp := b.Secret(SecretCredsType).Response(map[string]interface{}{
			"username": username,
			"password": password,
		}, map[string]interface{}{
			"username": username,
			"role":     name,
		})
		resp.Secret.TTL = role.DefaultTTL
		return resp, nil
	}
}

const pathCredsCreateReadHelpSyn = `
Request database credentials for a certain role.
`

const pathCredsCreateReadHelpDesc = `
This path reads database credentials for a certain role. The
database credentials will be generated on demand and will be automatically
revoked when the lease is up.
`
