package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
	"gopkg.in/yaml.v3"
)

const Version = 1
const MaxBytes = 2 << 20

type Config struct {
	Version                int          `yaml:"config_version" json:"config_version"`
	HTTP                   HTTP         `yaml:"http" json:"http"`
	Binding                Binding      `yaml:"binding" json:"binding"`
	Connections            []Connection `yaml:"connections" json:"connections"`
	Nodes                  []Node       `yaml:"nodes" json:"nodes"`
	Routes                 []Route      `yaml:"routes" json:"routes"`
	Commands               []Command    `yaml:"commands" json:"commands"`
	InventoryRetentionDays int          `yaml:"inventory_retention_days" json:"inventory_retention_days"`
}

type HTTP struct {
	Listen              string   `yaml:"listen" json:"listen"`
	AdminToken          string   `yaml:"admin_token" json:"admin_token"`
	AllowedHosts        []string `yaml:"allowed_hosts" json:"allowed_hosts"`
	TLSCert             string   `yaml:"tls_cert,omitempty" json:"tls_cert,omitempty"`
	TLSKey              string   `yaml:"tls_key,omitempty" json:"tls_key,omitempty"`
	AllowInsecureRemote bool     `yaml:"allow_insecure_remote" json:"allow_insecure_remote"`
}

type Binding struct {
	Enabled               bool `yaml:"enabled" json:"enabled"`
	CodeTTLSeconds        int  `yaml:"code_ttl_seconds" json:"code_ttl_seconds"`
	MaxAttempts           int  `yaml:"max_attempts" json:"max_attempts"`
	AttemptWindowSeconds  int  `yaml:"attempt_window_seconds" json:"attempt_window_seconds"`
	MaxPlayersPerIdentity int  `yaml:"max_players_per_identity" json:"max_players_per_identity"`
	KickOnUnbind          bool `yaml:"kick_on_unbind" json:"kick_on_unbind"`
}

type Connection struct {
	ID                  string `yaml:"id" json:"id"`
	Type                string `yaml:"type" json:"type"`
	Enabled             bool   `yaml:"enabled" json:"enabled"`
	Endpoint            string `yaml:"endpoint" json:"endpoint"`
	Token               string `yaml:"token" json:"token"`
	AccountID           string `yaml:"account_id" json:"account_id"`
	AllowInsecureRemote bool   `yaml:"allow_insecure_remote" json:"allow_insecure_remote"`
}

type Node struct {
	ID               string   `yaml:"id" json:"id"`
	NetworkID        string   `yaml:"network_id" json:"network_id"`
	ServerID         string   `yaml:"server_id" json:"server_id"`
	Token            string   `yaml:"token" json:"token"`
	IdentityMode     string   `yaml:"identity_mode" json:"identity_mode"`
	SessionAuthority bool     `yaml:"session_authority" json:"session_authority"`
	LoginGate        bool     `yaml:"login_gate" json:"login_gate"`
	Actions          []string `yaml:"actions" json:"actions"`
}

type Route struct {
	ID            string               `yaml:"id" json:"id"`
	ConnectionID  string               `yaml:"connection_id" json:"connection_id"`
	Conversation  adapter.Conversation `yaml:"conversation" json:"conversation"`
	NodeIDs       []string             `yaml:"node_ids" json:"node_ids"`
	ToGame        bool                 `yaml:"to_game" json:"to_game"`
	ToBot         bool                 `yaml:"to_bot" json:"to_bot"`
	Commands      bool                 `yaml:"commands" json:"commands"`
	Notifications bool                 `yaml:"notifications" json:"notifications"`
	UnbindOnLeave bool                 `yaml:"unbind_on_leave" json:"unbind_on_leave"`
	BotPrefix     string               `yaml:"bot_prefix" json:"bot_prefix"`
	GamePrefix    string               `yaml:"game_prefix" json:"game_prefix"`
}

type Command struct {
	Name            string   `yaml:"name" json:"name"`
	NodeID          string   `yaml:"node_id" json:"node_id"`
	Template        string   `yaml:"template" json:"template"`
	Arguments       []string `yaml:"arguments" json:"arguments"`
	Roles           []string `yaml:"roles" json:"roles"`
	RequireBinding  bool     `yaml:"require_binding" json:"require_binding"`
	CooldownSeconds int      `yaml:"cooldown_seconds" json:"cooldown_seconds"`
}

func Defaults() Config {
	return Config{
		Version: Version,
		HTTP:    HTTP{Listen: "127.0.0.1:17880", AdminToken: "file:admin.token"},
		Binding: Binding{Enabled: true, CodeTTLSeconds: 300, MaxAttempts: 5,
			AttemptWindowSeconds: 600, MaxPlayersPerIdentity: 3, KickOnUnbind: true},
		InventoryRetentionDays: 30,
		Connections:            []Connection{}, Nodes: []Node{}, Routes: []Route{}, Commands: []Command{},
	}
}

