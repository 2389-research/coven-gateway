// ABOUTME: Admin CLI for coven-gateway identity and binding management
// ABOUTME: Uses gRPC with JWT authentication to manage principals and bindings

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

const banner = `
  __       _     _                 _           _
 / _| ___ | | __| |       __ _  __| |_ __ ___ (_)_ __
| |_ / _ \| |/ _' |_____ / _' |/ _' | '_ ' _ \| | '_ \
|  _| (_) | | (_| |_____| (_| | (_| | | | | | | | | | |
|_|  \___/|_|\__,_|      \__,_|\__,_|_| |_| |_|_|_| |_|
`

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Get config from environment or token file
	// FOLD_GATEWAY_HOST is preferred; derives gRPC and HTTP URLs
	// Falls back to legacy FOLD_GATEWAY_GRPC for backwards compatibility
	grpcAddr := os.Getenv("FOLD_GATEWAY_GRPC")
	if grpcAddr == "" {
		if host := os.Getenv("FOLD_GATEWAY_HOST"); host != "" {
			grpcAddr = host + ":50051"
		} else {
			grpcAddr = "localhost:50051"
		}
	}
	token := getToken()

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "me":
		err = cmdMe(grpcAddr, token)
	case "bindings":
		err = cmdBindings(grpcAddr, token, args)
	case "token":
		err = cmdToken(grpcAddr, token, args)
	case "agents":
		err = cmdAgents(grpcAddr, token, args)
	case "status":
		err = cmdStatus(grpcAddr, token)
	case "invite":
		err = cmdInvite(args)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		color.Red("Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	cyan := color.New(color.FgCyan)
	yellow := color.New(color.FgYellow)

	cyan.Print(banner)
	fmt.Println()
	fmt.Println("Usage: coven-admin <command> [args]")
	fmt.Println()
	yellow.Println("Commands:")
	fmt.Println("  me                      Show your identity (principal + roles)")
	fmt.Println("  status                  Show gateway status and your identity")
	fmt.Println("  bindings                List all channel bindings")
	fmt.Println("  bindings list           List all channel bindings")
	fmt.Println("  bindings create         Create a new binding")
	fmt.Println("  bindings delete <id>    Delete a binding by ID")
	fmt.Println("  agents                  List all agent principals")
	fmt.Println("  agents list             List all agent principals")
	fmt.Println("  agents create           Register a new agent")
	fmt.Println("  agents delete <id>      Delete an agent by ID")
	fmt.Println("  token create            Generate a JWT token for a principal")
	fmt.Println("  invite create           Generate an admin web UI invite link")
	fmt.Println()
	yellow.Println("Environment:")
	fmt.Println("  FOLD_GATEWAY_HOST       Gateway hostname (derives gRPC :50051 and HTTPS URLs)")
	fmt.Println("  FOLD_TOKEN              JWT authentication token (required)")
	fmt.Println()
	yellow.Println("Legacy (overrides FOLD_GATEWAY_HOST if set):")
	fmt.Println("  FOLD_GATEWAY_GRPC       Gateway gRPC address (default: localhost:50051)")
	fmt.Println("  FOLD_ADMIN_URL          Gateway admin URL (default: http://localhost:8080)")
	fmt.Println()
	yellow.Println("Examples:")
	fmt.Println("  export FOLD_TOKEN=\"eyJhbG...\"")
	fmt.Println("  coven-admin me")
	fmt.Println("  coven-admin bindings")
	fmt.Println("  coven-admin agents create --name 'My Agent' --pubkey-fp <fingerprint>")
	fmt.Println("  coven-admin bindings create --frontend matrix --channel '!room:example.org' --agent <agent-id>")
	fmt.Println()
}

// createClient creates a gRPC client connection
func createClient(addr string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}
	return conn, nil
}

