package dbs

import (
	"fmt"
	"net/rpc"
	"sync"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/vault/helper/pluginutil"
)

// handshakeConfigs are used to just do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad plugins or executing a plugin
// directory. It is a UX feature, not a security feature.
var handshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "VAULT_DATABASE_PLUGIN",
	MagicCookieValue: "926a0820-aea2-be28-51d6-83cdf00e8edb",
}

type DatabasePlugin struct {
	impl DatabaseType
}

func (d DatabasePlugin) Server(*plugin.MuxBroker) (interface{}, error) {
	return &databasePluginRPCServer{impl: d.impl}, nil
}

func (DatabasePlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &databasePluginRPCClient{client: c}, nil
}

// DatabasePluginClient embeds a databasePluginRPCClient and wraps it's close
// method to also call Close() on the plugin.Client.
type DatabasePluginClient struct {
	client *plugin.Client
	sync.Mutex

	*databasePluginRPCClient
}

func (dc *DatabasePluginClient) Close() error {
	err := dc.databasePluginRPCClient.Close()
	dc.client.Kill()

	return err
}

// newPluginClient returns a databaseRPCClient with a connection to a running
// plugin. The client is wrapped in a DatabasePluginClient object to ensure the
// plugin is killed on call of Close().
func newPluginClient(sys pluginutil.Wrapper, pluginRunner *pluginutil.PluginRunner) (DatabaseType, error) {
	// pluginMap is the map of plugins we can dispense.
	var pluginMap = map[string]plugin.Plugin{
		"database": new(DatabasePlugin),
	}

	client, err := pluginRunner.Run(sys, pluginMap, handshakeConfig, []string{})
	if err != nil {
		return nil, err
	}

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		return nil, err
	}

	// Request the plugin
	raw, err := rpcClient.Dispense("database")
	if err != nil {
		return nil, err
	}

	// We should have a Greeter now! This feels like a normal interface
	// implementation but is in fact over an RPC connection.
	databaseRPC := raw.(*databasePluginRPCClient)

	return &DatabasePluginClient{
		client:                  client,
		databasePluginRPCClient: databaseRPC,
	}, nil
}

// NewPluginServer is called from within a plugin and wraps the provided
// DatabaseType implimentation in a databasePluginRPCServer object and starts a
// RPC server.
func NewPluginServer(db DatabaseType) {
	dbPlugin := &DatabasePlugin{
		impl: db,
	}

	// pluginMap is the map of plugins we can dispense.
	var pluginMap = map[string]plugin.Plugin{
		"database": dbPlugin,
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
		TLSProvider:     pluginutil.VaultPluginTLSProvider,
	})
}

// ---- RPC client domain ----

// databasePluginRPCClient impliments DatabaseType and is used on the client to
// make RPC calls to a plugin.
type databasePluginRPCClient struct {
	client *rpc.Client
}

func (dr *databasePluginRPCClient) Type() string {
	var dbType string
	//TODO: catch error
	dr.client.Call("Plugin.Type", struct{}{}, &dbType)

	return fmt.Sprintf("plugin-%s", dbType)
}

func (dr *databasePluginRPCClient) CreateUser(statements Statements, username, password, expiration string) error {
	req := CreateUserRequest{
		Statements: statements,
		Username:   username,
		Password:   password,
		Expiration: expiration,
	}

	err := dr.client.Call("Plugin.CreateUser", req, &struct{}{})

	return err
}

func (dr *databasePluginRPCClient) RenewUser(statements Statements, username, expiration string) error {
	req := RenewUserRequest{
		Statements: statements,
		Username:   username,
		Expiration: expiration,
	}

	err := dr.client.Call("Plugin.RenewUser", req, &struct{}{})

	return err
}

func (dr *databasePluginRPCClient) RevokeUser(statements Statements, username string) error {
	req := RevokeUserRequest{
		Statements: statements,
		Username:   username,
	}

	err := dr.client.Call("Plugin.RevokeUser", req, &struct{}{})

	return err
}

func (dr *databasePluginRPCClient) Initialize(conf map[string]interface{}) error {
	err := dr.client.Call("Plugin.Initialize", conf, &struct{}{})

	return err
}

func (dr *databasePluginRPCClient) Close() error {
	err := dr.client.Call("Plugin.Close", struct{}{}, &struct{}{})

	return err
}

func (dr *databasePluginRPCClient) GenerateUsername(displayName string) (string, error) {
	resp := &GenerateUsernameResponse{}
	err := dr.client.Call("Plugin.GenerateUsername", displayName, resp)

	return resp.Username, err
}

func (dr *databasePluginRPCClient) GeneratePassword() (string, error) {
	resp := &GeneratePasswordResponse{}
	err := dr.client.Call("Plugin.GeneratePassword", struct{}{}, resp)

	return resp.Password, err
}

func (dr *databasePluginRPCClient) GenerateExpiration(duration time.Duration) (string, error) {
	resp := &GenerateExpirationResponse{}
	err := dr.client.Call("Plugin.GenerateExpiration", duration, resp)

	return resp.Expiration, err
}

// ---- RPC server domain ----

// databasePluginRPCServer impliments DatabaseType and is run inside a plugin
type databasePluginRPCServer struct {
	impl DatabaseType
}

func (ds *databasePluginRPCServer) Type(_ struct{}, resp *string) error {
	*resp = ds.impl.Type()
	return nil
}

func (ds *databasePluginRPCServer) CreateUser(args *CreateUserRequest, _ *struct{}) error {
	err := ds.impl.CreateUser(args.Statements, args.Username, args.Password, args.Expiration)

	return err
}

func (ds *databasePluginRPCServer) RenewUser(args *RenewUserRequest, _ *struct{}) error {
	err := ds.impl.RenewUser(args.Statements, args.Username, args.Expiration)

	return err
}

func (ds *databasePluginRPCServer) RevokeUser(args *RevokeUserRequest, _ *struct{}) error {
	err := ds.impl.RevokeUser(args.Statements, args.Username)

	return err
}

func (ds *databasePluginRPCServer) Initialize(args map[string]interface{}, _ *struct{}) error {
	err := ds.impl.Initialize(args)

	return err
}

func (ds *databasePluginRPCServer) Close(_ struct{}, _ *struct{}) error {
	ds.impl.Close()
	return nil
}

func (ds *databasePluginRPCServer) GenerateUsername(args string, resp *GenerateUsernameResponse) error {
	var err error
	resp.Username, err = ds.impl.GenerateUsername(args)

	return err
}

func (ds *databasePluginRPCServer) GeneratePassword(_ struct{}, resp *GeneratePasswordResponse) error {
	var err error
	resp.Password, err = ds.impl.GeneratePassword()

	return err
}

func (ds *databasePluginRPCServer) GenerateExpiration(args time.Duration, resp *GenerateExpirationResponse) error {
	var err error
	resp.Expiration, err = ds.impl.GenerateExpiration(args)

	return err
}

// ---- Request Args Domain ----

type CreateUserRequest struct {
	Statements Statements
	Username   string
	Password   string
	Expiration string
}

type RenewUserRequest struct {
	Statements Statements
	Username   string
	Expiration string
}

type RevokeUserRequest struct {
	Statements Statements
	Username   string
}

// ---- Response Args Domain ----

type GenerateUsernameResponse struct {
	Username string
}
type GenerateExpirationResponse struct {
	Expiration string
}
type GeneratePasswordResponse struct {
	Password string
}
