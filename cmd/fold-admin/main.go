// ABOUTME: Admin CLI for fold-gateway status and management
// ABOUTME: Displays connected agents, bindings, and gateway health

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

type Binding struct {
	Frontend    string `json:"frontend"`
	ChannelID   string `json:"channel_id"`
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name"`
	AgentOnline bool   `json:"agent_online"`
	CreatedAt   string `json:"created_at"`
}

type BindingsResponse struct {
	Bindings []Binding `json:"bindings"`
}

const banner = `
  __       _     _                 _           _
 / _| ___ | | __| |       __ _  __| |_ __ ___ (_)_ __
| |_ / _ \| |/ _' |_____ / _' |/ _' | '_ ' _ \| | '_ \
|  _| (_) | | (_| |_____| (_| | (_| | | | | | | | | | |
|_|  \___/|_|\__,_|      \__,_|\__,_|_| |_| |_|_|_| |_|
`

func main() {
	gateway := flag.String("gateway", getEnv("FOLD_GATEWAY_HTTP", "http://localhost:8080"), "Gateway HTTP URL")
	watch := flag.Bool("watch", false, "Continuously watch gateway status")
	interval := flag.Duration("interval", 2*time.Second, "Watch interval (with -watch)")
	flag.Parse()

	baseURL := strings.TrimSuffix(*gateway, "/")

	if *watch {
		runWatch(baseURL, *interval)
		return
	}

	printStatus(baseURL)
}

func printStatus(baseURL string) {
	fmt.Print(banner)

	// Health check
	printHealth(baseURL)
	fmt.Println()

	// Connected agents
	printAgents(baseURL)
	fmt.Println()

	// Bindings
	printBindings(baseURL)
	fmt.Println()
}

func runWatch(baseURL string, interval time.Duration) {
	// Clear screen and hide cursor
	fmt.Print("\033[2J\033[H\033[?25l")
	defer fmt.Print("\033[?25h") // Show cursor on exit

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		// Move cursor to top
		fmt.Print("\033[H")
		printStatus(baseURL)
		fmt.Printf("  [watching every %v - press Ctrl+C to stop]\n", interval)

		<-ticker.C
	}
}

func printHealth(baseURL string) {
	fmt.Println("  Health")
	fmt.Println("  ------")

	// Basic health
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		fmt.Printf("  Gateway:  UNREACHABLE (%v)\n", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("  Gateway:  OK\n")
	} else {
		fmt.Printf("  Gateway:  ERROR (status %d)\n", resp.StatusCode)
	}

	// Ready check
	resp, err = http.Get(baseURL + "/health/ready")
	if err != nil {
		fmt.Printf("  Ready:    UNKNOWN\n")
		return
	}
	defer resp.Body.Close()

	var body [256]byte
	n, _ := resp.Body.Read(body[:])
	status := strings.TrimSpace(string(body[:n]))

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("  Ready:    %s\n", status)
	} else {
		fmt.Printf("  Ready:    NOT READY (%s)\n", status)
	}
}

func printAgents(baseURL string) {
	fmt.Println("  Connected Agents")
	fmt.Println("  ----------------")

	resp, err := http.Get(baseURL + "/api/agents")
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var agents []Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		fmt.Printf("  Error decoding response: %v\n", err)
		return
	}

	if len(agents) == 0 {
		fmt.Println("  (no agents connected)")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  ID\tNAME\tCAPABILITIES")
	fmt.Fprintln(w, "  --\t----\t------------")
	for _, a := range agents {
		caps := strings.Join(a.Capabilities, ", ")
		// Truncate long IDs for display
		id := a.ID
		if len(id) > 24 {
			id = id[:21] + "..."
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\n", id, a.Name, caps)
	}
	w.Flush()
}

func printBindings(baseURL string) {
	fmt.Println("  Channel Bindings")
	fmt.Println("  ----------------")

	resp, err := http.Get(baseURL + "/api/bindings")
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var bindingsResp BindingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&bindingsResp); err != nil {
		fmt.Printf("  Error decoding response: %v\n", err)
		return
	}

	if len(bindingsResp.Bindings) == 0 {
		fmt.Println("  (no bindings configured)")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  FRONTEND\tCHANNEL\tAGENT\tSTATUS\tCREATED")
	fmt.Fprintln(w, "  --------\t-------\t-----\t------\t-------")
	for _, b := range bindingsResp.Bindings {
		status := "offline"
		if b.AgentOnline {
			status = "online"
		}
		agentDisplay := b.AgentID
		if b.AgentName != "" {
			agentDisplay = b.AgentName
		}
		// Truncate long values
		if len(agentDisplay) > 20 {
			agentDisplay = agentDisplay[:17] + "..."
		}
		channelDisplay := b.ChannelID
		if len(channelDisplay) > 24 {
			channelDisplay = channelDisplay[:21] + "..."
		}
		// Parse and format created time
		created := b.CreatedAt
		if t, err := time.Parse(time.RFC3339, b.CreatedAt); err == nil {
			created = t.Format("Jan 02 15:04")
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", b.Frontend, channelDisplay, agentDisplay, status, created)
	}
	w.Flush()
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
