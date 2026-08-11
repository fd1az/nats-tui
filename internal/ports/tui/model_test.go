package tui

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	appconnections "github.com/clcsolutions/nats-tui/internal/app/connections"
	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/clcsolutions/nats-tui/internal/app/subjects"
	apptopology "github.com/clcsolutions/nats-tui/internal/app/topology"
	domainrequestreply "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
	domaintopology "github.com/clcsolutions/nats-tui/internal/domain/topology"
	"github.com/muesli/termenv"
)

func TestAppStartsWithPresentationThenRequestsConnection(t *testing.T) {
	t.Parallel()

	model := newTestModel(nil)
	if model.screen != screenWelcome {
		t.Fatalf("screen = %v, want welcome", model.screen)
	}
	if !strings.Contains(model.View(), "NATS TUI") {
		t.Fatalf("welcome view does not contain product name:\n%s", model.View())
	}

	updated, _ := model.Update(welcomeDoneMsg{})
	connectionModel := updated.(Model)

	if connectionModel.screen != screenConnection {
		t.Fatalf("screen = %v, want connection", connectionModel.screen)
	}
	for _, text := range []string{"Connect to NATS", "Server URL", "No authentication"} {
		if !strings.Contains(connectionModel.View(), text) {
			t.Errorf("connection view does not contain %q:\n%s", text, connectionModel.View())
		}
	}
}

func TestConnectionScreenSelectsRecentProfileWithoutExposingSecrets(t *testing.T) {
	var connected ConnectionInput
	model := newTestModel(func(input ConnectionInput) tea.Cmd {
		connected = input
		return func() tea.Msg { return ConnectionFailed(errors.New("stop")) }
	})
	updated, _ := model.Update(ConnectionProfilesLoaded([]appconnections.ProfileSummary{{Name: "prod", Servers: []string{"nats://a:4222", "nats://b:4222"}, Auth: "user-password", Username: "ops"}}))
	selected, _ := updated.(Model).Update(welcomeDoneMsg{})
	view := selected.(Model).View()
	if !strings.Contains(view, "prod") || !strings.Contains(view, "user-password") || strings.Contains(view, "secret") {
		t.Fatalf("view=%s", view)
	}
	_, command := selected.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil || !connected.UseProfile || connected.Profile != "prod" {
		t.Fatalf("input=%+v", connected)
	}
}

func TestConnectionsWorkspaceLoadsFiltersDetailsJumpsAndConfirmsClose(t *testing.T) {
	t.Parallel()
	snapshot := appconnections.Snapshot{ObservedAt: time.Now(), Connections: []appconnections.Connection{{ServerID: "A", ServerName: "nats-a", CID: 42, Name: "worker", User: "svc", Lang: "go", PendingBytes: 2 << 20, Subscriptions: []appconnections.Subscription{{Subject: "orders.*", Queue: "workers"}}}}}
	var closed uint64
	model := dashboardModel(t, 120, 34).WithConnections(ConnectionServices{Load: func() tea.Cmd { return func() tea.Msg { return ConnectionsLoaded(snapshot) } }, Close: func(_ string, cid uint64) tea.Cmd {
		closed = cid
		return func() tea.Msg { return ConnectionClosed("A", cid) }
	}})
	loading, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	if command == nil || loading.(Model).activeWorkspace != 9 {
		t.Fatal("connections workspace did not load")
	}
	loaded, _ := loading.(Model).Update(command())
	view := loaded.(Model).View()
	for _, want := range []string{"Connections & Subscriptions", "worker", "slow"} {
		if !strings.Contains(strings.ToLower(view), strings.ToLower(want)) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	detail, _ := loaded.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	jumped, _ := detail.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if jumped.(Model).activeWorkspace != 6 || jumped.(Model).subjectSearchQuery != "orders.*" {
		t.Fatalf("jump failed: workspace=%d query=%q", jumped.(Model).activeWorkspace, jumped.(Model).subjectSearchQuery)
	}
	confirm, _ := detail.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !strings.Contains(confirm.(Model).View(), "Close live connection?") {
		t.Fatal("confirmation missing")
	}
	_, closeCommand := confirm.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if closeCommand == nil || closed != 42 {
		t.Fatalf("closed=%d", closed)
	}
}

func TestTopologyWorkspaceLoadsGraphsDetailsAndConfirmsReload(t *testing.T) {
	t.Parallel()
	link, _ := domaintopology.NewLink(domaintopology.LinkRoute, "A", "B", "nats-b", true, 0, 2*time.Millisecond)
	server, _ := domaintopology.NewServer(domaintopology.ServerInput{ID: "A", Name: "nats-a", Cluster: "C1", Version: "2.14.4", Connections: 3, Subscriptions: 8, Links: []domaintopology.Link{link}, JetStream: domaintopology.NewResources(10, 20, 0, 0)})
	snapshot, _ := domaintopology.NewSnapshot([]domaintopology.Server{server}, domaintopology.MetaGroup{Leader: "nats-a"})
	var operated apptopology.Command
	model := dashboardModel(t, 120, 34).WithTopology(TopologyServices{
		Load: func() tea.Cmd { return func() tea.Msg { return TopologyLoaded(snapshot) } },
		Operate: func(command apptopology.Command) tea.Cmd {
			operated = command
			return func() tea.Msg { return TopologyOperationSucceeded(command) }
		},
	})
	loading, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	if command == nil || loading.(Model).activeWorkspace != 8 {
		t.Fatal("topology workspace did not start loading")
	}
	loaded, _ := loading.(Model).Update(command())
	view := loaded.(Model).View()
	for _, want := range []string{"Server Topology", "nats-a", "META", "2.14.4"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	graph, _ := loaded.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !strings.Contains(graph.(Model).View(), "Topology graph") || !strings.Contains(graph.(Model).View(), "route") {
		t.Fatalf("graph missing:\n%s", graph.(Model).View())
	}
	confirm, _ := graph.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if !strings.Contains(confirm.(Model).View(), "Confirm reload") {
		t.Fatalf("confirmation missing:\n%s", confirm.(Model).View())
	}
	_, operation := confirm.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if operation == nil || operated.Action != apptopology.ActionReload || operated.ServerID != "A" {
		t.Fatalf("operation=%#v", operated)
	}
}

func TestEnterStartsConnectionAndShowsLoading(t *testing.T) {
	t.Parallel()

	called := false
	connect := func(input ConnectionInput) tea.Cmd {
		called = input.Server == "nats://127.0.0.1:4222"
		return func() tea.Msg { return ConnectionFailed(errors.New("not running")) }
	}
	model := newTestModel(connect)
	model.screen = screenConnection

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	connecting := updated.(Model)

	if command == nil {
		t.Fatal("connection command = nil")
	}
	if !called {
		t.Fatal("connect function was not called with the server URL")
	}
	if connecting.screen != screenConnecting {
		t.Fatalf("screen = %v, want connecting", connecting.screen)
	}
	if !strings.Contains(connecting.View(), "Connecting") {
		t.Fatalf("loading view does not contain Connecting:\n%s", connecting.View())
	}
}

func TestConnectionFailureReturnsToFormWithError(t *testing.T) {
	t.Parallel()

	model := newTestModel(nil)
	model.screen = screenConnecting
	updated, _ := model.Update(connectionFailedMsg{err: errors.New("connection refused")})
	failed := updated.(Model)

	if failed.screen != screenConnection {
		t.Fatalf("screen = %v, want connection", failed.screen)
	}
	if !strings.Contains(failed.View(), "connection refused") {
		t.Fatalf("view does not contain connection error:\n%s", failed.View())
	}
}

func TestWorkspaceNavigationMovesBetweenSections(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 32)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	streams := updated.(Model)

	if streams.activeWorkspace != 1 {
		t.Fatalf("active workspace = %d, want 1", streams.activeWorkspace)
	}
	if !strings.Contains(streams.View(), "Stream Management") {
		t.Fatalf("view does not show stream workspace:\n%s", streams.View())
	}

	updated, _ = streams.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	subjects := updated.(Model)
	if subjects.activeWorkspace != 6 {
		t.Fatalf("active workspace = %d, want 6", subjects.activeWorkspace)
	}
	if !strings.Contains(subjects.View(), "Subject Explorer") {
		t.Fatalf("view does not show subject workspace:\n%s", subjects.View())
	}
}

func TestSubjectExplorerRendersTreeAndMovesSelection(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 32)
	model.activeWorkspace = 6
	model.streams = []streams.Summary{
		{Name: "EVENTS", Subjects: []string{"events.payment.success", "events.user.*"}},
	}
	model.rebuildSubjects()

	view := model.View()
	for _, text := range []string{"Subject Explorer", "events", "payment", "success", "EVENTS"} {
		if !strings.Contains(view, text) {
			t.Errorf("subject explorer does not contain %q:\n%s", text, view)
		}
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(Model).subjectCursor; got != 1 {
		t.Fatalf("subject cursor = %d, want 1", got)
	}
}

func TestSubjectExplorerSearchFiltersReactively(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 28)
	model.activeWorkspace = 6
	model.subjects = subjects.Build([]streams.Summary{
		{Name: "PAYMENTS", Subjects: []string{"events.payment.success"}},
		{Name: "USERS", Subjects: []string{"events.user.signup"}},
	})

	searching, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	searched, _ := searching.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("payment")})
	got := searched.(Model)
	if len(got.displayedSubjects()) != 1 || got.displayedSubjects()[0].Subject != "events.payment.success" {
		t.Fatalf("filtered subjects = %#v", got.displayedSubjects())
	}
}

func TestEnterOnSubjectJumpsToLiveMessageWatch(t *testing.T) {
	t.Parallel()

	var watched []string
	start := func(_ string, patterns []string) tea.Cmd {
		watched = append([]string(nil), patterns...)
		return func() tea.Msg { return WatchStarted(make(chan messages.Record)) }
	}
	model := dashboardModel(t, 100, 28).WithMessageWatch(start, nil)
	model.activeWorkspace = 6
	model.subjects = []subjects.Node{{Subject: "events.payment", WatchSubject: "events.payment.>"}}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	watching := updated.(Model)
	if command == nil || watching.activeWorkspace != 3 || !watching.watchStarting {
		t.Fatalf("subject watch did not start: command=%v workspace=%d starting=%v", command != nil, watching.activeWorkspace, watching.watchStarting)
	}
	if !reflect.DeepEqual(watched, []string{"events.payment.>"}) {
		t.Fatalf("watched subjects = %#v", watched)
	}
	if watching.messageStream != "events.payment.>" {
		t.Fatalf("message source = %q", watching.messageStream)
	}
}

func TestSubjectObservationMergesLiveSubjectsAndRates(t *testing.T) {
	t.Parallel()
	events := make(chan subjects.TrafficSample, 2)
	start := func() tea.Cmd { return func() tea.Msg { return SubjectObservationStarted(events) } }
	model := dashboardModel(t, 120, 30).WithSubjectObservation(start, nil)
	model.streams = []streams.Summary{{Name: "EVENTS", Subjects: []string{"events.>"}}}
	model.rebuildSubjects()

	selected, startCommand := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	if startCommand == nil || !selected.(Model).subjectObservationStarting {
		t.Fatalf("observation did not start: command=%v model=%+v", startCommand != nil, selected.(Model))
	}
	startedValue, _ := selected.(Model).Update(SubjectObservationStarted(events))
	started := startedValue.(Model)
	events <- subjects.TrafficSample{Subject: "events.payment.created", Bytes: 120, At: time.Now()}
	close(events)
	batch := started.waitForSubjectObservationBatch(events)()
	observedValue, _ := started.Update(batch)
	observed := observedValue.(Model)
	if observed.subjectObservationReceived != 1 {
		t.Fatalf("received=%d", observed.subjectObservationReceived)
	}
	view := observed.View()
	for _, want := range []string{"payment", "created", "msg/s", "LIVE"} {
		if !strings.Contains(view, want) {
			t.Fatalf("observed explorer missing %q:\n%s", want, view)
		}
	}
}

