package natsmonitor

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type monitoringDoer struct{}

func (monitoringDoer) Do(request *http.Request) (*http.Response, error) {
	body := `{}`
	switch request.URL.Path {
	case "/varz":
		body = `{"server_id":"A","server_name":"nats-a","version":"2.14.4","host":"127.0.0.1","port":4222,"connections":3,"subscriptions":7,"cluster":{"name":"C1"},"jetstream":{"config":{"domain":"edge"}}}`
	case "/routez":
		body = `{"routes":[{"remote_id":"B","remote_name":"nats-b","rtt":"1ms"}]}`
	case "/gatewayz":
		body = `{"outbound_gateways":{},"inbound_gateways":{}}`
	case "/leafz":
		body = `{"leafs":[]}`
	case "/jsz":
		if request.URL.Query().Get("accounts") == "true" {
			body = `{"account_details":[{"name":"orders","id":"A","memory":100,"storage":200,"streams":3,"consumers":4,"messages":50}]}`
		} else {
			body = `{"memory":10,"storage":20,"meta_cluster":{"leader":"nats-a"}}`
		}
	case "/connz":
		body = `{"server_id":"A","connections":[{"cid":42,"name":"worker","rtt":"2ms","subscriptions_list_detail":[{"subject":"orders.*"}]}]}`
	case "/accountz":
		if request.URL.Query().Get("acc") == "A" {
			body = `{"account_detail":{"account_name":"A","name_tag":"orders","client_connections":2,"subscriptions":4,"jwt":"jwt","decoded_jwt":{"nats":{"limits":{"conn":10}}}}}`
		} else {
			body = `{"system_account":"SYS","accounts":["A"]}`
		}
	}
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

func TestHTTPOperatorCatalogDetectsCapabilities(t *testing.T) {
	t.Parallel()
	collector, err := newHTTPTopology("http://127.0.0.1:8222", monitoringDoer{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := collector.OperatorCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	accounts := catalog.Accounts()
	if !catalog.Capabilities().OperatorMode || !catalog.Capabilities().ClaimsRead || len(accounts) != 1 || accounts[0].Limits.Connections != 10 || accounts[0].Streams != 3 || accounts[0].Consumers != 4 || accounts[0].Messages != 50 {
		t.Fatalf("capabilities=%#v accounts=%#v", catalog.Capabilities(), accounts)
	}
}

func TestHTTPConnectionFallbackIncludesSubscriptionDetails(t *testing.T) {
	t.Parallel()
	collector, err := newHTTPTopology("http://127.0.0.1:8222", monitoringDoer{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.ListConnections(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Connections) != 1 || snapshot.Connections[0].CID != 42 || len(snapshot.Connections[0].Subscriptions) != 1 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestHTTPTopologySupportsServersWithoutSystemAccount(t *testing.T) {
	t.Parallel()
	collector, err := newHTTPTopology("http://127.0.0.1:8222/", monitoringDoer{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Topology(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	server := snapshot.Servers()[0]
	if server.Name() != "nats-a" || server.Cluster() != "C1" || server.Domain() != "edge" || len(server.Links()) != 1 || server.JetStream().Storage() != 20 {
		t.Fatalf("server=%#v", server)
	}
}
