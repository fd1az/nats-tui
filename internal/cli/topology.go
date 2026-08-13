package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	app "github.com/fd1az/nats-tui/internal/app/topology"
	"github.com/fd1az/nats-tui/internal/config"
	domain "github.com/fd1az/nats-tui/internal/domain/topology"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type TopologyReader interface {
	GetTopology(context.Context, config.Settings) (domain.Snapshot, error)
}
type TopologyOperator interface {
	OperateTopology(context.Context, config.Settings, app.Command) error
}

func newTopologyCommand(values *viper.Viper, reader TopologyReader, operator TopologyOperator) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{Use: "topology", Short: "Inspect NATS server topology", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if reader == nil {
			return errors.New("topology reader is not configured")
		}
		snapshot, err := reader.GetTopology(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeTopologyJSON(cmd.OutOrStdout(), snapshot)
		}
		return writeTopologyTable(cmd.OutOrStdout(), snapshot)
	}}
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable JSON")
	command.AddCommand(newTopologyMutation(values, operator, "reload SERVER_ID", app.ActionReload))
	command.AddCommand(newTopologyMutation(values, operator, "step-down", app.ActionStepDownMeta))
	return command
}

func newTopologyMutation(values *viper.Viper, operator TopologyOperator, use string, action app.Action) *cobra.Command {
	var confirmed bool
	command := &cobra.Command{Use: use, Short: string(action), Args: func(cmd *cobra.Command, args []string) error {
		expected := 0
		if action == app.ActionReload {
			expected = 1
		}
		if len(args) != expected {
			return fmt.Errorf("expected %d arguments", expected)
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		if !confirmed {
			return errors.New("operation requires explicit confirmation with --yes")
		}
		if operator == nil {
			return errors.New("topology operator is not configured")
		}
		serverID := ""
		if len(args) > 0 {
			serverID = args[0]
		}
		return operator.OperateTopology(cmd.Context(), config.Load(values), app.Command{Action: action, ServerID: serverID})
	}}
	command.Flags().BoolVar(&confirmed, "yes", false, "confirm the operation")
	return command
}

type topologyJSON struct {
	Meta    metaJSON     `json:"meta"`
	Servers []serverJSON `json:"servers"`
}
type metaJSON struct {
	Name   string     `json:"name,omitempty"`
	Leader string     `json:"leader,omitempty"`
	Peers  []peerJSON `json:"peers,omitempty"`
}
type peerJSON struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Offline bool   `json:"offline"`
	Lag     uint64 `json:"lag"`
}
type serverJSON struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Host             string     `json:"host,omitempty"`
	Cluster          string     `json:"cluster,omitempty"`
	Domain           string     `json:"domain,omitempty"`
	Version          string     `json:"version"`
	Uptime           string     `json:"uptime"`
	Tags             []string   `json:"tags,omitempty"`
	Endpoints        []string   `json:"endpoints,omitempty"`
	Connections      int        `json:"connections"`
	Subscriptions    int        `json:"subscriptions"`
	CPU              float64    `json:"cpu"`
	Memory           uint64     `json:"memory"`
	JetStreamMemory  uint64     `json:"jetstream_memory"`
	JetStreamStorage uint64     `json:"jetstream_storage"`
	Links            []linkJSON `json:"links,omitempty"`
}
type linkJSON struct {
	Kind         string `json:"kind"`
	Remote       string `json:"remote"`
	Name         string `json:"name,omitempty"`
	Health       string `json:"health"`
	RTT          string `json:"rtt"`
	PendingBytes int64  `json:"pending_bytes"`
}

func topologyDTO(snapshot domain.Snapshot) topologyJSON {
	meta := snapshot.Meta()
	result := topologyJSON{Meta: metaJSON{Name: meta.Name, Leader: meta.Leader}}
	for _, peer := range meta.Peers {
		result.Meta.Peers = append(result.Meta.Peers, peerJSON{Name: peer.Name, Current: peer.Current, Offline: peer.Offline, Lag: peer.Lag})
	}
	for _, server := range snapshot.Servers() {
		item := serverJSON{ID: server.ID().String(), Name: server.Name(), Host: server.Host(), Cluster: server.Cluster(), Domain: server.Domain(), Version: server.Version(), Uptime: server.Uptime().String(), Tags: server.Tags(), Endpoints: server.Endpoints(), Connections: server.Connections(), Subscriptions: server.Subscriptions(), CPU: server.CPU(), Memory: nonNegativeCLIBytes(server.Memory()), JetStreamMemory: server.JetStream().Memory(), JetStreamStorage: server.JetStream().Storage()}
		for _, link := range server.Links() {
			item.Links = append(item.Links, linkJSON{Kind: string(link.Kind()), Remote: link.Remote().String(), Name: link.RemoteName(), Health: string(link.Health()), RTT: link.RTT().String(), PendingBytes: link.PendingBytes()})
		}
		result.Servers = append(result.Servers, item)
	}
	return result
}
func writeTopologyJSON(output io.Writer, snapshot domain.Snapshot) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(topologyDTO(snapshot))
}
func writeTopologyTable(output io.Writer, snapshot domain.Snapshot) error {
	meta := snapshot.Meta()
	if meta.Leader != "" {
		if _, err := fmt.Fprintf(output, "META LEADER  %s  PEERS %d\n\n", meta.Leader, len(meta.Peers)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "SERVER\tID\tCLUSTER\tVERSION\tCONNS\tSUBS\tJETSTREAM\tHEALTH"); err != nil {
		return err
	}
	for _, server := range snapshot.Servers() {
		health := "healthy"
		for _, link := range server.Links() {
			if link.Health() != domain.HealthHealthy {
				health = string(link.Health())
				break
			}
		}
		if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\n", topologyServerLabel(server), server.ID(), valueOr(server.Cluster(), "-"), server.Version(), server.Connections(), server.Subscriptions(), humanCLIBytes(server.JetStream().Memory()+server.JetStream().Storage()), health); err != nil {
			return err
		}
	}
	return nil
}

func topologyServerLabel(server domain.Server) string {
	name, id := server.Name(), server.ID().String()
	if name == id && len(name) > 12 {
		return "server-" + name[:8]
	}
	return name
}
func humanCLIBytes(value uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	amount := float64(value)
	unit := 0
	for amount >= 1024 && unit < len(units)-1 {
		amount /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", value)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", amount), "0"), ".") + " " + units[unit]
}

func nonNegativeCLIBytes(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	// #nosec G115 -- every positive int64 value is representable by uint64.
	return uint64(value)
}