func TestSubjectActionsOpenPrefilledPublishAndRequestComposer(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 100, 28)
	model.activeWorkspace = 6
	model.subjects = []subjects.Node{{Subject: "events.payment.created", WatchSubject: "events.payment.created", Observed: true}}

	publishValue, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	publish := publishValue.(Model)
	if publish.activeWorkspace != 7 || publish.requestReply.requestMode || publish.requestReply.subject.Value() != "events.payment.created" {
		t.Fatalf("publish workspace=%d request=%v subject=%q", publish.activeWorkspace, publish.requestReply.requestMode, publish.requestReply.subject.Value())
	}

	requestValue, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	request := requestValue.(Model)
	if request.activeWorkspace != 7 || !request.requestReply.requestMode || request.requestReply.subject.Value() != "events.payment.created" {
		t.Fatalf("request workspace=%d request=%v subject=%q", request.activeWorkspace, request.requestReply.requestMode, request.requestReply.subject.Value())
	}
}

func TestStreamsWorkspaceRendersRealCatalogAndMovesSelection(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 32)
	model.activeWorkspace = 1
	model.streams = []streams.Summary{
		{Name: "ORDERS", Subjects: []string{"orders.>"}, Messages: 1420, Bytes: 2_400_000, Consumers: 3},
		{Name: "AUDIT", Subjects: []string{"audit.*"}, Messages: 91, Bytes: 84_000, Consumers: 1},
	}

	view := model.View()
	for _, text := range []string{"ORDERS", "orders.>", "1.4K", "2.4 MB", "AUDIT"} {
		if !strings.Contains(view, text) {
			t.Errorf("stream view does not contain %q:\n%s", text, view)
		}
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(Model).streamCursor; got != 1 {
		t.Fatalf("stream cursor = %d, want 1", got)
	}
}

func TestStreamSelectionStaysWithinCatalog(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 80, 24)
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ONLY"}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(Model).streamCursor; got != 0 {
		t.Fatalf("stream cursor = %d, want 0", got)
	}
}

func TestKeyValueWorkspaceRendersBucketCatalogAndMovesSelection(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 32)
	model.activeWorkspace = 4
	model.keyValueBuckets = []kvstore.BucketSummary{
		{Name: "SESSIONS", Description: "active sessions", Values: 12, Bytes: 2048, History: 5, TTL: time.Hour, Storage: kvstore.StorageMemory, Replicas: 3},
		{Name: "CONFIG", Values: 2, Bytes: 512, History: 1, Storage: kvstore.StorageFile, Replicas: 1},
	}

	view := model.View()
	for _, text := range []string{"Key-Value Stores", "SESSIONS", "MEMORY", "2.0 KB", "1h", "R3", "active sessions"} {
		if !strings.Contains(view, text) {
			t.Errorf("KV view does not contain %q:\n%s", text, view)
		}
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(Model).keyValueCursor; got != 1 {
		t.Fatalf("KV cursor = %d, want 1", got)
	}
}

func TestKeyValueCreateFormUsesSharedDefinition(t *testing.T) {
	t.Parallel()

	var input KeyValueCreateInput
	create := func(got KeyValueCreateInput) tea.Cmd {
		input = got
		return func() tea.Msg { return KeyValueCreateSucceeded(got.Generation, got.Definition) }
	}
	model := dashboardModel(t, 100, 30).WithKeyValueCreate(create)
	model.activeWorkspace = 4

	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	form := opened.(Model)
	if form.keyValueForm == nil || !strings.Contains(form.View(), "Create KV bucket") {
		t.Fatalf("KV create form not opened:\n%s", form.View())
	}
	form.keyValueForm.name = "SESSIONS"
	form.keyValueForm.description = "active sessions"
	form.keyValueForm.storage = kvstore.StorageMemory
	form.keyValueForm.history = "5"
	form.keyValueForm.ttl = "1h"
	form.keyValueForm.maxBytes = "2048"
	form.keyValueForm.maxValueSize = "512"
	form.keyValueForm.replicas = "3"
	form.keyValueForm.compression = true

	updated, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	saving := updated.(Model)
	if command == nil || !saving.keyValueCreateRunning {
		t.Fatalf("KV create did not start: command=%v running=%v", command != nil, saving.keyValueCreateRunning)
	}
	want := kvstore.Definition{
		Name: "SESSIONS", Description: "active sessions", Storage: kvstore.StorageMemory,
		History: 5, TTL: time.Hour, MaxBytes: 2048, MaxValueSize: 512, Replicas: 3, Compression: true,
	}
	if !reflect.DeepEqual(input.Definition, want) {
		t.Fatalf("definition = %#v, want %#v", input.Definition, want)
	}

	succeeded, _ := saving.Update(KeyValueCreateSucceeded(input.Generation, input.Definition))
	created := succeeded.(Model)
	if created.keyValueForm != nil || created.keyValueCreateRunning || len(created.keyValueBuckets) != 1 || created.keyValueBuckets[0].Name != "SESSIONS" {
		t.Fatalf("created state = %#v", created.keyValueBuckets)
	}
}

func TestKeyValueBucketFormIsCenteredAndExplainsFocusedField(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 40)
	form := newKeyValueForm()
	model.keyValueForm = &form
	view := model.workspaceView(120, 40)
	plain := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(view, "")
	lines := strings.Split(plain, "\n")
	titleLine := -1
	for index, line := range lines {
		if strings.Contains(line, "Create KV bucket") {
			titleLine = index
			if len(line)-len(strings.TrimLeft(line, " ")) < 10 {
				t.Fatalf("form is not horizontally centered:\n%s", plain)
			}
			break
		}
	}
	if titleLine < 3 || !strings.Contains(plain, "Required") || !strings.Contains(plain, "Example: SESSIONS") || !strings.Contains(plain, "Field 1 of 11") {
		t.Fatalf("centered contextual form missing:\n%s", plain)
	}

	form.field = 4
	view = model.workspaceView(120, 40)
	if !strings.Contains(view, "30m") || !strings.Contains(view, "never expire") {
		t.Fatalf("TTL help missing:\n%s", view)
	}
}

func TestKeyValueBucketFormFitsNarrowViewport(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 48, 24)
	form := newKeyValueForm()
	model.keyValueForm = &form
	view := model.workspaceView(48, 24)
	if got := lipgloss.Width(view); got != 48 {
		t.Fatalf("form width = %d, want 48", got)
	}
	if got := lipgloss.Height(view); got != 24 {
		t.Fatalf("form height = %d, want 24", got)
	}
	for _, want := range []string{"Create KV bucket", "Field 1 of 11", "Example:", "SESSIONS", "ctrl+s"} {
		if !strings.Contains(view, want) {
			t.Fatalf("narrow form missing %q:\n%s", want, view)
		}
	}
}

func TestKeyValueBucketFormFitsMinimumViewport(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 32, 20)
	form := newKeyValueForm()
	model.keyValueForm = &form
	view := model.workspaceView(32, 20)
	if got := lipgloss.Width(view); got != 32 {
		t.Fatalf("form width = %d, want 32", got)
	}
	if got := lipgloss.Height(view); got != 20 {
		t.Fatalf("form height = %d, want 20", got)
	}
	if !strings.Contains(view, "Create KV bucket") || !strings.Contains(view, "ctrl+s") {
		t.Fatalf("minimum form lost primary actions:\n%s", view)
	}
}

func TestKeyValueEditFormKeepsNameAndStorageImmutable(t *testing.T) {
	t.Parallel()

	var input KeyValueUpdateInput
	update := func(got KeyValueUpdateInput) tea.Cmd {
		input = got
		return func() tea.Msg { return KeyValueUpdateSucceeded(got.Generation, got.Name, got.Changes) }
	}
	model := dashboardModel(t, 100, 30).WithKeyValueUpdate(update)
	model.activeWorkspace = 4
	model.keyValueBuckets = []kvstore.BucketSummary{
		{Name: "SESSIONS", Description: "old", Storage: kvstore.StorageFile, History: 5, TTL: time.Hour, MaxBytes: 1000, MaxValueSize: 100, Replicas: 1},
	}
	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	form := opened.(Model)
	if form.keyValueForm == nil || !form.keyValueForm.editing || form.keyValueForm.name != "SESSIONS" {
		t.Fatal("KV edit form not populated")
	}
	form.keyValueForm.description = "new"
	form.keyValueForm.history = "7"
	updated, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || !updated.(Model).keyValueCreateRunning {
		t.Fatal("KV update did not start")
	}
	if input.Name != "SESSIONS" || input.Changes.Description == nil || *input.Changes.Description != "new" || input.Changes.History == nil || *input.Changes.History != 7 {
		t.Fatalf("update input = %#v", input)
	}
}

func TestKeyValueDeleteShowsImpactAndRequiresConfirmation(t *testing.T) {
	t.Parallel()

	var input KeyValueDeleteInput
	remove := func(got KeyValueDeleteInput) tea.Cmd {
		input = got
		return func() tea.Msg { return KeyValueDeleteSucceeded(got.Generation, got.Name) }
	}
	model := dashboardModel(t, 100, 30).WithKeyValueDelete(remove)
	model.activeWorkspace = 4
	model.keyValueBuckets = []kvstore.BucketSummary{{Name: "SESSIONS", Values: 42, Bytes: 2048}}

	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	confirming := opened.(Model)
	if !confirming.confirmKeyValueDelete || !strings.Contains(confirming.View(), "42 values") || !strings.Contains(confirming.View(), "cannot be undone") {
		t.Fatalf("delete impact missing:\n%s", confirming.View())
	}
	confirmed, command := confirming.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if command == nil || !confirmed.(Model).keyValueCreateRunning || input.Name != "SESSIONS" {
		t.Fatal("confirmed KV delete did not start")
	}
	deleted, _ := confirmed.(Model).Update(KeyValueDeleteSucceeded(input.Generation, input.Name))
	if len(deleted.(Model).keyValueBuckets) != 0 {
		t.Fatal("deleted bucket remains in catalog")
	}
}

func TestEnterOnKVBucketLoadsKeysAndSearchFiltersReactively(t *testing.T) {
	t.Parallel()

	loadedBucket := ""
	loader := func(bucket string) tea.Cmd {
		loadedBucket = bucket
		return func() tea.Msg {
			return KeyValueKeysLoaded(bucket, []string{"session.alpha", "session.beta", "tenant.config"})
		}
	}
	model := dashboardModel(t, 100, 30).WithKeyValueKeys(loader)
	model.activeWorkspace = 4
	model.keyValueBuckets = []kvstore.BucketSummary{{Name: "SESSIONS"}}
	opened, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil || loadedBucket != "SESSIONS" || !opened.(Model).loadingKeyValueKeys {
		t.Fatal("key loading did not start")
	}
	loaded, _ := opened.(Model).Update(KeyValueKeysLoaded("SESSIONS", []string{"session.alpha", "session.beta", "tenant.config"}))
	browser := loaded.(Model)
	if !browser.browsingKeyValue || len(browser.keyValueKeys) != 3 || !strings.Contains(browser.View(), "session.alpha") {
		t.Fatal("key browser not opened")
	}
	searching, _ := browser.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	filtered, _ := searching.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("beta")})
	if got := filtered.(Model).displayedKeyValueKeys(); len(got) != 1 || got[0] != "session.beta" {
		t.Fatalf("filtered keys = %#v", got)
	}
}