func Decode(data []byte) (Config, error) {
	c := Defaults()
	if len(data) == 0 || len(data) > MaxBytes {
		return c, errors.New("configuration must be 1 byte to 2 MiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, errors.New("configuration must contain exactly one YAML document")
	}
	return c, c.Validate()
}

var localID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
var placeholder = regexp.MustCompile(`\{([^{}]+)\}`)

func validRef(ref string) bool {
	if strings.HasPrefix(ref, "env:") {
		return localID.MatchString(strings.TrimPrefix(ref, "env:"))
	}
	if strings.HasPrefix(ref, "file:") {
		n := strings.TrimPrefix(ref, "file:")
		return localID.MatchString(n) && n != "." && n != ".."
	}
	return false
}

func loopback(host string) bool { ip := net.ParseIP(host); return ip != nil && ip.IsLoopback() }

func (c Config) Validate() error {
	if c.Version != Version {
		return fmt.Errorf("unsupported config_version %d", c.Version)
	}
	host, port, err := net.SplitHostPort(c.HTTP.Listen)
	if err != nil || port == "" {
		return errors.New("http.listen must be an IP address and port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		return errors.New("http.listen port must be between 0 and 65535")
	}
	if net.ParseIP(host) == nil {
		return errors.New("http.listen requires a literal IP address")
	}
	if !validRef(c.HTTP.AdminToken) {
		return errors.New("http.admin_token must reference env:NAME or file:NAME")
	}
	if (c.HTTP.TLSCert == "") != (c.HTTP.TLSKey == "") {
		return errors.New("TLS certificate and key must be set together")
	}
	if !loopback(host) && c.HTTP.TLSCert == "" && !c.HTTP.AllowInsecureRemote {
		return errors.New("remote listeners require TLS or explicit allow_insecure_remote")
	}
	if !loopback(host) && len(c.HTTP.AllowedHosts) == 0 {
		return errors.New("remote listeners require explicit allowed_hosts")
	}
	for _, h := range c.HTTP.AllowedHosts {
		if !adapter.ValidID(h) || strings.ContainsAny(h, "/\\@?#*") {
			return errors.New("allowed_hosts contains an invalid host")
		}
	}
	b := c.Binding
	if b.CodeTTLSeconds < 30 || b.CodeTTLSeconds > 3600 || b.AttemptWindowSeconds < 30 || b.AttemptWindowSeconds > 86400 || b.MaxAttempts < 1 || b.MaxAttempts > 100 || b.MaxPlayersPerIdentity < 1 || b.MaxPlayersPerIdentity > 100 {
		return errors.New("binding limits are out of range")
	}
	if c.InventoryRetentionDays < 1 || c.InventoryRetentionDays > 3650 {
		return errors.New("inventory retention must be between 1 and 3650 days")
	}
	if len(c.Nodes) > 256 || len(c.Connections) > 64 || len(c.Routes) > 1024 || len(c.Commands) > 256 {
		return errors.New("too many configured connections, nodes, routes or commands")
	}
	connections := map[string]Connection{}
	for _, conn := range c.Connections {
		if !localID.MatchString(conn.ID) || !localID.MatchString(conn.Type) {
			return errors.New("invalid connection ID or type")
		}
		if _, exists := connections[conn.ID]; exists {
			return fmt.Errorf("duplicate connection %s", conn.ID)
		}
		connections[conn.ID] = conn
		if !conn.Enabled {
			continue
		}
		if !validRef(conn.Token) {
			return fmt.Errorf("connection %s requires a secret reference", conn.ID)
		}
		if conn.Type == "onebot.v11" {
			u, err := url.Parse(conn.Endpoint)
			if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "ws" && u.Scheme != "wss") {
				return fmt.Errorf("invalid OneBot endpoint for %s", conn.ID)
			}
			if u.Scheme == "ws" && !loopback(u.Hostname()) && u.Hostname() != "localhost" && !conn.AllowInsecureRemote {
				return fmt.Errorf("connection %s requires wss or explicit allow_insecure_remote", conn.ID)
			}
			if !regexp.MustCompile(`^[1-9][0-9]{4,19}$`).MatchString(conn.AccountID) {
				return fmt.Errorf("connection %s requires its numeric bot account_id", conn.ID)
			}
		}
	}
	nodes := map[string]Node{}
	authorities := map[string]int{}
	networkModes := map[string]string{}
	refs := map[string]bool{c.HTTP.AdminToken: true}
	for _, n := range c.Nodes {
		if !localID.MatchString(n.ID) || !localID.MatchString(n.NetworkID) || !localID.MatchString(n.ServerID) {
			return errors.New("invalid node, network or server ID")
		}
		if _, exists := nodes[n.ID]; exists {
			return fmt.Errorf("duplicate node %s", n.ID)
		}
		if !validRef(n.Token) || refs[n.Token] {
			return errors.New("every node requires an independent secret reference")
		}
		refs[n.Token] = true
		if n.IdentityMode != "online" && n.IdentityMode != "forwarded" && n.IdentityMode != "offline" {
			return fmt.Errorf("invalid identity mode for %s", n.ID)
		}
		if mode, exists := networkModes[n.NetworkID]; exists && mode != n.IdentityMode {
			return errors.New("nodes in a network must use the same identity mode")
		}
		networkModes[n.NetworkID] = n.IdentityMode
		nodes[n.ID] = n
		if n.SessionAuthority {
			authorities[n.NetworkID]++
		}
		for _, action := range n.Actions {
			switch action {
			case "chat.send", "player.kick", "command.execute", "inventory.read", "tps.read", "permission.check", "placeholder.resolve":
			default:
				return fmt.Errorf("unknown action %s", action)
			}
		}
	}
	for network := range networkModes {
		if authorities[network] != 1 {
			return fmt.Errorf("network %s requires exactly one session_authority node", network)
		}
	}
	routeIDs := map[string]bool{}
	conversationNetworks := map[string]string{}
	commandRoutes := map[string]bool{}
	for _, r := range c.Routes {
		if !localID.MatchString(r.ID) || routeIDs[r.ID] {
			return errors.New("invalid or duplicate route ID")
		}
		routeIDs[r.ID] = true
		conn, exists := connections[r.ConnectionID]
		if !exists {
			return fmt.Errorf("route %s references unknown connection", r.ID)
		}
		if err := r.Conversation.Validate(); err != nil {
			return err
		}
		if conn.Type == "onebot.v11" && (r.Conversation.Platform != "qq.numeric" || r.Conversation.Scope != "global" || (r.Conversation.Kind != "group" && r.Conversation.Kind != "private")) {
			return fmt.Errorf("route %s has an unsupported OneBot conversation", r.ID)
		}
		if len(r.NodeIDs) == 0 {
			return fmt.Errorf("route %s needs at least one node", r.ID)
		}
		seenNodes := map[string]bool{}
		network := ""
		for _, id := range r.NodeIDs {
			n, exists := nodes[id]
			if !exists || seenNodes[id] {
				return fmt.Errorf("route %s references unknown or duplicate node", r.ID)
			}
			seenNodes[id] = true
			if network != "" && network != n.NetworkID {
				return errors.New("a route cannot span identity networks")
			}
			network = n.NetworkID
		}
		conversationKey := r.ConnectionID + "\x1f" + r.Conversation.Key()
		if previous, exists := conversationNetworks[conversationKey]; exists && previous != network {
			return errors.New("one bot conversation cannot map to multiple identity networks")
		}
		conversationNetworks[conversationKey] = network
		if r.Commands && commandRoutes[conversationKey] {
			return errors.New("one conversation must have a single command route")
		}
		commandRoutes[conversationKey] = commandRoutes[conversationKey] || r.Commands
		if len(r.BotPrefix) > 256 || len(r.GamePrefix) > 256 {
			return errors.New("route prefix is too long")
		}
	}
	commandIDs := map[string]bool{}
	for _, command := range c.Commands {
		if !localID.MatchString(command.Name) || commandIDs[command.Name] {
			return errors.New("invalid or duplicate command name")
		}
		commandIDs[command.Name] = true
		switch command.Name {
		case "bind", "unbind", "bindings", "online", "help":
			return errors.New("command name is reserved")
		}
		n, exists := nodes[command.NodeID]
		if !exists {
			return errors.New("command references unknown node")
		}
		allowed := false
		for _, a := range n.Actions {
			if a == "command.execute" {
				allowed = true
			}
		}
		if !allowed {
			return errors.New("command node must allow command.execute")
		}
		if strings.TrimSpace(command.Template) == "" || len(command.Template) > 2048 || strings.ContainsAny(command.Template, "\r\n\x00") || command.CooldownSeconds < 1 || command.CooldownSeconds > 86400 || len(command.Arguments) > 16 {
			return errors.New("invalid command template, arguments or cooldown")
		}
		for _, pattern := range command.Arguments {
			if len(pattern) > 512 {
				return errors.New("command argument pattern too long")
			}
			if _, err := regexp.Compile("^(?:" + pattern + ")$"); err != nil {
				return err
			}
		}
		for _, match := range placeholder.FindAllStringSubmatch(command.Template, -1) {
			if match[1] == "player" && command.RequireBinding {
				continue
			}
			valid := false
			for i := range command.Arguments {
				if match[1] == fmt.Sprint(i) {
					valid = true
				}
			}
			if !valid {
				return fmt.Errorf("unknown or unsafe command placeholder %s", match[0])
			}
		}
	}
	return nil
}

func (c Config) Node(id string) (Node, bool) {
	for _, n := range c.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

func ResolvePath(dir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}