// authContext creates a context with the JWT token in metadata
func authContext(token string) context.Context {
	ctx := context.Background()
	if token != "" {
		md := metadata.Pairs("authorization", "Bearer "+token)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

// cmdMe shows the current user's identity
func cmdMe(addr, token string) error {
	if token == "" {
		return fmt.Errorf("FOLD_TOKEN environment variable is required")
	}

	conn, err := createClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewClientServiceClient(conn)
	ctx := authContext(token)

	resp, err := client.GetMe(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("GetMe: %w", err)
	}

	green := color.New(color.FgGreen)
	cyan := color.New(color.FgCyan)

	fmt.Println()
	cyan.Println("  Identity")
	cyan.Println("  --------")
	fmt.Printf("  Principal ID:   %s\n", resp.PrincipalId)
	fmt.Printf("  Type:           %s\n", resp.PrincipalType)
	fmt.Printf("  Display Name:   %s\n", resp.DisplayName)
	fmt.Printf("  Status:         %s\n", resp.Status)

	if len(resp.Roles) > 0 {
		green.Printf("  Roles:          %s\n", strings.Join(resp.Roles, ", "))
	} else {
		fmt.Printf("  Roles:          (none)\n")
	}

	if resp.MemberId != nil {
		fmt.Printf("  Member ID:      %s\n", *resp.MemberId)
	}
	fmt.Println()

	return nil
}

// cmdStatus shows gateway status and identity
func cmdStatus(addr, token string) error {
	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	cyan.Print(banner)
	fmt.Println()

	// Try to connect
	conn, err := createClient(addr)
	if err != nil {
		yellow.Printf("  Gateway:  ")
		color.Red("UNREACHABLE (%v)\n", err)
		return nil
	}
	defer conn.Close()

	green.Printf("  Gateway:  ")
	fmt.Printf("connected to %s\n", addr)

	// If we have a token, show identity
	if token != "" {
		client := pb.NewClientServiceClient(conn)
		ctx := authContext(token)

		resp, err := client.GetMe(ctx, &emptypb.Empty{})
		if err != nil {
			yellow.Printf("  Identity: ")
			color.Red("auth failed (%v)\n", err)
		} else {
			green.Printf("  Identity: ")
			fmt.Printf("%s (%s)\n", resp.DisplayName, resp.PrincipalType)
			green.Printf("  Roles:    ")
			if len(resp.Roles) > 0 {
				fmt.Printf("%s\n", strings.Join(resp.Roles, ", "))
			} else {
				fmt.Println("(none)")
			}
		}
	} else {
		yellow.Printf("  Identity: ")
		fmt.Println("(no token - set FOLD_TOKEN)")
	}

	fmt.Println()
	return nil
}

// cmdBindings handles bindings subcommands
func cmdBindings(addr, token string, args []string) error {
	if token == "" {
		return fmt.Errorf("FOLD_TOKEN environment variable is required")
	}

	// Default to list
	subcmd := "list"
	if len(args) > 0 {
		subcmd = args[0]
		args = args[1:]
	}

	switch subcmd {
	case "list", "ls":
		return cmdBindingsList(addr, token)
	case "create", "add":
		return cmdBindingsCreate(addr, token, args)
	case "delete", "rm", "remove":
		return cmdBindingsDelete(addr, token, args)
	default:
		return fmt.Errorf("unknown bindings subcommand: %s (use list, create, delete)", subcmd)
	}
}

// cmdBindingsList lists all bindings
func cmdBindingsList(addr, token string) error {
	conn, err := createClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewAdminServiceClient(conn)
	ctx := authContext(token)

	resp, err := client.ListBindings(ctx, &pb.ListBindingsRequest{})
	if err != nil {
		return fmt.Errorf("ListBindings: %w", err)
	}

	cyan := color.New(color.FgCyan)
	fmt.Println()
	cyan.Println("  Channel Bindings")
	cyan.Println("  ----------------")

	if len(resp.Bindings) == 0 {
		fmt.Println("  (no bindings)")
		fmt.Println()
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  ID\tFRONTEND\tCHANNEL\tAGENT\tCREATED")
	fmt.Fprintln(w, "  --\t--------\t-------\t-----\t-------")

	for _, b := range resp.Bindings {
		id := truncate(b.Id, 12)
		channel := truncate(b.ChannelId, 24)
		agent := truncate(b.AgentId, 20)
		created := b.CreatedAt
		if t, err := time.Parse(time.RFC3339, b.CreatedAt); err == nil {
			created = t.Format("Jan 02 15:04")
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", id, b.Frontend, channel, agent, created)
	}
	w.Flush()
	fmt.Println()

	return nil
}

// cmdBindingsCreate creates a new binding
func cmdBindingsCreate(addr, token string, args []string) error {
	// Parse args
	var frontend, channelID, agentID string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--frontend", "-f":
			if i+1 < len(args) {
				frontend = args[i+1]
				i++
			}
		case "--channel", "-c":
			if i+1 < len(args) {
				channelID = args[i+1]
				i++
			}
		case "--agent", "-a":
			if i+1 < len(args) {
				agentID = args[i+1]
				i++
			}
		}
	}

	if frontend == "" || channelID == "" || agentID == "" {
		return fmt.Errorf("usage: bindings create --frontend <name> --channel <id> --agent <id>")
	}

	conn, err := createClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewAdminServiceClient(conn)
	ctx := authContext(token)

	resp, err := client.CreateBinding(ctx, &pb.CreateBindingRequest{
		Frontend:  frontend,
		ChannelId: channelID,
		AgentId:   agentID,
	})
	if err != nil {
		return fmt.Errorf("CreateBinding: %w", err)
	}

	green := color.New(color.FgGreen)
	green.Printf("✓ Created binding: %s\n", resp.Id)
	fmt.Printf("  Frontend:  %s\n", resp.Frontend)
	fmt.Printf("  Channel:   %s\n", resp.ChannelId)
	fmt.Printf("  Agent:     %s\n", resp.AgentId)

	return nil
}

// cmdBindingsDelete deletes a binding
func cmdBindingsDelete(addr, token string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: bindings delete <binding-id>")
	}

	bindingID := args[0]

	conn, err := createClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewAdminServiceClient(conn)
	ctx := authContext(token)

	_, err = client.DeleteBinding(ctx, &pb.DeleteBindingRequest{
		Id: bindingID,
	})
	if err != nil {
		return fmt.Errorf("DeleteBinding: %w", err)
	}

	green := color.New(color.FgGreen)
	green.Printf("✓ Deleted binding: %s\n", bindingID)

	return nil
}

// cmdToken handles token subcommands
func cmdToken(addr, token string, args []string) error {
	if token == "" {
		return fmt.Errorf("FOLD_TOKEN environment variable is required")
	}

	// Default to showing usage
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
		args = args[1:]
	}

	switch subcmd {
	case "create":
		return cmdTokenCreate(addr, token, args)
	default:
		return fmt.Errorf("usage: token create --principal <id> [--ttl <duration>]")
	}
}