func TestEnterOnKVKeyLoadsResponsiveJSONDetail(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 8, 7, 14, 30, 0, 0, time.UTC)
	loadedBucket, loadedKey := "", ""
	loader := func(bucket, key string) tea.Cmd {
		loadedBucket, loadedKey = bucket, key
		return func() tea.Msg {
			return KeyValueEntryLoaded(kvstore.Entry{Bucket: bucket, Key: key, Value: []byte(`{"active":true,"count":42}`), Operation: kvstore.OperationPut, Revision: 9, Created: created, Delta: 0})
		}
	}
	model := dashboardModel(t, 72, 24).WithKeyValueEntry(loader)
	model.activeWorkspace = 4
	model.browsingKeyValue = true
	model.keyValueBucket = "SESSIONS"
	model.keyValueKeys = []string{"session.42"}

	loading, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil || !loading.(Model).loadingKeyValueEntry || loadedBucket != "SESSIONS" || loadedKey != "session.42" {
		t.Fatal("entry loading did not start")
	}
	loaded, _ := loading.(Model).Update(KeyValueEntryLoaded(kvstore.Entry{Bucket: "SESSIONS", Key: "session.42", Value: []byte(`{"active":true,"count":42}`), Operation: kvstore.OperationPut, Revision: 9, Created: created}))
	view := loaded.(Model).View()
	for _, want := range []string{"session.42", "Operation", "PUT", "Revision", "9", "Created", "VALUE", `"active"`} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail missing %q:\n%s", want, view)
		}
	}
}

func TestKVEntryCreateFormWritesValueAndOpensStoredRevision(t *testing.T) {
	t.Parallel()

	var input KeyValuePutInput
	put := func(got KeyValuePutInput) tea.Cmd {
		input = got
		return func() tea.Msg {
			return KeyValuePutSucceeded(got.Generation, kvstore.Entry{Bucket: got.Command.Bucket, Key: got.Command.Key, Value: got.Command.Value, Operation: kvstore.OperationPut, Revision: 1})
		}
	}
	model := dashboardModel(t, 100, 30).WithKeyValuePut(put)
	model.activeWorkspace = 4
	model.browsingKeyValue = true
	model.keyValueBucket = "CONFIG"

	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	form := opened.(Model)
	if form.keyValueEntryForm == nil || !strings.Contains(form.View(), "Create KV entry") {
		t.Fatalf("entry form not opened:\n%s", form.View())
	}
	form.keyValueEntryForm.key.SetValue("feature.api")
	form.keyValueEntryForm.value.SetValue(`{"enabled":true}`)
	saving, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || !saving.(Model).keyValueEntrySaving || input.Command.Key != "feature.api" || string(input.Command.Value) != `{"enabled":true}` || input.Command.ExpectedRevision != nil {
		t.Fatalf("saving=%v input=%#v", saving.(Model).keyValueEntrySaving, input)
	}
	stored, _ := saving.(Model).Update(KeyValuePutSucceeded(input.Generation, kvstore.Entry{Bucket: "CONFIG", Key: "feature.api", Value: input.Command.Value, Operation: kvstore.OperationPut, Revision: 1}))
	if stored.(Model).keyValueEntryForm != nil || !stored.(Model).showKeyValueEntry || stored.(Model).keyValueEntry.Revision != 1 {
		t.Fatal("stored entry detail was not opened")
	}
}

func TestKVEntryEditUsesLoadedRevisionForCAS(t *testing.T) {
	t.Parallel()

	var input KeyValuePutInput
	put := func(got KeyValuePutInput) tea.Cmd {
		input = got
		return func() tea.Msg { return KeyValuePutSucceeded(got.Generation, kvstore.Entry{}) }
	}
	entry := kvstore.Entry{Bucket: "CONFIG", Key: "feature.api", Value: []byte(`{"enabled":true}`), Revision: 9}
	model := dashboardModel(t, 100, 30).WithKeyValuePut(put)
	model.activeWorkspace = 4
	model.browsingKeyValue = true
	model.showKeyValueEntry = true
	model.keyValueEntry = &entry

	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	form := opened.(Model)
	if form.keyValueEntryForm == nil || !form.keyValueEntryForm.editing {
		t.Fatal("edit entry form not opened")
	}
	form.keyValueEntryForm.value.SetValue(`{"enabled":false}`)
	_, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || input.Command.ExpectedRevision == nil || *input.Command.ExpectedRevision != 9 {
		t.Fatalf("CAS input = %#v", input.Command)
	}
}

func TestKVEntryDeleteConfirmsRecoverableMarkerWithCAS(t *testing.T) {
	t.Parallel()

	var input KeyValueRemoveInput
	remove := func(got KeyValueRemoveInput) tea.Cmd {
		input = got
		return func() tea.Msg { return KeyValueRemoveSucceeded(got.Generation, got.Command) }
	}
	entry := kvstore.Entry{Bucket: "CONFIG", Key: "feature.api", Revision: 9}
	model := dashboardModel(t, 100, 30).WithKeyValueRemove(remove)
	model.activeWorkspace = 4
	model.browsingKeyValue = true
	model.showKeyValueEntry = true
	model.keyValueEntry = &entry
	model.keyValueKeys = []string{"feature.api"}

	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	confirming := opened.(Model)
	if confirming.pendingKeyValueEntryRemoval != kvstore.RemoveDelete || !strings.Contains(confirming.View(), "history remains") || !strings.Contains(confirming.View(), "Revision 9") {
		t.Fatalf("delete confirmation missing impact:\n%s", confirming.View())
	}
	saving, command := confirming.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if command == nil || !saving.(Model).keyValueEntrySaving || input.Command.ExpectedRevision == nil || *input.Command.ExpectedRevision != 9 || input.Command.Action != kvstore.RemoveDelete {
		t.Fatalf("delete input = %#v", input)
	}
	removed, _ := saving.(Model).Update(KeyValueRemoveSucceeded(input.Generation, input.Command))
	if removed.(Model).showKeyValueEntry || len(removed.(Model).keyValueKeys) != 0 {
		t.Fatal("removed key remains visible")
	}
}

func TestKVEntryPurgeWarnsThatHistoryIsDestroyed(t *testing.T) {
	t.Parallel()

	entry := kvstore.Entry{Bucket: "CONFIG", Key: "feature.api", Revision: 9}
	model := dashboardModel(t, 100, 30).WithKeyValueRemove(func(KeyValueRemoveInput) tea.Cmd { return nil })
	model.activeWorkspace = 4
	model.browsingKeyValue = true
	model.showKeyValueEntry = true
	model.keyValueEntry = &entry

	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	confirming := opened.(Model)
	if confirming.pendingKeyValueEntryRemoval != kvstore.RemovePurge || !strings.Contains(confirming.View(), "permanently removes all previous revisions") || !strings.Contains(confirming.View(), "cannot be undone") {
		t.Fatalf("purge warning missing:\n%s", confirming.View())
	}
}

func TestDInKVKeyListNeverTargetsBucket(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 30)
	model.activeWorkspace = 4
	model.browsingKeyValue = true
	model.keyValueBuckets = []kvstore.BucketSummary{{Name: "CONFIG"}}
	model.keyValueKeys = []string{"feature.api"}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if updated.(Model).confirmKeyValueDelete {
		t.Fatal("d in key list targeted the bucket")
	}
}

func TestKVHistoryTimelineOpensHistoricalValueWithoutAnotherRequest(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 8, 7, 18, 30, 0, 0, time.UTC)
	loads := 0
	loader := func(bucket, key string) tea.Cmd {
		loads++
		return func() tea.Msg {
			return KeyValueHistoryLoaded(bucket, key, []kvstore.Entry{
				{Bucket: bucket, Key: key, Value: []byte(`{"version":1}`), Operation: kvstore.OperationPut, Revision: 7, Created: created, Delta: 1},
				{Bucket: bucket, Key: key, Value: []byte(`{"version":2}`), Operation: kvstore.OperationPut, Revision: 8, Created: created.Add(time.Minute), Delta: 0},
			})
		}
	}
	entry := kvstore.Entry{Bucket: "CONFIG", Key: "feature.api", Revision: 8}
	model := dashboardModel(t, 100, 30).WithKeyValueHistory(loader)
	model.activeWorkspace = 4
	model.browsingKeyValue = true
	model.showKeyValueEntry = true
	model.keyValueEntry = &entry

	loading, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	if command == nil || !loading.(Model).loadingKeyValueHistory || loads != 1 {
		t.Fatal("history loading did not start")
	}
	loaded, _ := loading.(Model).Update(KeyValueHistoryLoaded("CONFIG", "feature.api", []kvstore.Entry{
		{Bucket: "CONFIG", Key: "feature.api", Value: []byte(`{"version":1}`), Operation: kvstore.OperationPut, Revision: 7, Created: created, Delta: 1},
		{Bucket: "CONFIG", Key: "feature.api", Value: []byte(`{"version":2}`), Operation: kvstore.OperationPut, Revision: 8, Created: created.Add(time.Minute)},
	}))
	timeline := loaded.(Model)
	if !timeline.showKeyValueHistory || timeline.keyValueHistoryCursor != 1 || !strings.Contains(timeline.View(), "Revision History") || !strings.Contains(timeline.View(), "7") {
		t.Fatalf("timeline missing:\n%s", timeline.View())
	}
	older, _ := timeline.Update(tea.KeyMsg{Type: tea.KeyUp})
	detail, _ := older.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if loads != 1 || !detail.(Model).showKeyValueHistoryEntry || !strings.Contains(detail.(Model).View(), "Historical revision 7") || !strings.Contains(detail.(Model).View(), `"version"`) {
		t.Fatalf("historical detail missing:\n%s", detail.(Model).View())
	}
}

func TestEnterOnStreamLoadsMessagesAndOpensMonitor(t *testing.T) {
	t.Parallel()

	loadedStream := ""
	loader := func(stream string) tea.Cmd {
		loadedStream = stream
		return func() tea.Msg {
			return MessagesLoaded(stream, []messages.Record{{Sequence: 9, Subject: "orders.created", Payload: []byte(`{"id":42}`)}})
		}
	}
	model := NewModel("local", "nats://127.0.0.1:4222", nil, loader)
	model.screen = screenWorkspace
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ORDERS"}}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	loading := updated.(Model)
	if command == nil || loadedStream != "ORDERS" || !loading.loadingMessages {
		t.Fatalf("message loading did not start: command=%v stream=%q loading=%v", command != nil, loadedStream, loading.loadingMessages)
	}

	loaded, _ := loading.Update(MessagesLoaded("ORDERS", []messages.Record{{Sequence: 9, Subject: "orders.created", Payload: []byte(`{"id":42}`)}}))
	monitor := loaded.(Model)
	if monitor.activeWorkspace != 3 || monitor.loadingMessages {
		t.Fatalf("workspace=%d loading=%v, want message workspace and loaded", monitor.activeWorkspace, monitor.loadingMessages)
	}
	if view := monitor.View(); !strings.Contains(view, "orders.created") || !strings.Contains(view, "#9") {
		t.Fatalf("message monitor missing loaded record:\n%s", view)
	}
}

func TestEnterOnMessageOpensFormattedJSONDetail(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 30)
	model.activeWorkspace = 3
	model.messages = []messages.Record{{Sequence: 9, Subject: "orders.created", Payload: []byte(`{"id":42,"paid":true}`)}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	detail := updated.(Model)
	if !detail.showMessageDetail {
		t.Fatal("showMessageDetail = false, want true")
	}
	for _, text := range []string{"orders.created", `"id": 42`, `"paid": true`} {
		if !strings.Contains(detail.View(), text) {
			t.Errorf("message detail missing %q:\n%s", text, detail.View())
		}
	}
}

func TestMessageDetailScrollsWithoutGrowingPastViewport(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 80, 24)
	model.activeWorkspace = 3
	model.showMessageDetail = true
	model.messages = []messages.Record{{Sequence: 1, Subject: "events", Payload: []byte("{\n" + strings.Repeat("  \"field\": true,\n", 30) + "  \"last\": null\n}")}}

	before := model.View()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	after := updated.(Model).View()

	if before == after {
		t.Fatal("detail did not scroll")
	}
	if height := lipgloss.Height(after); height != 24 {
		t.Fatalf("detail height = %d, want 24", height)
	}
}