// cmdTokenCreate creates a new JWT token
func cmdTokenCreate(addr, token string, args []string) error {
	// Parse args
	var principalID string
	var ttlDays int64 = 30 // default 30 days

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--principal", "-p":
			if i+1 < len(args) {
				principalID = args[i+1]
				i++
			}
		case "--ttl", "-t":
			if i+1 < len(args) {
				// Parse as days
				days, err := parseIntArg(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid ttl: %w", err)
				}
				ttlDays = days
				i++
			}
		}
	}

	if principalID == "" {
		return fmt.Errorf("usage: token create --principal <id> [--ttl <days>]")
	}

	conn, err := createClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewAdminServiceClient(conn)
	ctx := authContext(token)

	// Convert days to seconds
	ttlSeconds := ttlDays * 24 * 60 * 60

	resp, err := client.CreateToken(ctx, &pb.CreateTokenRequest{
		PrincipalId: principalID,
		TtlSeconds:  ttlSeconds,
	})
	if err != nil {
		return fmt.Errorf("CreateToken: %w", err)
	}

	green := color.New(color.FgGreen)
	cyan := color.New(color.FgCyan)

	fmt.Println()
	green.Println("  Token created successfully")
	fmt.Println()
	cyan.Println("  Principal:  " + principalID)
	cyan.Println("  Expires:    " + resp.ExpiresAt)
	fmt.Println()
	fmt.Println("  Token (keep this secret!):")
	fmt.Println()
	fmt.Println("  " + resp.Token)
	fmt.Println()

	return nil
}

// parseIntArg parses a string to int64
func parseIntArg(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

// cmdAgents handles agent subcommands
func cmdAgents(addr, token string, args []string) error {
	if token == "" {
		return fmt.Errorf("FOLD_TOKEN environment variable is required")
	}

	// Default to list
	subcmd := "list"
	if len(args) > 0 {
		subcmd = args[0]
		args = args[1:]
	}

	switch subcmd {
	case "list", "ls":
		return cmdAgentsList(addr, token)
	case "create", "add":
		return cmdAgentsCreate(addr, token, args)
	case "delete", "rm", "remove":
		return cmdAgentsDelete(addr, token, args)
	default:
		return fmt.Errorf("unknown agents subcommand: %s (use list, create, delete)", subcmd)
	}
}

// cmdAgentsList lists all agent principals
func cmdAgentsList(addr, token string) error {
	conn, err := createClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewAdminServiceClient(conn)
	ctx := authContext(token)

	agentType := "agent"
	resp, err := client.ListPrincipals(ctx, &pb.ListPrincipalsRequest{
		Type: &agentType,
	})
	if err != nil {
		return fmt.Errorf("ListPrincipals: %w", err)
	}

	cyan := color.New(color.FgCyan)
	fmt.Println()
	cyan.Println("  Agent Principals")
	cyan.Println("  ----------------")

	if len(resp.Principals) == 0 {
		fmt.Println("  (no agents registered)")
		fmt.Println()
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  ID\tNAME\tSTATUS\tFINGERPRINT\tCREATED")
	fmt.Fprintln(w, "  --\t----\t------\t-----------\t-------")

	for _, p := range resp.Principals {
		id := truncate(p.Id, 20)
		name := truncate(p.DisplayName, 24)
		fp := ""
		if p.PubkeyFp != nil {
			fp = truncate(*p.PubkeyFp, 20)
		}
		created := p.CreatedAt
		if t, err := time.Parse(time.RFC3339, p.CreatedAt); err == nil {
			created = t.Format("Jan 02 15:04")
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", id, name, p.Status, fp, created)
	}
	w.Flush()
	fmt.Println()

	return nil
}

// cmdAgentsCreate creates a new agent principal
func cmdAgentsCreate(addr, token string, args []string) error {
	// Parse args
	var name, pubkey, pubkeyFP string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name", "-n":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "--pubkey", "-k":
			if i+1 < len(args) {
				pubkey = args[i+1]
				i++
			}
		case "--pubkey-fp", "--fp", "-f":
			if i+1 < len(args) {
				pubkeyFP = args[i+1]
				i++
			}
		}
	}

	if name == "" {
		return fmt.Errorf("usage: agents create --name <name> [--pubkey <key> | --pubkey-fp <fingerprint>]")
	}

	if pubkey == "" && pubkeyFP == "" {
		return fmt.Errorf("agent requires either --pubkey <key> or --pubkey-fp <fingerprint>")
	}

	conn, err := createClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewAdminServiceClient(conn)
	ctx := authContext(token)

	req := &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: name,
		Roles:       []string{"member"},
	}
	if pubkey != "" {
		req.Pubkey = &pubkey
	}
	if pubkeyFP != "" {
		req.PubkeyFp = &pubkeyFP
	}

	resp, err := client.CreatePrincipal(ctx, req)
	if err != nil {
		return fmt.Errorf("CreatePrincipal: %w", err)
	}

	green := color.New(color.FgGreen)
	green.Printf("✓ Created agent: %s\n", resp.Id)
	fmt.Printf("  Name:        %s\n", resp.DisplayName)
	if resp.PubkeyFp != nil {
		fmt.Printf("  Fingerprint: %s\n", *resp.PubkeyFp)
	}
	fmt.Printf("  Status:      %s\n", resp.Status)

	return nil
}