func TestMessageListScrollKeepsCursorInsideVisibleWindow(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 90, 18)
	model.activeWorkspace = 3
	for sequence := uint64(1); sequence <= 30; sequence++ {
		model.messages = append(model.messages, messages.Record{
			Sequence: sequence,
			Subject:  fmt.Sprintf("events.%02d", sequence),
			Payload:  []byte(`{"ok":true}`),
		})
	}
	for range 20 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}

	view := model.messagesView(86, 9)
	if !strings.Contains(view, "events.21") {
		t.Fatalf("selected message is outside visible window:\n%s", view)
	}
	if strings.Contains(view, "events.01") {
		t.Fatalf("message window did not scroll past first row:\n%s", view)
	}
	if height := lipgloss.Height(view); height > 9 {
		t.Fatalf("message view height = %d, want <= 9", height)
	}
}

func TestWideMessageMonitorShowsListAndSynchronizedPreview(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 140, 30)
	model.activeWorkspace = 3
	model.messageStream = "EVENTS"
	model.messages = []messages.Record{{
		Sequence: 511,
		Subject:  "events.payment.success",
		Payload:  []byte(`{"event":"payment.success","user_id":"usr-08155"}`),
	}}

	view := model.messagesView(136, 20)
	for _, text := range []string{"MESSAGES", "MESSAGE DETAIL", "events.payment.success", `"user_id"`} {
		if !strings.Contains(view, text) {
			t.Errorf("wide monitor missing %q:\n%s", text, view)
		}
	}
}

func TestMessageSearchFiltersReactivelyAcrossSubjectAndPayload(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 26)
	model.activeWorkspace = 3
	model.setMessages([]messages.Record{
		{Sequence: 1, Subject: "orders.created", Payload: []byte(`{"tenant":"alpha"}`)},
		{Sequence: 2, Subject: "payments.settled", Payload: []byte(`{"tenant":"beta"}`)},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("orders alpha")})
	searched := updated.(Model)

	if searched.searchQuery != "orders alpha" {
		t.Fatalf("search query = %q", searched.searchQuery)
	}
	view := searched.View()
	if !strings.Contains(view, "orders.created") || strings.Contains(view, "payments.settled") {
		t.Fatalf("reactive filter rendered wrong records:\n%s", view)
	}
	if !strings.Contains(view, "/ orders alpha") {
		t.Fatalf("search bar missing active query:\n%s", view)
	}
}

func TestBackspaceUpdatesMessageSearchAndEscapeLeavesFilterApplied(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 24)
	model.activeWorkspace = 3
	model.setMessages([]messages.Record{{Subject: "orders"}, {Subject: "order"}})
	model.searchMode = true
	model.searchQuery = "orders"
	model.applyMessageSearch()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	searched := updated.(Model)
	if searched.searchQuery != "order" || len(searched.filteredMessages) != 2 {
		t.Fatalf("query=%q results=%d, want order and 2", searched.searchQuery, len(searched.filteredMessages))
	}
	updated, _ = searched.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).searchMode {
		t.Fatal("search mode still active after escape")
	}
}

func TestWStartsLiveWatchForSelectedStreamSubjects(t *testing.T) {
	t.Parallel()

	startedStream := ""
	var startedSubjects []string
	start := func(stream string, subjects []string) tea.Cmd {
		startedStream = stream
		startedSubjects = subjects
		return func() tea.Msg { return WatchFailed(errors.New("fixture stopped")) }
	}
	model := dashboardModel(t, 120, 26).WithMessageWatch(start, func() {})
	model.activeWorkspace = 3
	model.messageStream = "ORDERS"
	model.streams = []streams.Summary{{Name: "ORDERS", Subjects: []string{"orders.>", "returns.*"}}}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	watching := updated.(Model)
	if command == nil || !watching.watchStarting || startedStream != "ORDERS" {
		t.Fatalf("watch did not start: command=%v starting=%v stream=%q", command != nil, watching.watchStarting, startedStream)
	}
	if !reflect.DeepEqual(startedSubjects, []string{"orders.>", "returns.*"}) {
		t.Fatalf("subjects = %#v", startedSubjects)
	}
}

func TestLiveBatchPrependsMessagesUpdatesSearchAndCapsBuffer(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 26)
	model.activeWorkspace = 3
	existing := make([]messages.Record, messageBufferLimit)
	for index := range existing {
		existing[index] = messages.Record{Sequence: uint64(index + 1), Subject: "history"}
	}
	model.setMessages(existing)
	model.searchQuery = "orders"
	model.applyMessageSearch()

	model.prependLiveMessages([]messages.Record{{Subject: "orders.created", Payload: []byte(`{"tenant":"alpha"}`)}})
	model.liveReceived++

	if len(model.messages) != messageBufferLimit {
		t.Fatalf("buffer length = %d, want %d", len(model.messages), messageBufferLimit)
	}
	if len(model.filteredMessages) != 1 || model.filteredMessages[0].Subject != "orders.created" {
		t.Fatalf("live search results = %#v", model.filteredMessages)
	}
	if model.liveReceived != 1 {
		t.Fatalf("live received = %d, want 1", model.liveReceived)
	}
}

func TestSpacePausesLiveWatchWithoutStoppingIt(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 24)
	model.activeWorkspace = 3
	model.watching = true
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	paused := updated.(Model)
	if !paused.watching || !paused.watchPaused {
		t.Fatalf("watching=%v paused=%v", paused.watching, paused.watchPaused)
	}
	withBatch, _ := paused.Update(watchBatchMsg{records: []messages.Record{{Subject: "ignored.while.paused"}}})
	pausedModel := withBatch.(Model)
	if pausedModel.liveReceived != 1 || len(pausedModel.messages) != 0 {
		t.Fatalf("paused batch received=%d buffered=%d", pausedModel.liveReceived, len(pausedModel.messages))
	}
}

func TestMessageExportUsesImmutableVisibleSnapshot(t *testing.T) {
	t.Parallel()
	var request messages.ExportRequest
	export := func(input messages.ExportRequest) tea.Cmd {
		request = input
		return func() tea.Msg {
			return MessageExportSucceeded(messages.ExportResult{Path: input.Path, Format: string(input.Format), Messages: len(input.Records)})
		}
	}
	model := dashboardModel(t, 120, 28).WithMessageOperations(export, nil)
	model.activeWorkspace = 3
	model.messageStream = "EVENTS"
	model.setMessages([]messages.Record{{Sequence: 1, Subject: "events.one"}, {Sequence: 2, Subject: "events.two"}})

	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	form := opened.(Model)
	if form.messageExportForm == nil || !strings.Contains(form.View(), "WHAT TO PUT HERE") {
		t.Fatalf("export form not rendered:\n%s", form.View())
	}
	submitted, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || !submitted.(Model).messageOperationRunning || len(request.Records) != 2 || request.Format != messages.ExportJSONL {
		t.Fatalf("running=%v request=%+v", submitted.(Model).messageOperationRunning, request)
	}
}

func TestFreezeStopsSubscriptionAndKeepsSnapshot(t *testing.T) {
	t.Parallel()
	stopped := false
	model := dashboardModel(t, 100, 24).WithMessageWatch(nil, func() { stopped = true })
	model.activeWorkspace = 3
	model.watching = true
	model.liveReceived = 12
	model.setMessages([]messages.Record{{Subject: "events.kept"}})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	frozen := updated.(Model)
	if !stopped || frozen.watching || !frozen.watchFrozen || len(frozen.messages) != 1 || !strings.Contains(frozen.watchStatusView(), "FROZEN") {
		t.Fatalf("stopped=%v watching=%v frozen=%v messages=%d status=%q", stopped, frozen.watching, frozen.watchFrozen, len(frozen.messages), frozen.watchStatusView())
	}
}

func TestLiveTelemetryReportsBufferDropsAndSubscriptionPressure(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 26)
	model.activeWorkspace = 3
	model.watching = true
	existing := make([]messages.Record, messageBufferLimit)
	model.setMessages(existing)
	updated, _ := model.Update(watchBatchMsg{records: []messages.Record{{Subject: "events.new", WatchPending: 8, WatchCapacity: 32}}})
	observed := updated.(Model)
	if observed.liveDropped != 1 || observed.watchPressure != 8 || observed.watchCapacity != 32 {
		t.Fatalf("dropped=%d pressure=%d capacity=%d", observed.liveDropped, observed.watchPressure, observed.watchCapacity)
	}
	status := observed.watchStatusView()
	if !strings.Contains(status, "queue 8/32") || !strings.Contains(status, "1 not retained") {
		t.Fatalf("status=%q", status)
	}
}

func TestConsumerWorkspaceRendersLagAndMovesSelection(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 140, 30)
	model.activeWorkspace = 2
	model.consumers = []consumers.Summary{
		{Stream: "ORDERS", Name: "settlement-worker", Status: consumers.StatusActive, Pending: 40, AckPending: 2},
		{Stream: "AUDIT", Name: "archiver", Status: consumers.StatusIdle},
	}

	view := model.View()
	for _, text := range []string{"CONSUMERS", "CONSUMER DETAIL", "settlement-worker", "42", "ACTIVE"} {
		if !strings.Contains(view, text) {
			t.Errorf("consumer dashboard missing %q:\n%s", text, view)
		}
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(Model).consumerCursor; got != 1 {
		t.Fatalf("consumer cursor = %d, want 1", got)
	}
}

func TestConsumerListScrollKeepsSelectionVisible(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 90, 18)
	model.activeWorkspace = 2
	for index := range 30 {
		model.consumers = append(model.consumers, consumers.Summary{Stream: "EVENTS", Name: fmt.Sprintf("worker-%02d", index)})
	}
	for range 20 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}

	view := model.consumerView(86, 9)
	if !strings.Contains(view, "worker-20") || strings.Contains(view, "worker-00") {
		t.Fatalf("consumer viewport did not follow selection:\n%s", view)
	}
	if height := lipgloss.Height(view); height > 9 {
		t.Fatalf("consumer view height = %d, want <= 9", height)
	}
}

func TestPeriodicRefreshUpdatesMetricsAndPreservesConsumerSelection(t *testing.T) {
	t.Parallel()

	requestedGeneration := uint64(0)
	refresh := func(generation uint64) tea.Cmd {
		requestedGeneration = generation
		return func() tea.Msg {
			return Refreshed(generation,
				dashboard.Snapshot{Messages: 99, ObservedAt: time.Now()},
				[]streams.Summary{{Name: "EVENTS"}},
				[]consumers.Summary{{Stream: "EVENTS", Name: "new"}, {Stream: "ORDERS", Name: "selected", Pending: 7}},
			)
		}
	}
	model := dashboardModel(t, 120, 30).WithRefresh(refresh)
	model.consumers = []consumers.Summary{{Stream: "ORDERS", Name: "first"}, {Stream: "ORDERS", Name: "selected"}}
	model.consumerCursor = 1
	model.refreshGeneration = 4

	updated, command := model.Update(refreshTickMsg{generation: 4})
	refreshing := updated.(Model)
	if command == nil || !refreshing.refreshing || requestedGeneration != 4 {
		t.Fatalf("refresh did not start: command=%v refreshing=%v generation=%d", command != nil, refreshing.refreshing, requestedGeneration)
	}
	updated, _ = refreshing.Update(command())
	refreshed := updated.(Model)
	if refreshed.snapshot.Messages != 99 || refreshed.refreshing || refreshed.refreshErr != nil {
		t.Fatalf("refresh result not applied: %#v", refreshed.snapshot)
	}
	if selected := refreshed.consumers[refreshed.consumerCursor]; selected.Name != "selected" {
		t.Fatalf("consumer selection moved to %q", selected.Name)
	}
}

func TestRefreshFailureKeepsLastGoodDataAndShowsStaleState(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 24)
	model.refreshing = true
	model.refreshGeneration = 8
	wantMessages := model.snapshot.Messages

	updated, _ := model.Update(RefreshFailed(8, errors.New("nats timeout")))
	stale := updated.(Model)
	if stale.snapshot.Messages != wantMessages || stale.refreshErr == nil || stale.refreshing {
		t.Fatalf("failure replaced data or state is wrong")
	}
	if view := stale.header(96); !strings.Contains(view, "Stale") || !strings.Contains(view, "retrying") {
		t.Fatalf("header does not show stale retry state: %s", view)
	}
}

func TestRefreshResponseFromPreviousConnectionIsIgnored(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 24)
	model.refreshGeneration = 10
	want := model.snapshot.Messages

	updated, _ := model.Update(Refreshed(9, dashboard.Snapshot{Messages: 1}, nil, nil))
	if got := updated.(Model).snapshot.Messages; got != want {
		t.Fatalf("stale refresh changed messages to %d, want %d", got, want)
	}
}

func TestRefreshTickDoesNotStartOverlappingRequest(t *testing.T) {
	t.Parallel()

	calls := 0
	model := dashboardModel(t, 100, 24).WithRefresh(func(uint64) tea.Cmd {
		calls++
		return func() tea.Msg { return nil }
	})
	model.refreshGeneration = 3
	model.refreshing = true

	updated, command := model.Update(refreshTickMsg{generation: 3})
	if calls != 0 || command != nil || !updated.(Model).refreshing {
		t.Fatalf("overlapping refresh started: calls=%d command=%v", calls, command != nil)
	}
}

func TestSuccessfulRetryClearsStaleState(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 24)
	model.refreshGeneration = 2
	model.refreshErr = errors.New("temporary timeout")
	updated, _ := model.Update(Refreshed(2, dashboard.Snapshot{ObservedAt: time.Now()}, nil, nil))
	recovered := updated.(Model)
	if recovered.refreshErr != nil || !strings.Contains(recovered.header(96), "Connected") {
		t.Fatalf("successful retry did not recover connection state")
	}
}

func TestDeleteConsumerRequiresExplicitConfirmation(t *testing.T) {
	t.Parallel()

	calls := 0
	action := func(ConsumerActionInput) tea.Cmd {
		calls++
		return func() tea.Msg { return nil }
	}
	model := dashboardModel(t, 100, 24).WithConsumerActions(action)
	model.activeWorkspace = 2
	model.consumers = []consumers.Summary{{Stream: "ORDERS", Name: "worker"}}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	confirming := updated.(Model)
	if command != nil || calls != 0 || confirming.pendingConsumerAction != consumers.ActionDelete {
		t.Fatalf("delete executed without confirmation")
	}
	view := confirming.View()
	if !strings.Contains(view, "Delete consumer") || !strings.Contains(view, "ORDERS / worker") {
		t.Fatalf("confirmation does not identify target:\n%s", view)
	}

	updated, command = confirming.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	executing := updated.(Model)
	if command == nil || calls != 1 || !executing.consumerActionRunning {
		t.Fatalf("confirmed delete did not execute")
	}
}

func TestResetConfirmationCanBeCancelled(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 24).WithConsumerActions(func(ConsumerActionInput) tea.Cmd {
		t.Fatal("cancelled action was executed")
		return nil
	})
	model.activeWorkspace = 2
	model.consumers = []consumers.Summary{{Stream: "ORDERS", Name: "worker"}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).pendingConsumerAction != 0 {
		t.Fatal("escape did not cancel reset confirmation")
	}
}

func TestPImmediatelyTogglesPauseAndResume(t *testing.T) {
	t.Parallel()

	actions := make([]consumers.Action, 0, 2)
	operate := func(input ConsumerActionInput) tea.Cmd {
		actions = append(actions, input.Action)
		return func() tea.Msg { return ConsumerActionSucceeded(input.Generation, input) }
	}
	model := dashboardModel(t, 100, 24).WithConsumerActions(operate)
	model.activeWorkspace = 2
	model.consumers = []consumers.Summary{{Stream: "ORDERS", Name: "worker", Status: consumers.StatusActive}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if len(actions) != 1 || actions[0] != consumers.ActionPause || !updated.(Model).consumerActionRunning {
		t.Fatalf("active consumer did not start pause: %#v", actions)
	}

	model.consumers[0].Status = consumers.StatusPaused
	model.consumerActionRunning = false
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if len(actions) != 2 || actions[1] != consumers.ActionResume || !updated.(Model).consumerActionRunning {
		t.Fatalf("paused consumer did not start resume: %#v", actions)
	}
}

func TestEnterOpensStreamDetail(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 24)
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ORDERS", Subjects: []string{"orders.>"}, Storage: streams.StorageFile, Retention: streams.RetentionLimits, Replicas: 3, MaxMessages: 1000}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	detail := updated.(Model)
	if !detail.showStreamDetail {
		t.Fatal("stream detail did not open")
	}
	for _, text := range []string{"ORDERS", "orders.>", "file", "limits", "3 replicas", "1.0K"} {
		if !strings.Contains(detail.View(), text) {
			t.Errorf("stream detail missing %q:\n%s", text, detail.View())
		}
	}
}

func TestNOpensCreateStreamFormAndEscapeCancels(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 28)
	model.activeWorkspace = 1
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	form := updated.(Model)
	if form.streamForm == nil || form.streamForm.action != streams.ActionCreate {
		t.Fatal("create stream form did not open")
	}
	if !strings.Contains(form.View(), "Create stream") {
		t.Fatalf("create form title missing:\n%s", form.View())
	}
	updated, _ = form.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).streamForm != nil {
		t.Fatal("escape did not close stream form")
	}
}

func TestEditStreamFormIsPrefilledAndSubmitsValidatedDefinition(t *testing.T) {
	t.Parallel()

	var submitted StreamActionInput
	operate := func(input StreamActionInput) tea.Cmd {
		submitted = input
		return func() tea.Msg { return StreamActionSucceeded(input.Generation, input) }
	}
	model := dashboardModel(t, 100, 28).WithStreamActions(operate)
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ORDERS", Description: "payments", Subjects: []string{"orders.>"}, Storage: streams.StorageFile, Retention: streams.RetentionLimits, Replicas: 1, MaxMessages: -1, MaxBytes: -1}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	form := updated.(Model)
	if form.streamForm == nil || form.streamForm.name != "ORDERS" || form.streamForm.subjects != "orders.>" {
		t.Fatalf("edit form was not prefilled: %#v", form.streamForm)
	}
	updated, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || submitted.Action != streams.ActionUpdate || submitted.Definition.Name != "ORDERS" {
		t.Fatalf("edit was not submitted: %#v", submitted)
	}
}

func TestDeleteStreamRequiresConfirmation(t *testing.T) {
	t.Parallel()

	calls := 0
	model := dashboardModel(t, 100, 24).WithStreamActions(func(StreamActionInput) tea.Cmd {
		calls++
		return func() tea.Msg { return nil }
	})
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ORDERS", Messages: 42}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	confirming := updated.(Model)
	if !confirming.confirmStreamDelete || calls != 0 {
		t.Fatal("stream delete did not require confirmation")
	}
	if view := confirming.View(); !strings.Contains(view, "42 stored messages") || !strings.Contains(view, "cannot be") || !strings.Contains(view, "undone") {
		t.Fatalf("delete warning lacks impact:\n%s", view)
	}
}

func TestCompactWorkspaceTabsRemainVisibleAtEightyColumns(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 80, 24)
	tabs := model.workspaceTabs(76)

	for _, label := range []string{"1 Dash", "2 Str", "3 Con", "4 Msg", "5 KV", "6 Obj", "7 Sub"} {
		if !strings.Contains(tabs, label) {
			t.Errorf("tabs do not contain %q: %s", label, tabs)
		}
	}
	if width := lipgloss.Width(tabs); width > 76 {
		t.Errorf("tabs width = %d, want <= 76", width)
	}
}

func TestDashboardSnapshotRendersMidnightDashboard(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 32)
	view := model.View()

	for _, text := range []string{"production", "2.4M", "14.8 GB", "12", "31"} {
		if !strings.Contains(view, text) {
			t.Errorf("View() does not contain %q:\n%s", text, view)
		}
	}
}

func TestAllScreensFillAvailableViewport(t *testing.T) {
	t.Parallel()

	for _, viewport := range []struct {
		width  int
		height int
	}{
		{width: 160, height: 40},
		{width: 80, height: 24},
		{width: 48, height: 30},
	} {
		model := newTestModel(nil)
		model.width = viewport.width
		model.height = viewport.height

		for _, screen := range []screen{screenWelcome, screenConnection, screenConnecting, screenWorkspace} {
			model.screen = screen
			view := model.View()
			if width := lipgloss.Width(view); width != viewport.width {
				t.Errorf("%v at %dx%d rendered width = %d", screen, viewport.width, viewport.height, width)
			}
			if height := lipgloss.Height(view); height != viewport.height {
				t.Errorf("%v at %dx%d rendered height = %d", screen, viewport.width, viewport.height, height)
			}
		}
	}
}

func TestDashboardGridRespondsToViewportWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		width int
		check func([]int) bool
	}{
		{
			name:  "wide has four columns",
			width: 160,
			check: func(lines []int) bool {
				return lines[0] == lines[1] && lines[1] == lines[2] && lines[2] == lines[3]
			},
		},
		{
			name:  "medium has two columns",
			width: 80,
			check: func(lines []int) bool {
				return lines[0] == lines[1] && lines[2] == lines[3] && lines[2] > lines[0]
			},
		},
		{
			name:  "narrow has one column",
			width: 48,
			check: func(lines []int) bool {
				return lines[0] < lines[1] && lines[1] < lines[2] && lines[2] < lines[3]
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := dashboardModel(t, tt.width, 36).View()
			lines := cardLabelLines(view)
			if !tt.check(lines) {
				t.Errorf("card label lines = %v", lines)
			}
		})
	}
}

func TestDashboardGridUsesAvailableWidth(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 160, 40)
	const contentWidth = 156
	grid := model.dashboardView(contentWidth)

	if width := lipgloss.Width(grid); width < contentWidth-1 {
		t.Fatalf("dashboard width = %d, want at least %d", width, contentWidth-1)
	}
}

func TestDashboardMetricsUseOpenPanelsWithoutCardBoxes(t *testing.T) {
	t.Parallel()

	grid := dashboardModel(t, 160, 40).dashboardView(156)
	for _, border := range []string{"╭", "╮", "╰", "╯"} {
		if strings.Contains(grid, border) {
			t.Errorf("dashboard still contains closed card border %q", border)
		}
	}
	if count := strings.Count(grid, "┃"); count < 4 {
		t.Errorf("dashboard accent bars = %d, want at least 4", count)
	}
}