// cmdAgentsDelete deletes an agent principal
func cmdAgentsDelete(addr, token string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: agents delete <agent-id>")
	}

	agentID := args[0]

	conn, err := createClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewAdminServiceClient(conn)
	ctx := authContext(token)

	_, err = client.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{
		Id: agentID,
	})
	if err != nil {
		return fmt.Errorf("DeletePrincipal: %w", err)
	}

	green := color.New(color.FgGreen)
	green.Printf("✓ Deleted agent: %s\n", agentID)

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getToken returns the JWT token from FOLD_TOKEN env var or ~/.config/coven/token file
func getToken() string {
	// Check env var first
	if token := os.Getenv("FOLD_TOKEN"); token != "" {
		return token
	}

	// Try to read from token file
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(homeDir, ".config")
	}

	tokenPath := filepath.Join(configDir, "fold", "token")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// cmdInvite handles admin invite subcommands
func cmdInvite(args []string) error {
	// Default to create
	subcmd := "create"
	if len(args) > 0 {
		subcmd = args[0]
		args = args[1:]
	}

	switch subcmd {
	case "create":
		return cmdInviteCreate(args)
	default:
		return fmt.Errorf("unknown invite subcommand: %s (use create)", subcmd)
	}
}

// cmdInviteCreate creates a new admin invite link
func cmdInviteCreate(args []string) error {
	// Parse args
	var baseURL string
	var dbPath string
	var ttlHours int64 = 24 // default 24 hours

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--url", "-u":
			if i+1 < len(args) {
				baseURL = args[i+1]
				i++
			}
		case "--db", "-d":
			if i+1 < len(args) {
				dbPath = args[i+1]
				i++
			}
		case "--ttl", "-t":
			if i+1 < len(args) {
				hours, err := parseIntArg(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid ttl: %w", err)
				}
				ttlHours = hours
				i++
			}
		}
	}

	// Default database path
	// FOLD_DB_PATH env var takes precedence for Docker deployments
	if dbPath == "" {
		if envPath := os.Getenv("FOLD_DB_PATH"); envPath != "" {
			dbPath = envPath
		} else {
			configDir := os.Getenv("XDG_CONFIG_HOME")
			if configDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("could not determine config directory: %w", err)
				}
				configDir = filepath.Join(homeDir, ".config")
			}
			dbPath = filepath.Join(configDir, "fold", "gateway.db")
		}
	}

	// Default base URL
	// FOLD_GATEWAY_URL takes full URL, or FOLD_GATEWAY_HOST derives http:// URL
	// (use http:// for tailnet-only; WireGuard already encrypts)
	if baseURL == "" {
		if url := os.Getenv("FOLD_GATEWAY_URL"); url != "" {
			baseURL = url
		} else if host := os.Getenv("FOLD_GATEWAY_HOST"); host != "" {
			baseURL = "http://" + host
		} else {
			baseURL = "http://localhost:8080"
		}
	}

	// Open database
	db, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Generate token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generating token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Create invite
	invite := &store.AdminInvite{
		ID:        token,
		CreatedBy: "", // bootstrap invite, no creator
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Duration(ttlHours) * time.Hour),
	}

	if err := db.CreateAdminInvite(context.Background(), invite); err != nil {
		return fmt.Errorf("creating invite: %w", err)
	}

	inviteURL := baseURL + "/admin/invite/" + token

	green := color.New(color.FgGreen)
	cyan := color.New(color.FgCyan)
	yellow := color.New(color.FgYellow)

	fmt.Println()
	green.Println("  Admin invite created!")
	fmt.Println()
	cyan.Println("  Invite URL:")
	fmt.Println()
	fmt.Println("  " + inviteURL)
	fmt.Println()
	yellow.Printf("  Expires: %s (%d hours)\n", invite.ExpiresAt.Format(time.RFC3339), ttlHours)
	fmt.Println()

	return nil
}