func TestScreensDoNotEmitANSIBackgroundColors(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	model := newTestModel(nil)
	model.serverInput.Focus()
	model.serverInput.Cursor.Blink = false
	views := map[string]string{
		"welcome":    model.welcomeView(160, 40),
		"connection": model.connectionView(160, 40),
		"connecting": model.connectingView(160, 40),
		"workspace":  dashboardModel(t, 160, 40).workspaceView(160, 40),
		"help":       model.helpView(160, 40),
	}
	subjectModel := dashboardModel(t, 160, 40)
	subjectModel.activeWorkspace = 6
	subjectModel.streams = []streams.Summary{{Name: "ORDERS", Subjects: []string{"orders.>"}}}
	subjectModel.rebuildSubjects()
	views["subjects"] = subjectModel.workspaceView(160, 40)
	keyValueModel := dashboardModel(t, 160, 40)
	keyValueModel.activeWorkspace = 4
	keyValueModel.keyValueBuckets = []kvstore.BucketSummary{{Name: "SESSIONS", Storage: kvstore.StorageFile, Replicas: 1}}
	views["key-value"] = keyValueModel.workspaceView(160, 40)
	formModel := dashboardModel(t, 160, 40)
	keyValueForm := newKeyValueForm()
	formModel.keyValueForm = &keyValueForm
	views["key-value form"] = formModel.workspaceView(160, 40)
	deleteKVModel := dashboardModel(t, 160, 40)
	deleteKVModel.keyValueBuckets = []kvstore.BucketSummary{{Name: "SESSIONS", Values: 42}}
	deleteKVModel.confirmKeyValueDelete = true
	views["key-value confirmation"] = deleteKVModel.workspaceView(160, 40)
	keyBrowserModel := dashboardModel(t, 160, 40)
	keyBrowserModel.activeWorkspace = 4
	keyBrowserModel.browsingKeyValue = true
	keyBrowserModel.keyValueBucket = "SESSIONS"
	keyBrowserModel.keyValueKeys = []string{"session.alpha", "session.beta"}
	views["key-value keys"] = keyBrowserModel.workspaceView(160, 40)
	entryFormModel := dashboardModel(t, 160, 40)
	entryForm := newCreateKeyValueEntryForm("SESSIONS", entryFormModel.theme)
	entryFormModel.keyValueEntryForm = &entryForm
	views["key-value entry form"] = entryFormModel.workspaceView(160, 40)
	entryRemovalModel := dashboardModel(t, 160, 40)
	removalEntry := kvstore.Entry{Bucket: "SESSIONS", Key: "session.alpha", Revision: 7}
	entryRemovalModel.keyValueEntry = &removalEntry
	entryRemovalModel.pendingKeyValueEntryRemoval = kvstore.RemovePurge
	views["key-value entry removal"] = entryRemovalModel.workspaceView(160, 40)
	historyModel := dashboardModel(t, 160, 40)
	historyEntry := kvstore.Entry{Bucket: "SESSIONS", Key: "session.alpha", Revision: 8}
	historyModel.activeWorkspace = 4
	historyModel.keyValueEntry = &historyEntry
	historyModel.showKeyValueEntry = true
	historyModel.showKeyValueHistory = true
	historyModel.keyValueHistory = []kvstore.Entry{{Bucket: "SESSIONS", Key: "session.alpha", Revision: 7, Operation: kvstore.OperationPut}, historyEntry}
	historyModel.keyValueHistoryCursor = 1
	views["key-value history"] = historyModel.workspaceView(160, 40)
	historyModel.showKeyValueHistoryEntry = true
	views["key-value historical detail"] = historyModel.workspaceView(160, 40)
	model.consumers = []consumers.Summary{{Stream: "ORDERS", Name: "worker"}}
	model.pendingConsumerAction = consumers.ActionDelete
	views["consumer confirmation"] = model.consumerConfirmationView(160, 40)
	form := newCreateStreamForm()
	model.streamForm = &form
	views["stream form"] = model.streamFormView(160, 40)
	model.streamForm = nil
	model.streams = []streams.Summary{{Name: "ORDERS", Messages: 42}}
	model.confirmStreamDelete = true
	views["stream confirmation"] = model.streamDeleteConfirmationView(160, 40)
	for name, view := range views {
		if strings.Contains(view, "\x1b[48;") {
			t.Errorf("%s emits an ANSI background color", name)
		}
		if regexp.MustCompile("\\x1b\\[(?:[0-9]+;)*7(?:;[0-9]+)*m").MatchString(view) {
			t.Errorf("%s emits ANSI reverse video", name)
		}
	}
}

func TestQuestionMarkTogglesHelpInWorkspace(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 80, 24)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	if !updated.(Model).showHelp {
		t.Fatal("showHelp = false, want true")
	}
}

func TestKeyValueDetailCopiesValueAndAppliesLiveWatchEvent(t *testing.T) {
	t.Parallel()

	events := make(chan kvstore.Entry, 1)
	started := false
	model := dashboardModel(t, 100, 30)
	model.activeWorkspace = 4
	entry := kvstore.Entry{Bucket: "CONFIG", Key: "feature.api", Revision: 7, Value: []byte("old")}
	model.keyValueEntry = &entry
	model.showKeyValueEntry = true
	model = model.WithCopyValue(func(value string) error {
		if value != "old" {
			t.Fatalf("copied value = %q", value)
		}
		return nil
	}).WithKeyValueWatch(func(bucket, key string) tea.Cmd {
		started = bucket == "CONFIG" && key == "feature.api"
		return func() tea.Msg { return KeyValueWatchStarted(events) }
	}, func() {})

	copiedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	if !strings.Contains(copiedModel.(Model).keyValueCopyNotice, "copied") {
		t.Fatal("copy notice was not set")
	}
	startingModel, command := copiedModel.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'W'}})
	if command == nil || !started || !startingModel.(Model).keyValueWatchStarting {
		t.Fatalf("watch did not start: command=%v started=%v", command != nil, started)
	}
	watchingModel, wait := startingModel.(Model).Update(KeyValueWatchStarted(events))
	events <- kvstore.Entry{Bucket: "CONFIG", Key: "feature.api", Revision: 8, Value: []byte("new"), Operation: kvstore.OperationPut}
	eventMsg := wait()
	updatedModel, _ := watchingModel.(Model).Update(eventMsg)
	if updatedModel.(Model).keyValueEntry.Revision != 8 || string(updatedModel.(Model).keyValueEntry.Value) != "new" {
		t.Fatalf("live entry = %#v", updatedModel.(Model).keyValueEntry)
	}
}

func TestObjectStoreWorkspaceRendersCatalogAndDetail(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 120, 30)
	model.activeWorkspace = 5
	model.objectBuckets = []objectstore.BucketSummary{{Name: "ARTIFACTS", Description: "release binaries", Size: 4096, Storage: "FILE", Replicas: 3, Compressed: true, Sources: []string{"REGION_A", "REGION_B"}}}
	view := model.View()
	for _, want := range []string{"OBJECT BUCKETS", "ARTIFACTS", "release binaries", "R3", "4.1 KB", "REGION_A, REGION_B"} {
		if !strings.Contains(view, want) {
			t.Fatalf("object store view missing %q:\n%s", want, view)
		}
	}
}

func TestObjectStoreBrowsesSearchesAndShowsMetadata(t *testing.T) {
	t.Parallel()

	loadedBucket := ""
	model := dashboardModel(t, 132, 34)
	model.activeWorkspace = 5
	model.objectBuckets = []objectstore.BucketSummary{{Name: "ARTIFACTS", Storage: "FILE"}}
	model = model.WithObjectCatalog(func(bucket string) tea.Cmd {
		loadedBucket = bucket
		return nil
	})
	loading, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil || loadedBucket != "ARTIFACTS" || !loading.(Model).loadingObjects {
		t.Fatalf("object load command=%v bucket=%q loading=%v", command != nil, loadedBucket, loading.(Model).loadingObjects)
	}
	objects := []objectstore.ObjectSummary{
		{Name: "debug.log", Size: 80},
		{Name: "release.tar.gz", Description: "production release", Size: 4096, Chunks: 2, Digest: "SHA-256=abc", Metadata: map[string]string{"environment": "production"}, Headers: map[string][]string{"Content-Type": {"application/gzip"}}},
	}
	browsingValue, _ := loading.(Model).Update(ObjectsLoaded("ARTIFACTS", objects))
	browsing := browsingValue.(Model)
	if !browsing.browsingObjects || !strings.Contains(browsing.View(), "debug.log") || !strings.Contains(browsing.View(), "release.tar.gz") {
		t.Fatalf("object browser missing catalog:\n%s", browsing.View())
	}
	searchingValue, _ := browsing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	searchingValue, _ = searchingValue.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("production")})
	searched := searchingValue.(Model)
	if len(searched.displayedObjects()) != 1 || searched.displayedObjects()[0].Name != "release.tar.gz" {
		t.Fatalf("filtered objects = %#v", searched.displayedObjects())
	}
	searched.objectSearchMode = false
	detailValue, _ := searched.Update(tea.KeyMsg{Type: tea.KeyEnter})
	detail := detailValue.(Model).View()
	for _, want := range []string{"OBJECT DETAIL", "SHA-256=abc", "environment", "production", "Content-Type", "application/gzip", "2"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("object detail missing %q:\n%s", want, detail)
		}
	}
}

func TestObjectStoreMirrorAllowsReadsAndRejectsObjectMutations(t *testing.T) {
	t.Parallel()
	base := dashboardModel(t, 110, 30)
	base.activeWorkspace = 5
	base.browsingObjects = true
	base.objectBucket = "MIRROR"
	base.objectBuckets = []objectstore.BucketSummary{{Name: "MIRROR", Mirror: "PRIMARY"}}
	base.objects = []objectstore.ObjectSummary{{Bucket: "MIRROR", Name: "release.tar"}}
	for _, key := range []rune{'u', 'e', 'd', 'L'} {
		model := base
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		result := updated.(Model)
		if result.objectTransferForm != nil || result.objectEditForm != nil || result.objectLinkForm != nil || result.confirmObjectDelete {
			t.Fatalf("key %q opened a mutation flow for mirror", key)
		}
		if result.objectOperationErr == nil || !strings.Contains(result.objectOperationErr.Error(), "read-only") {
			t.Fatalf("key %q error = %v", key, result.objectOperationErr)
		}
	}
	if !strings.Contains(base.View(), "mirror is read-only") || !strings.Contains(base.View(), "D download") {
		t.Fatalf("mirror guidance missing:\n%s", base.View())
	}
}

func TestObjectUploadFormReportsProgressAndRefreshesCatalog(t *testing.T) {
	t.Parallel()

	events := make(chan objectstore.TransferEvent, 2)
	var uploaded objectstore.UploadFile
	canceled := false
	model := dashboardModel(t, 110, 32)
	model.activeWorkspace = 5
	model.browsingObjects = true
	model.objectBucket = "ARTIFACTS"
	model = model.WithObjectTransfers(func(command objectstore.UploadFile) tea.Cmd {
		uploaded = command
		return func() tea.Msg { return ObjectTransferStarted(events) }
	}, nil, func() { canceled = true })

	formValue, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	formModel := formValue.(Model)
	if formModel.objectTransferForm == nil || !strings.Contains(formModel.View(), "Upload object") {
		t.Fatalf("upload form not opened:\n%s", formModel.View())
	}
	formModel.objectTransferForm.path = "/tmp/release.tar"
	formModel.objectTransferForm.objectName = "release.tar"
	startingValue, startCommand := formModel.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	starting := startingValue.(Model)
	if startCommand == nil || uploaded.Name != "release.tar" || !starting.objectTransferStarting {
		t.Fatalf("upload did not start: command=%v upload=%#v", startCommand != nil, uploaded)
	}
	startedValue, wait := starting.Update(startCommand())
	started := startedValue.(Model)
	events <- objectstore.TransferEvent{Progress: objectstore.Progress{Transferred: 5, Total: 10}}
	progressValue, waitNext := started.Update(wait())
	progressModel := progressValue.(Model)
	if !strings.Contains(progressModel.View(), "50.0%") {
		t.Fatalf("progress view missing percentage:\n%s", progressModel.View())
	}
	events <- objectstore.TransferEvent{Done: true, Object: objectstore.ObjectSummary{Bucket: "ARTIFACTS", Name: "release.tar", Size: 10}}
	completedValue, _ := progressModel.Update(waitNext())
	completed := completedValue.(Model)
	if completed.objectTransferActive || len(completed.objects) != 1 || completed.objects[0].Name != "release.tar" || !strings.Contains(completed.View(), "uploaded release.tar") {
		t.Fatalf("completed state objects=%#v notice=%q", completed.objects, completed.objectTransferNotice)
	}

	active := started
	active.objectTransferActive = true
	activeValue, _ := active.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !canceled || !activeValue.(Model).objectTransferCanceling {
		t.Fatal("esc did not request transfer cancellation")
	}
}

func TestObjectDownloadFormDefaultsToSafeDestination(t *testing.T) {
	t.Parallel()

	model := dashboardModel(t, 100, 28)
	model.activeWorkspace = 5
	model.browsingObjects = true
	model.objectBucket = "ARTIFACTS"
	model.objects = []objectstore.ObjectSummary{{Name: "releases/v1.tar"}}
	value, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	form := value.(Model).objectTransferForm
	if form == nil || form.mode != objectDownload || form.path != "v1.tar" || form.force {
		t.Fatalf("download form = %#v", form)
	}
}

func TestObjectEditAndDeleteUseCenteredSafeFlows(t *testing.T) {
	t.Parallel()
	var edited objectstore.EditObject
	deletedBucket, deletedName := "", ""
	model := dashboardModel(t, 110, 30)
	model.activeWorkspace = 5
	model.browsingObjects = true
	model.objectBucket = "ARTIFACTS"
	model.objects = []objectstore.ObjectSummary{{Bucket: "ARTIFACTS", Name: "release.tar", Description: "old", Size: 42, Chunks: 2, Metadata: map[string]string{"env": "prod"}}}
	model.rebuildObjectSearch()
	model = model.WithObjectOperations(func(patch objectstore.EditObject) tea.Cmd {
		edited = patch
		return func() tea.Msg {
			return ObjectEditSucceeded(objectstore.ObjectSummary{Bucket: patch.Bucket, Name: "stable.tar", Description: "stable", Size: 42})
		}
	}, func(bucket, name string) tea.Cmd {
		deletedBucket, deletedName = bucket, name
		return func() tea.Msg { return ObjectDeleteSucceeded(bucket, name) }
	})

	formValue, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	form := formValue.(Model)
	if form.objectEditForm == nil || !strings.Contains(form.View(), "Edit object metadata") || !strings.Contains(form.View(), "env=prod") {
		t.Fatalf("edit form not populated:\n%s", form.View())
	}
	form.objectEditForm.name = "stable.tar"
	form.objectEditForm.description = "stable"
	startedValue, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || edited.NewName == nil || *edited.NewName != "stable.tar" {
		t.Fatalf("edit command = %#v", edited)
	}
	editedValue, _ := startedValue.(Model).Update(command())
	editedModel := editedValue.(Model)
	if len(editedModel.objects) != 1 || editedModel.objects[0].Name != "stable.tar" {
		t.Fatalf("objects after rename = %#v", editedModel.objects)
	}

	confirmValue, _ := editedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	confirm := confirmValue.(Model)
	if deletedName != "" || !strings.Contains(confirm.View(), "Delete object permanently?") || !strings.Contains(confirm.View(), "cannot be undone") {
		t.Fatalf("unsafe confirmation state delete=%q view=%s", deletedName, confirm.View())
	}
	deletingValue, deleteCommand := confirm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if deleteCommand == nil || deletedBucket != "ARTIFACTS" || deletedName != "stable.tar" {
		t.Fatalf("delete=%s/%s", deletedBucket, deletedName)
	}
	deletedValue, _ := deletingValue.(Model).Update(deleteCommand())
	if len(deletedValue.(Model).objects) != 0 {
		t.Fatalf("deleted objects = %#v", deletedValue.(Model).objects)
	}
}

func TestObjectWatchAppliesUpdatesByStableIdentityAndSupportsPause(t *testing.T) {
	t.Parallel()
	events := make(chan objectstore.ObjectEvent, 3)
	stopped := false
	reloaded := ""
	model := dashboardModel(t, 110, 30)
	model.activeWorkspace = 5
	model.browsingObjects = true
	model.objectBucket = "ARTIFACTS"
	model.objects = []objectstore.ObjectSummary{{Bucket: "ARTIFACTS", ID: "id-1", Name: "release.tar", Description: "old"}}
	model.rebuildObjectSearch()
	model = model.WithObjectWatch(func(input ObjectWatchInput) tea.Cmd {
		if input.Bucket != "ARTIFACTS" {
			t.Fatalf("watch bucket = %q", input.Bucket)
		}
		return func() tea.Msg { return ObjectWatchStarted(input.Generation, events) }
	}, func() { stopped = true }).WithObjectCatalog(func(bucket string) tea.Cmd {
		reloaded = bucket
		return nil
	})

	startingValue, start := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'W'}})
	if start == nil || !startingValue.(Model).objectWatchStarting {
		t.Fatal("W did not start object watch")
	}
	startedValue, wait := startingValue.(Model).Update(start())
	started := startedValue.(Model)
	if !started.objectWatching || wait == nil || !strings.Contains(started.View(), "LIVE") {
		t.Fatalf("watch did not become live: %s", started.View())
	}

	events <- objectstore.ObjectEvent{Kind: objectstore.ObjectUpserted, Object: objectstore.ObjectSummary{Bucket: "ARTIFACTS", ID: "id-1", Name: "stable.tar", Description: "renamed"}}
	renamedValue, next := started.Update(wait())
	renamed := renamedValue.(Model)
	if len(renamed.objects) != 1 || renamed.objects[0].Name != "stable.tar" || renamed.objectWatchReceived != 1 {
		t.Fatalf("renamed objects = %#v received=%d", renamed.objects, renamed.objectWatchReceived)
	}

	pausedValue, _ := renamed.Update(tea.KeyMsg{Type: tea.KeySpace})
	paused := pausedValue.(Model)
	if !paused.objectWatchPaused || !strings.Contains(paused.View(), "PAUSED") {
		t.Fatalf("watch did not pause: %s", paused.View())
	}
	events <- objectstore.ObjectEvent{Kind: objectstore.ObjectDeleted, Object: objectstore.ObjectSummary{Bucket: "ARTIFACTS", ID: "id-1", Name: "stable.tar", Deleted: true}}
	pausedEventValue, next := paused.Update(next())
	pausedEvent := pausedEventValue.(Model)
	if len(pausedEvent.objects) != 1 || pausedEvent.objectWatchReceived != 2 {
		t.Fatalf("paused watch mutated catalog: %#v", pausedEvent.objects)
	}
	resumedValue, _ := pausedEvent.Update(tea.KeyMsg{Type: tea.KeySpace})
	if resumedValue.(Model).objectWatchPaused || reloaded != "ARTIFACTS" {
		t.Fatalf("resume paused=%v reloaded=%q", resumedValue.(Model).objectWatchPaused, reloaded)
	}

	stoppedValue, _ := resumedValue.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'W'}})
	if !stopped || stoppedValue.(Model).objectWatching {
		t.Fatal("W did not stop object watch")
	}
	close(events)
	_ = next
}

func TestObjectWatchIgnoresEventsFromStoppedGeneration(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 90, 24)
	model.objectWatchGeneration = 2
	model.objectWatching = true
	model.objects = []objectstore.ObjectSummary{{ID: "current", Name: "current.tar"}}
	value, _ := model.Update(objectWatchEventMsg{
		generation: 1,
		event:      objectstore.ObjectEvent{Kind: objectstore.ObjectDeleted, Object: objectstore.ObjectSummary{ID: "current", Name: "current.tar"}},
	})
	result := value.(Model)
	if !result.objectWatching || len(result.objects) != 1 || result.objectWatchReceived != 0 {
		t.Fatalf("stale event changed current watch: %#v", result.objects)
	}
}

func TestObjectLinkFormCreatesAndDisplaysCrossBucketLink(t *testing.T) {
	t.Parallel()
	var created objectstore.LinkDefinition
	model := dashboardModel(t, 110, 30)
	model.activeWorkspace = 5
	model.browsingObjects = true
	model.objectBucket = "ROOT"
	model = model.WithObjectLinks(func(definition objectstore.LinkDefinition) tea.Cmd {
		created = definition
		return func() tea.Msg {
			return ObjectLinkSucceeded(objectstore.ObjectSummary{
				Bucket: definition.Bucket, ID: "link-1", Name: definition.Name,
				Link: &objectstore.LinkTarget{Bucket: definition.TargetBucket, Name: definition.TargetObject},
			})
		}
	})
	formValue, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	form := formValue.(Model)
	if form.objectLinkForm == nil || !strings.Contains(form.View(), "Create Object Store link") || !strings.Contains(form.View(), "Leave target object empty") {
		t.Fatalf("link form missing guidance:\n%s", form.View())
	}
	form.objectLinkForm.name = "latest"
	form.objectLinkForm.targetBucket = "RELEASES"
	form.objectLinkForm.targetObject = "v1.tar"
	startedValue, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || created != (objectstore.LinkDefinition{Bucket: "ROOT", Name: "latest", TargetBucket: "RELEASES", TargetObject: "v1.tar"}) {
		t.Fatalf("created = %#v", created)
	}
	completedValue, _ := startedValue.(Model).Update(command())
	completed := completedValue.(Model)
	if len(completed.objects) != 1 || completed.objects[0].Link == nil || !strings.Contains(completed.View(), "RELEASES/v1.tar") {
		t.Fatalf("linked objects=%#v view=%s", completed.objects, completed.View())
	}
}

func TestObjectBucketCreateEditAndDeleteUseGuidedForms(t *testing.T) {
	t.Parallel()
	var created ObjectBucketCreateInput
	var updated ObjectBucketUpdateInput
	var deleted ObjectBucketDeleteInput
	model := dashboardModel(t, 110, 32)
	model.activeWorkspace = 5
	model = model.WithObjectBucketCreate(func(input ObjectBucketCreateInput) tea.Cmd {
		created = input
		return func() tea.Msg { return ObjectBucketCreateSucceeded(input) }
	}).WithObjectBucketUpdate(func(input ObjectBucketUpdateInput) tea.Cmd {
		updated = input
		return func() tea.Msg { return ObjectBucketUpdateSucceeded(input) }
	}).WithObjectBucketDelete(func(input ObjectBucketDeleteInput) tea.Cmd {
		deleted = input
		return func() tea.Msg { return ObjectBucketDeleteSucceeded(input) }
	})

	formValue, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	form := formValue.(Model)
	if form.objectBucketForm == nil || !strings.Contains(form.View(), "Create Object Store bucket") || !strings.Contains(form.View(), "Defaults are safe") {
		t.Fatalf("create form missing:\n%s", form.View())
	}
	form.objectBucketForm.name = "ARTIFACTS"
	form.objectBucketForm.description = "release files"
	form.objectBucketForm.metadata = "owner=platform"
	form.objectBucketForm.sources = "REGION_A, REGION_B"
	startedValue, createCommand := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if createCommand == nil || created.Definition.Name != "ARTIFACTS" || created.Definition.Metadata["owner"] != "platform" || !reflect.DeepEqual(created.Definition.Sources, []string{"REGION_A", "REGION_B"}) {
		t.Fatalf("created = %#v", created)
	}
	createdValue, _ := startedValue.(Model).Update(createCommand())
	createdModel := createdValue.(Model)
	if len(createdModel.objectBuckets) != 1 || createdModel.objectBuckets[0].Name != "ARTIFACTS" {
		t.Fatalf("buckets = %#v", createdModel.objectBuckets)
	}

	editValue, _ := createdModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	edit := editValue.(Model)
	if edit.objectBucketForm == nil || !strings.Contains(edit.View(), "Name (immutable)") || !strings.Contains(edit.View(), "Storage (immutable)") {
		t.Fatalf("edit form missing immutable guidance:\n%s", edit.View())
	}
	edit.objectBucketForm.description = "stable releases"
	editStarted, editCommand := edit.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if editCommand == nil || updated.Name != "ARTIFACTS" || updated.Changes.Description == nil || *updated.Changes.Description != "stable releases" {
		t.Fatalf("updated = %#v", updated)
	}
	updatedValue, _ := editStarted.(Model).Update(editCommand())
	updatedModel := updatedValue.(Model)
	if updatedModel.objectBuckets[0].Description != "stable releases" {
		t.Fatalf("updated bucket = %#v", updatedModel.objectBuckets[0])
	}

	confirmValue, _ := updatedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	confirm := confirmValue.(Model)
	if deleted.Name != "" || !strings.Contains(confirm.View(), "Delete Object Store bucket?") || !strings.Contains(confirm.View(), "cannot be undone") {
		t.Fatalf("delete confirmation missing:\n%s", confirm.View())
	}
	deletingValue, deleteCommand := confirm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if deleteCommand == nil || deleted.Name != "ARTIFACTS" {
		t.Fatalf("deleted = %#v", deleted)
	}
	deletedValue, _ := deletingValue.(Model).Update(deleteCommand())
	if len(deletedValue.(Model).objectBuckets) != 0 {
		t.Fatalf("buckets after delete = %#v", deletedValue.(Model).objectBuckets)
	}
}

func TestRequestReplyWorkspacePublishesRequestsFormatsJSONAndShowsMultipleReplies(t *testing.T) {
	t.Parallel()
	var published domainrequestreply.Message
	var requested domainrequestreply.Request
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	model := dashboardModel(t, 132, 36)
	model.activeWorkspace = 7
	model = model.WithRequestReply(RequestReplyServices{
		Publish: func(message domainrequestreply.Message) tea.Cmd {
			published = message
			return func() tea.Msg { return CorePublishSucceeded(message) }
		},
		Request: func(request domainrequestreply.Request) tea.Cmd {
			requested = request
			replyA, _ := domainrequestreply.NewReply([]byte(`{"node":"a"}`), nil, now, time.Millisecond)
			replyB, _ := domainrequestreply.NewReply([]byte(`{"node":"b"}`), nil, now, 2*time.Millisecond)
			exchange, _ := domainrequestreply.NewExchange("req-1", now, request, []domainrequestreply.Reply{replyA, replyB})
			return func() tea.Msg { return CoreRequestSucceeded(exchange) }
		},
		LoadPayload: func(path string) tea.Cmd {
			return func() tea.Msg { return PayloadFileLoaded(path, []byte(`{"id":42}`)) }
		},
	})
	if view := model.View(); !strings.Contains(view, "Publish / Request Lab") || !strings.Contains(view, "Pretty JSON") {
		t.Fatalf("request workspace missing:\n%s", view)
	}
	model.requestReply.subject.SetValue("events.created")
	model.requestReply.file.SetValue("payload.json")
	loadingPayload, payloadCommand := model.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if payloadCommand == nil {
		t.Fatal("payload file command missing")
	}
	loadedPayload, _ := loadingPayload.(Model).Update(payloadCommand())
	model = loadedPayload.(Model)
	model.requestReply.headers.SetValue("Content-Type=application/json")
	formattedValue, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	formatted := formattedValue.(Model)
	if !strings.Contains(formatted.requestReply.payload.Value(), "\n") {
		t.Fatalf("payload was not formatted: %q", formatted.requestReply.payload.Value())
	}
	publishingValue, publishCommand := formatted.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if publishCommand == nil || published.Subject().String() != "events.created" {
		t.Fatalf("published=%#v", published)
	}
	publishedValue, _ := publishingValue.(Model).Update(publishCommand())
	if !strings.Contains(publishedValue.(Model).View(), "Published") || !strings.Contains(publishedValue.(Model).View(), "nats-tui publish events.created") {
		t.Fatalf("publish notice missing:\n%s", publishedValue.(Model).View())
	}

	requestModeValue, _ := publishedValue.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	requestMode := requestModeValue.(Model)
	requestMode.requestReply.subject.SetValue("rpc.lookup")
	requestMode.requestReply.timeout.SetValue("3s")
	requestMode.requestReply.maxReplies.SetValue("2")
	requestingValue, requestCommand := requestMode.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if requestCommand == nil || requested.MaxReplies() != 2 {
		t.Fatalf("requested=%#v", requested)
	}
	resultValue, _ := requestingValue.(Model).Update(requestCommand())
	result := resultValue.(Model)
	view := result.View()
	for _, want := range []string{"2 replies", `"node": "a"`, "nats-tui request rpc.lookup", "req-1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("result missing %q:\n%s", want, view)
		}
	}
}

func TestRequestReplyHistoryReplayAndPersistentTemplateFlows(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC)
	message, _ := domainrequestreply.NewMessage("rpc.lookup", []byte(`{"id":42}`), nil)
	request, _ := domainrequestreply.NewRequest(message, 2*time.Second, 1)
	exchange, _ := domainrequestreply.NewExchange("history-1", now, request, nil)
	template, _ := domainrequestreply.NewTemplate("lookup", request)
	var replayed domainrequestreply.Request
	var saved domainrequestreply.Template
	var deleted string
	model := dashboardModel(t, 120, 32)
	model.activeWorkspace = 7
	model = model.WithRequestReply(RequestReplyServices{
		Request: func(request domainrequestreply.Request) tea.Cmd {
			replayed = request
			return func() tea.Msg { return CoreRequestSucceeded(exchange) }
		},
		LoadHistory: func() tea.Cmd {
			return func() tea.Msg { return RequestHistoryLoaded([]domainrequestreply.Exchange{exchange}) }
		},
		LoadTemplates: func() tea.Cmd {
			return func() tea.Msg { return RequestTemplatesLoaded([]domainrequestreply.Template{template}) }
		},
		SaveTemplate: func(template domainrequestreply.Template) tea.Cmd {
			saved = template
			return func() tea.Msg { return RequestTemplateSaved(template) }
		},
		DeleteTemplate: func(name string) tea.Cmd {
			deleted = name
			return func() tea.Msg { return RequestTemplateDeleted(name) }
		},
	})

	loadingHistory, historyCommand := model.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	if historyCommand == nil {
		t.Fatal("history command missing")
	}
	historyValue, _ := loadingHistory.(Model).Update(historyCommand())
	history := historyValue.(Model)
	if !strings.Contains(history.View(), "REQUEST HISTORY") || !strings.Contains(history.View(), "history-1") {
		t.Fatalf("history missing:\n%s", history.View())
	}
	_, replayCommand, _ := history.updateRequestReplyKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if replayCommand == nil || replayed.Message().Subject().String() != "rpc.lookup" {
		t.Fatalf("replayed=%#v", replayed)
	}

	loadingTemplates, templatesCommand, _ := history.updateRequestReplyKey(tea.KeyMsg{Type: tea.KeyCtrlO})
	templatesValue, _ := loadingTemplates.(Model).Update(templatesCommand())
	templates := templatesValue.(Model)
	if !strings.Contains(templates.View(), "REQUEST TEMPLATES") || !strings.Contains(templates.View(), "lookup") {
		t.Fatalf("templates missing:\n%s", templates.View())
	}
	deletingValue, deleteCommand, _ := templates.updateRequestReplyKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if deleteCommand == nil || deleted != "lookup" {
		t.Fatalf("deleted=%q", deleted)
	}
	_, _ = deletingValue.(Model).Update(deleteCommand())

	composeValue, _, _ := templates.updateRequestReplyKey(tea.KeyMsg{Type: tea.KeyEnter})
	compose := composeValue.(Model)
	compose.requestReply.template.SetValue("lookup-copy")
	savingValue, saveCommand, _ := compose.updateRequestReplyKey(tea.KeyMsg{Type: tea.KeyCtrlT})
	if saveCommand == nil || saved.Name() != "lookup-copy" {
		t.Fatalf("saved=%#v", saved)
	}
	_, _ = savingValue.(Model).Update(saveCommand())
}

func TestRequestReplyWorkspaceFitsNarrowViewport(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 48, 24)
	model.activeWorkspace = 7
	view := model.workspaceView(48, 24)
	if width := lipgloss.Width(view); width != 48 {
		t.Fatalf("request workspace width=%d, want 48\n%s", width, view)
	}
	if height := lipgloss.Height(view); height != 24 {
		t.Fatalf("request workspace height=%d, want 24\n%s", height, view)
	}
	for _, want := range []string{"Publish / Request", "SUBJECT", "PAYLOAD", "ctrl+s"} {
		if !strings.Contains(view, want) {
			t.Fatalf("narrow workspace missing %q:\n%s", want, view)
		}
	}
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	message, _ := domainrequestreply.NewMessage("rpc.lookup", []byte(`{"id":42}`), nil)
	request, _ := domainrequestreply.NewRequest(message, time.Second, 1)
	reply, _ := domainrequestreply.NewReply([]byte(`{"ok":true}`), nil, now, time.Millisecond)
	exchange, _ := domainrequestreply.NewExchange("req-1", now, request, []domainrequestreply.Reply{reply})
	model.requestReply.exchange = &exchange
	model.requestReply.pane = requestResults
	resultView := model.workspaceView(48, 24)
	if width := lipgloss.Width(resultView); width != 48 {
		t.Fatalf("narrow results width=%d\n%s", width, resultView)
	}
	if height := lipgloss.Height(resultView); height != 24 {
		t.Fatalf("narrow results height=%d\n%s", height, resultView)
	}
}

func TestRequestReplyComposerAcceptsDigitsShowsCursorAndEscapeLeavesWorkspace(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 100, 28)
	enteredValue, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'8'}})
	entered := enteredValue.(Model)
	if entered.activeWorkspace != 7 {
		t.Fatalf("workspace=%d, want publish/request", entered.activeWorkspace)
	}
	if entered.requestReply.subject.Cursor.Mode().String() == "hidden" {
		t.Fatal("focused composer cursor is hidden")
	}
	secondValue, _ := entered.Update(tea.KeyMsg{Type: tea.KeyTab})
	thirdValue, _ := secondValue.(Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	payload := thirdValue.(Model)
	if payload.requestReply.focus != 2 {
		t.Fatalf("focus=%d, want payload", payload.requestReply.focus)
	}
	typedValue, _ := payload.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	typed := typedValue.(Model)
	if typed.activeWorkspace != 7 || typed.requestReply.payload.Value() != "1" {
		t.Fatalf("digit switched workspace or was lost: workspace=%d payload=%q", typed.activeWorkspace, typed.requestReply.payload.Value())
	}
	leftValue, _ := typed.Update(tea.KeyMsg{Type: tea.KeyEsc})
	left := leftValue.(Model)
	if left.activeWorkspace != 0 {
		t.Fatalf("escape left workspace=%d, want dashboard", left.activeWorkspace)
	}
}

func newTestModel(connect ConnectFunc) Model {
	return NewModel("local", "nats://127.0.0.1:4222", connect)
}

func dashboardModel(t *testing.T, width, height int) Model {
	t.Helper()

	model := NewModel("production", "nats://127.0.0.1:4222", nil)
	model.screen = screenWorkspace
	model.snapshot = dashboard.Snapshot{
		ServerName: "nats-a",
		Cluster:    "production",
		Version:    "2.12.0",
		Streams:    12,
		Consumers:  31,
		Messages:   2_410_000,
		Bytes:      14_800_000_000,
		ObservedAt: time.Now(),
	}
	model.width = width
	model.height = height
	return model
}

func cardLabelLines(view string) []int {
	labels := []string{"MESSAGES", "STORAGE", "STREAMS", "CONSUMERS"}
	result := make([]int, len(labels))
	for lineNumber, line := range strings.Split(view, "\n") {
		for index, label := range labels {
			if strings.Contains(line, label) {
				result[index] = lineNumber
			}
		}
	}
	return result
}
