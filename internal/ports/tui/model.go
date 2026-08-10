package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	appalerts "github.com/clcsolutions/nats-tui/internal/app/alerts"
	appbookmarks "github.com/clcsolutions/nats-tui/internal/app/bookmarks"
	appcommandbar "github.com/clcsolutions/nats-tui/internal/app/commandbar"
	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/clcsolutions/nats-tui/internal/app/subjects"
	domainrequestreply "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
)

type screen uint8

const (
	screenWelcome screen = iota
	screenConnection
	screenConnecting
	screenWorkspace
)

const minimumConnectionDisplay = 450 * time.Millisecond

const (
	messageBufferLimit = 2_000
	watchBatchLimit    = 100
	watchBatchInterval = 50 * time.Millisecond
	refreshInterval    = 3 * time.Second
	refreshRetryDelay  = 2 * time.Second
)

type ConnectionInput struct {
	Profile string
	Server  string
}

type ConnectFunc func(ConnectionInput) tea.Cmd
type LoadMessagesFunc func(string) tea.Cmd
type StartWatchFunc func(string, []string) tea.Cmd
type StopWatchFunc func()
type RefreshFunc func(uint64) tea.Cmd
type ConsumerActionFunc func(ConsumerActionInput) tea.Cmd
type StreamActionFunc func(StreamActionInput) tea.Cmd
type StreamPurgeFunc func(StreamPurgeInput) tea.Cmd
type StartStreamBackupFunc func(streams.BackupFile) tea.Cmd
type StartStreamRestoreFunc func(streams.RestoreFile) tea.Cmd
type KeyValueCreateFunc func(KeyValueCreateInput) tea.Cmd
type KeyValueUpdateFunc func(KeyValueUpdateInput) tea.Cmd
type KeyValueDeleteFunc func(KeyValueDeleteInput) tea.Cmd
type LoadKeyValueKeysFunc func(string) tea.Cmd
type LoadKeyValueEntryFunc func(string, string) tea.Cmd
type KeyValuePutFunc func(KeyValuePutInput) tea.Cmd
type KeyValueRemoveFunc func(KeyValueRemoveInput) tea.Cmd
type LoadKeyValueHistoryFunc func(string, string) tea.Cmd
type KeyValueRestoreFunc func(KeyValueRestoreInput) tea.Cmd
type StartKeyValueWatchFunc func(string, string) tea.Cmd
type CopyValueFunc func(string) error
type LoadObjectsFunc func(string) tea.Cmd
type StartObjectUploadFunc func(objectstore.UploadFile) tea.Cmd
type StartObjectDownloadFunc func(objectstore.DownloadFile) tea.Cmd
type EditObjectFunc func(objectstore.EditObject) tea.Cmd
type DeleteObjectFunc func(string, string) tea.Cmd
type StartObjectWatchFunc func(ObjectWatchInput) tea.Cmd
type CreateObjectLinkFunc func(objectstore.LinkDefinition) tea.Cmd
type ObjectBucketCreateFunc func(ObjectBucketCreateInput) tea.Cmd
type ObjectBucketUpdateFunc func(ObjectBucketUpdateInput) tea.Cmd
type ObjectBucketDeleteFunc func(ObjectBucketDeleteInput) tea.Cmd

// ObjectWatchInput identifies one watch lifecycle generation and bucket.
type ObjectWatchInput struct {
	Generation uint64
	Bucket     string
}

type ObjectBucketCreateInput struct {
	Generation uint64
	Definition objectstore.Definition
}

type ObjectBucketUpdateInput struct {
	Generation uint64
	Name       string
	Changes    objectstore.Changes
}

type ObjectBucketDeleteInput struct {
	Generation uint64
	Name       string
}

type ConsumerActionInput struct {
	Generation uint64
	Stream     string
	Name       string
	Action     consumers.Action
}

type StreamActionInput struct {
	Generation uint64
	Action     streams.Action
	Name       string
	Definition streams.Definition
}

type StreamPurgeInput struct {
	Generation uint64
	Command    streams.PurgeCommand
}

type KeyValueCreateInput struct {
	Generation uint64
	Definition kvstore.Definition
}

type KeyValueUpdateInput struct {
	Generation uint64
	Name       string
	Changes    kvstore.Changes
}

type KeyValueDeleteInput struct {
	Generation uint64
	Name       string
}

type KeyValuePutInput struct {
	Generation uint64
	Command    kvstore.WriteEntry
}

type KeyValueRemoveInput struct {
	Generation uint64
	Command    kvstore.RemoveEntry
}

type KeyValueRestoreInput struct {
	Generation uint64
	Command    kvstore.RestoreEntry
}

type Model struct {
	profile                      string
	snapshot                     dashboard.Snapshot
	connect                      ConnectFunc
	loadMessages                 LoadMessagesFunc
	startWatch                   StartWatchFunc
	stopWatch                    StopWatchFunc
	refresh                      RefreshFunc
	consumerAction               ConsumerActionFunc
	streamAction                 StreamActionFunc
	streamPurge                  StreamPurgeFunc
	startStreamBackup            StartStreamBackupFunc
	startStreamRestore           StartStreamRestoreFunc
	stopStreamTransfer           StopWatchFunc
	keyValueCreate               KeyValueCreateFunc
	keyValueUpdate               KeyValueUpdateFunc
	keyValueDelete               KeyValueDeleteFunc
	loadKeyValueKeys             LoadKeyValueKeysFunc
	loadKeyValueEntry            LoadKeyValueEntryFunc
	keyValuePut                  KeyValuePutFunc
	keyValueRemove               KeyValueRemoveFunc
	loadKeyValueHistory          LoadKeyValueHistoryFunc
	keyValueRestore              KeyValueRestoreFunc
	startKeyValueWatch           StartKeyValueWatchFunc
	stopKeyValueWatch            StopWatchFunc
	copyValue                    CopyValueFunc
	loadObjects                  LoadObjectsFunc
	startObjectUpload            StartObjectUploadFunc
	startObjectDownload          StartObjectDownloadFunc
	stopObjectTransfer           StopWatchFunc
	editObject                   EditObjectFunc
	deleteObject                 DeleteObjectFunc
	startObjectWatch             StartObjectWatchFunc
	stopObjectWatch              StopWatchFunc
	createObjectLink             CreateObjectLinkFunc
	objectBucketCreate           ObjectBucketCreateFunc
	objectBucketUpdate           ObjectBucketUpdateFunc
	objectBucketDelete           ObjectBucketDeleteFunc
	requestReplyServices         RequestReplyServices
	topologyServices             TopologyServices
	connectionServices           ConnectionServices
	operatorServices             OperatorServices
	alertServices                AlertServices
	commandBarServices           CommandBarServices
	bookmarkServices             BookmarkServices
	operator                     operatorState
	alerts                       alertState
	commandBar                   commandBarState
	bookmarks                    bookmarkState
	serverInput                  textinput.Model
	spinner                      spinner.Model
	err                          error
	width                        int
	height                       int
	screen                       screen
	activeWorkspace              int
	showHelp                     bool
	streams                      []streams.Summary
	streamCursor                 int
	streamForm                   *streamForm
	streamPurgeForm              *streamPurgeForm
	streamTransferForm           *streamTransferForm
	streamTransferStarting       bool
	streamTransferActive         bool
	streamTransferCanceling      bool
	streamTransferEvents         <-chan streams.TransferEvent
	streamTransferProgress       streams.TransferProgress
	streamTransferErr            error
	streamTransferNotice         string
	streamTransferDirection      string
	streamTransferName           string
	streamTransferDirectory      string
	streamTransferStartedAt      time.Time
	showStreamDetail             bool
	confirmStreamDelete          bool
	streamActionRunning          bool
	streamActionErr              error
	streamActionNotice           string
	consumers                    []consumers.Summary
	consumerCursor               int
	showConsumerDetail           bool
	keyValueBuckets              []kvstore.BucketSummary
	keyValueCursor               int
	keyValueForm                 *keyValueForm
	keyValueCreateRunning        bool
	keyValueCreateErr            error
	keyValueCreateNotice         string
	confirmKeyValueDelete        bool
	browsingKeyValue             bool
	keyValueBucket               string
	keyValueKeys                 []string
	filteredKeyValueKeys         []string
	keyValueKeyCursor            int
	keyValueKeyQuery             string
	keyValueKeySearchMode        bool
	loadingKeyValueKeys          bool
	keyValueKeysErr              error
	keyValueEntry                *kvstore.Entry
	loadingKeyValueEntry         bool
	keyValueEntryErr             error
	showKeyValueEntry            bool
	keyValueEntryOffset          int
	keyValueEntryForm            *keyValueEntryForm
	keyValueEntrySaving          bool
	keyValueEntryWriteErr        error
	pendingKeyValueEntryRemoval  kvstore.RemoveAction
	keyValueHistory              []kvstore.Entry
	keyValueHistoryCursor        int
	loadingKeyValueHistory       bool
	keyValueHistoryErr           error
	showKeyValueHistory          bool
	showKeyValueHistoryEntry     bool
	keyValueHistoryOffset        int
	pendingKeyValueRestore       *kvstore.Entry
	keyValueWatching             bool
	keyValueWatchStarting        bool
	keyValueWatchEvents          <-chan kvstore.Entry
	keyValueCopyNotice           string
	objectBuckets                []objectstore.BucketSummary
	objectCursor                 int
	browsingObjects              bool
	objectBucket                 string
	objects                      []objectstore.ObjectSummary
	filteredObjects              []objectstore.ObjectSummary
	objectSearchDocuments        []string
	objectObjectCursor           int
	objectSearchQuery            string
	objectSearchMode             bool
	showObjectDetail             bool
	objectDetailOffset           int
	loadingObjects               bool
	objectsErr                   error
	objectTransferForm           *objectTransferForm
	objectTransferStarting       bool
	objectTransferActive         bool
	objectTransferCanceling      bool
	objectTransferEvents         <-chan objectstore.TransferEvent
	objectTransferProgress       objectstore.Progress
	objectTransferErr            error
	objectTransferNotice         string
	objectTransferDirection      string
	objectTransferName           string
	objectTransferPath           string
	objectTransferStartedAt      time.Time
	objectEditForm               *objectEditForm
	confirmObjectDelete          bool
	objectOperationRunning       bool
	objectOperationErr           error
	objectOperationNotice        string
	objectOperationName          string
	objectWatching               bool
	objectWatchStarting          bool
	objectWatchPaused            bool
	objectWatchEvents            <-chan objectstore.ObjectEvent
	objectWatchReceived          uint64
	objectWatchErr               error
	objectWatchGeneration        uint64
	objectLinkForm               *objectLinkForm
	objectBucketForm             *objectBucketForm
	objectBucketOperationRunning bool
	objectBucketOperationErr     error
	objectBucketOperationNotice  string
	confirmObjectBucketDelete    bool
	requestReply                 requestReplyState
	topology                     topologyState
	connections                  connectionsState
	refreshGeneration            uint64
	refreshing                   bool
	refreshErr                   error
	pendingConsumerAction        consumers.Action
	consumerActionRunning        bool
	consumerActionErr            error
	consumerActionNotice         string
	messages                     []messages.Record
	messageSearch                messages.SearchIndex
	filteredMessages             []messages.Record
	searchQuery                  string
	searchMode                   bool
	watchStarting                bool
	watching                     bool
	watchPaused                  bool
	watchEvents                  <-chan messages.Record
	liveReceived                 uint64
	messageStream                string
	watchSubjects                []string
	messageCursor                int
	loadingMessages              bool
	messageErr                   error
	showMessageDetail            bool
	messageDetailOffset          int
	subjects                     []subjects.Node
	filteredSubjects             []subjects.Node
	subjectCursor                int
	subjectSearchQuery           string
	subjectSearchMode            bool
	theme                        theme
	runtime                      Runtime
}

type welcomeDoneMsg struct{}

type connectedMsg struct {
	profile   string
	snapshot  dashboard.Snapshot
	streams   []streams.Summary
	consumers []consumers.Summary
	buckets   []kvstore.BucketSummary
	objects   []objectstore.BucketSummary
}

type connectionFailedMsg struct {
	err error
}

type messagesLoadedMsg struct {
	stream  string
	records []messages.Record
}

type messagesFailedMsg struct{ err error }

type watchStartedMsg struct{ events <-chan messages.Record }
type watchFailedMsg struct{ err error }
type watchBatchMsg struct{ records []messages.Record }
type watchEndedMsg struct{}

type refreshTickMsg struct{ generation uint64 }
type refreshedMsg struct {
	generation uint64
	snapshot   dashboard.Snapshot
	streams    []streams.Summary
	consumers  []consumers.Summary
	buckets    []kvstore.BucketSummary
	objects    []objectstore.BucketSummary
}
type refreshFailedMsg struct {
	generation uint64
	err        error
}
type consumerActionSucceededMsg struct{ input ConsumerActionInput }
type consumerActionFailedMsg struct {
	input ConsumerActionInput
	err   error
}
type streamActionSucceededMsg struct{ input StreamActionInput }
type streamActionFailedMsg struct {
	input StreamActionInput
	err   error
}
type streamPurgeSucceededMsg struct{ input StreamPurgeInput }
type streamPurgeFailedMsg struct {
	input StreamPurgeInput
	err   error
}
type streamTransferStartedMsg struct{ events <-chan streams.TransferEvent }
type streamTransferFailedMsg struct{ err error }
type streamTransferEventMsg struct {
	event  streams.TransferEvent
	events <-chan streams.TransferEvent
}
type streamTransferEndedMsg struct{}
type keyValueCreateSucceededMsg struct{ input KeyValueCreateInput }
type keyValueCreateFailedMsg struct {
	input KeyValueCreateInput
	err   error
}
type keyValueUpdateSucceededMsg struct{ input KeyValueUpdateInput }
type keyValueUpdateFailedMsg struct {
	input KeyValueUpdateInput
	err   error
}
type keyValueDeleteSucceededMsg struct{ input KeyValueDeleteInput }
type keyValueDeleteFailedMsg struct {
	input KeyValueDeleteInput
	err   error
}
type keyValueKeysLoadedMsg struct {
	bucket string
	keys   []string
}
type keyValueKeysFailedMsg struct{ err error }
type keyValueEntryLoadedMsg struct{ entry kvstore.Entry }
type keyValueEntryFailedMsg struct{ err error }
type keyValuePutSucceededMsg struct {
	generation uint64
	entry      kvstore.Entry
}
type keyValuePutFailedMsg struct {
	generation uint64
	command    kvstore.WriteEntry
	err        error
}
type keyValueRemoveSucceededMsg struct {
	generation uint64
	command    kvstore.RemoveEntry
}
type keyValueRemoveFailedMsg struct {
	generation uint64
	command    kvstore.RemoveEntry
	err        error
}
type keyValueHistoryLoadedMsg struct {
	bucket  string
	key     string
	entries []kvstore.Entry
}
type keyValueHistoryFailedMsg struct{ err error }
type keyValueRestoreSucceededMsg struct {
	generation uint64
	command    kvstore.RestoreEntry
	result     kvstore.RestoredEntry
}
type keyValueRestoreFailedMsg struct {
	generation uint64
	err        error
}
type keyValueWatchStartedMsg struct{ events <-chan kvstore.Entry }
type keyValueWatchFailedMsg struct{ err error }
type keyValueWatchEventMsg struct {
	entry  kvstore.Entry
	events <-chan kvstore.Entry
}
type keyValueWatchEndedMsg struct{}
type objectsLoadedMsg struct {
	bucket  string
	objects []objectstore.ObjectSummary
}
type objectsFailedMsg struct{ err error }
type objectTransferStartedMsg struct {
	events <-chan objectstore.TransferEvent
}
type objectTransferFailedMsg struct{ err error }
type objectTransferEventMsg struct {
	event  objectstore.TransferEvent
	events <-chan objectstore.TransferEvent
}
type objectTransferEndedMsg struct{}
type objectEditSucceededMsg struct{ object objectstore.ObjectSummary }
type objectEditFailedMsg struct{ err error }
type objectDeleteSucceededMsg struct{ bucket, name string }
type objectDeleteFailedMsg struct{ err error }
type objectWatchStartedMsg struct {
	generation uint64
	events     <-chan objectstore.ObjectEvent
}
type objectWatchFailedMsg struct {
	generation uint64
	err        error
}
type objectWatchEventMsg struct {
	generation uint64
	event      objectstore.ObjectEvent
	events     <-chan objectstore.ObjectEvent
}
type objectWatchEndedMsg struct{ generation uint64 }
type objectLinkSucceededMsg struct{ object objectstore.ObjectSummary }
type objectLinkFailedMsg struct{ err error }
type objectBucketCreateSucceededMsg struct{ input ObjectBucketCreateInput }
type objectBucketCreateFailedMsg struct {
	input ObjectBucketCreateInput
	err   error
}
type objectBucketUpdateSucceededMsg struct{ input ObjectBucketUpdateInput }
type objectBucketUpdateFailedMsg struct {
	input ObjectBucketUpdateInput
	err   error
}
type objectBucketDeleteSucceededMsg struct{ input ObjectBucketDeleteInput }
type objectBucketDeleteFailedMsg struct {
	input ObjectBucketDeleteInput
	err   error
}

func NewModel(profile, server string, connect ConnectFunc, loaders ...LoadMessagesFunc) Model {
	colors := midnight()
	serverInput := textinput.New()
	serverInput.SetValue(server)
	serverInput.Placeholder = "nats://server:4222"
	serverInput.Prompt = "› "
	serverInput.CharLimit = 512
	serverInput.Width = 62
	serverInput.PromptStyle = lipgloss.NewStyle().Foreground(colors.accent)
	serverInput.TextStyle = lipgloss.NewStyle().Foreground(colors.primary)
	serverInput.Cursor.Style = lipgloss.NewStyle().Foreground(colors.cyan)

	loader := spinner.New()
	loader.Spinner = spinner.Dot
	loader.Style = lipgloss.NewStyle().Foreground(colors.accent)

	model := Model{
		profile:      profile,
		connect:      connect,
		serverInput:  serverInput,
		spinner:      loader,
		screen:       screenWelcome,
		theme:        colors,
		runtime:      systemRuntime{},
		requestReply: newRequestReplyState(colors),
	}
	if len(loaders) > 0 {
		model.loadMessages = loaders[0]
	}
	return model
}

func (m Model) WithRuntime(runtime Runtime) Model {
	if runtime == nil {
		panic("nil TUI runtime")
	}
	m.runtime = runtime
	return m
}

func (m Model) WithMessageWatch(start StartWatchFunc, stop StopWatchFunc) Model {
	m.startWatch = start
	m.stopWatch = stop
	return m
}

func (m Model) WithRefresh(refresh RefreshFunc) Model {
	m.refresh = refresh
	return m
}

func (m Model) WithConsumerActions(action ConsumerActionFunc) Model {
	m.consumerAction = action
	return m
}

func (m Model) WithStreamActions(action StreamActionFunc) Model {
	m.streamAction = action
	return m
}

func (m Model) WithStreamPurge(purge StreamPurgeFunc) Model {
	m.streamPurge = purge
	return m
}

func (m Model) WithStreamTransfers(backup StartStreamBackupFunc, restore StartStreamRestoreFunc, stop StopWatchFunc) Model {
	m.startStreamBackup, m.startStreamRestore, m.stopStreamTransfer = backup, restore, stop
	return m
}

func (m Model) WithKeyValueCreate(create KeyValueCreateFunc) Model {
	m.keyValueCreate = create
	return m
}

func (m Model) WithKeyValueUpdate(update KeyValueUpdateFunc) Model {
	m.keyValueUpdate = update
	return m
}

func (m Model) WithKeyValueDelete(remove KeyValueDeleteFunc) Model {
	m.keyValueDelete = remove
	return m
}

func (m Model) WithKeyValueKeys(load LoadKeyValueKeysFunc) Model {
	m.loadKeyValueKeys = load
	return m
}

func (m Model) WithKeyValueEntry(load LoadKeyValueEntryFunc) Model {
	m.loadKeyValueEntry = load
	return m
}

func (m Model) WithKeyValuePut(put KeyValuePutFunc) Model {
	m.keyValuePut = put
	return m
}

func (m Model) WithKeyValueRemove(remove KeyValueRemoveFunc) Model {
	m.keyValueRemove = remove
	return m
}

func (m Model) WithKeyValueHistory(load LoadKeyValueHistoryFunc) Model {
	m.loadKeyValueHistory = load
	return m
}

func (m Model) WithKeyValueRestore(restore KeyValueRestoreFunc) Model {
	m.keyValueRestore = restore
	return m
}

func (m Model) WithKeyValueWatch(start StartKeyValueWatchFunc, stop StopWatchFunc) Model {
	m.startKeyValueWatch = start
	m.stopKeyValueWatch = stop
	return m
}

func (m Model) WithCopyValue(copyValue CopyValueFunc) Model {
	m.copyValue = copyValue
	return m
}

func (m Model) WithObjectCatalog(load LoadObjectsFunc) Model {
	m.loadObjects = load
	return m
}

func (m Model) WithObjectTransfers(upload StartObjectUploadFunc, download StartObjectDownloadFunc, stop StopWatchFunc) Model {
	m.startObjectUpload = upload
	m.startObjectDownload = download
	m.stopObjectTransfer = stop
	return m
}

func (m Model) WithObjectOperations(edit EditObjectFunc, remove DeleteObjectFunc) Model {
	m.editObject = edit
	m.deleteObject = remove
	return m
}

func (m Model) WithObjectWatch(start StartObjectWatchFunc, stop StopWatchFunc) Model {
	m.startObjectWatch = start
	m.stopObjectWatch = stop
	return m
}

func (m Model) WithObjectLinks(create CreateObjectLinkFunc) Model {
	m.createObjectLink = create
	return m
}

func (m Model) WithObjectBucketCreate(create ObjectBucketCreateFunc) Model {
	m.objectBucketCreate = create
	return m
}

func (m Model) WithObjectBucketUpdate(update ObjectBucketUpdateFunc) Model {
	m.objectBucketUpdate = update
	return m
}

func (m Model) WithObjectBucketDelete(remove ObjectBucketDeleteFunc) Model {
	m.objectBucketDelete = remove
	return m
}

func Connected(profile string, snapshot dashboard.Snapshot, streamCatalog []streams.Summary, consumerCatalog []consumers.Summary, buckets ...[]kvstore.BucketSummary) tea.Msg {
	var bucketCatalog []kvstore.BucketSummary
	if len(buckets) > 0 {
		bucketCatalog = buckets[0]
	}
	return connectedMsg{profile: profile, snapshot: snapshot, streams: streamCatalog, consumers: consumerCatalog, buckets: bucketCatalog}
}

func ConnectedWithObjects(profile string, snapshot dashboard.Snapshot, streamCatalog []streams.Summary, consumerCatalog []consumers.Summary, buckets []kvstore.BucketSummary, objects []objectstore.BucketSummary) tea.Msg {
	return connectedMsg{profile: profile, snapshot: snapshot, streams: streamCatalog, consumers: consumerCatalog, buckets: buckets, objects: objects}
}

func ConnectionFailed(err error) tea.Msg {
	return connectionFailedMsg{err: err}
}

func MessagesLoaded(stream string, records []messages.Record) tea.Msg {
	return messagesLoadedMsg{stream: stream, records: records}
}

func MessagesFailed(err error) tea.Msg { return messagesFailedMsg{err: err} }

func WatchStarted(events <-chan messages.Record) tea.Msg { return watchStartedMsg{events: events} }
func WatchFailed(err error) tea.Msg                      { return watchFailedMsg{err: err} }

func Refreshed(generation uint64, snapshot dashboard.Snapshot, streamCatalog []streams.Summary, consumerCatalog []consumers.Summary, buckets ...[]kvstore.BucketSummary) tea.Msg {
	var bucketCatalog []kvstore.BucketSummary
	if len(buckets) > 0 {
		bucketCatalog = buckets[0]
	}
	return refreshedMsg{generation: generation, snapshot: snapshot, streams: streamCatalog, consumers: consumerCatalog, buckets: bucketCatalog}
}

func RefreshedWithObjects(generation uint64, snapshot dashboard.Snapshot, streamCatalog []streams.Summary, consumerCatalog []consumers.Summary, buckets []kvstore.BucketSummary, objects []objectstore.BucketSummary) tea.Msg {
	return refreshedMsg{generation: generation, snapshot: snapshot, streams: streamCatalog, consumers: consumerCatalog, buckets: buckets, objects: objects}
}

func RefreshFailed(generation uint64, err error) tea.Msg {
	return refreshFailedMsg{generation: generation, err: err}
}

func ConsumerActionSucceeded(generation uint64, input ConsumerActionInput) tea.Msg {
	input.Generation = generation
	return consumerActionSucceededMsg{input: input}
}

func ConsumerActionFailed(generation uint64, input ConsumerActionInput, err error) tea.Msg {
	input.Generation = generation
	return consumerActionFailedMsg{input: input, err: err}
}

func StreamActionSucceeded(generation uint64, input StreamActionInput) tea.Msg {
	input.Generation = generation
	return streamActionSucceededMsg{input: input}
}

func StreamActionFailed(generation uint64, input StreamActionInput, err error) tea.Msg {
	input.Generation = generation
	return streamActionFailedMsg{input: input, err: err}
}

func StreamPurgeSucceeded(input StreamPurgeInput) tea.Msg { return streamPurgeSucceededMsg{input} }
func StreamPurgeFailed(input StreamPurgeInput, err error) tea.Msg {
	return streamPurgeFailedMsg{input, err}
}

func KeyValueCreateSucceeded(generation uint64, definition kvstore.Definition) tea.Msg {
	return keyValueCreateSucceededMsg{input: KeyValueCreateInput{Generation: generation, Definition: definition}}
}

func KeyValueCreateFailed(generation uint64, definition kvstore.Definition, err error) tea.Msg {
	return keyValueCreateFailedMsg{input: KeyValueCreateInput{Generation: generation, Definition: definition}, err: err}
}

func KeyValueUpdateSucceeded(generation uint64, name string, changes kvstore.Changes) tea.Msg {
	return keyValueUpdateSucceededMsg{input: KeyValueUpdateInput{Generation: generation, Name: name, Changes: changes}}
}

func KeyValueUpdateFailed(generation uint64, name string, changes kvstore.Changes, err error) tea.Msg {
	return keyValueUpdateFailedMsg{input: KeyValueUpdateInput{Generation: generation, Name: name, Changes: changes}, err: err}
}

func KeyValueDeleteSucceeded(generation uint64, name string) tea.Msg {
	return keyValueDeleteSucceededMsg{input: KeyValueDeleteInput{Generation: generation, Name: name}}
}

func KeyValueDeleteFailed(generation uint64, name string, err error) tea.Msg {
	return keyValueDeleteFailedMsg{input: KeyValueDeleteInput{Generation: generation, Name: name}, err: err}
}

func KeyValueKeysLoaded(bucket string, keys []string) tea.Msg {
	return keyValueKeysLoadedMsg{bucket: bucket, keys: keys}
}
func KeyValueKeysFailed(err error) tea.Msg            { return keyValueKeysFailedMsg{err: err} }
func KeyValueEntryLoaded(entry kvstore.Entry) tea.Msg { return keyValueEntryLoadedMsg{entry: entry} }
func KeyValueEntryFailed(err error) tea.Msg           { return keyValueEntryFailedMsg{err: err} }
func KeyValuePutSucceeded(generation uint64, entry kvstore.Entry) tea.Msg {
	return keyValuePutSucceededMsg{generation: generation, entry: entry}
}
func KeyValuePutFailed(generation uint64, command kvstore.WriteEntry, err error) tea.Msg {
	return keyValuePutFailedMsg{generation: generation, command: command, err: err}
}
func KeyValueRemoveSucceeded(generation uint64, command kvstore.RemoveEntry) tea.Msg {
	return keyValueRemoveSucceededMsg{generation: generation, command: command}
}
func KeyValueRemoveFailed(generation uint64, command kvstore.RemoveEntry, err error) tea.Msg {
	return keyValueRemoveFailedMsg{generation: generation, command: command, err: err}
}
func KeyValueHistoryLoaded(bucket, key string, entries []kvstore.Entry) tea.Msg {
	return keyValueHistoryLoadedMsg{bucket: bucket, key: key, entries: entries}
}
func KeyValueHistoryFailed(err error) tea.Msg { return keyValueHistoryFailedMsg{err: err} }
func KeyValueRestoreSucceeded(generation uint64, command kvstore.RestoreEntry, result kvstore.RestoredEntry) tea.Msg {
	return keyValueRestoreSucceededMsg{generation: generation, command: command, result: result}
}
func KeyValueRestoreFailed(generation uint64, err error) tea.Msg {
	return keyValueRestoreFailedMsg{generation: generation, err: err}
}
func KeyValueWatchStarted(events <-chan kvstore.Entry) tea.Msg {
	return keyValueWatchStartedMsg{events: events}
}
func KeyValueWatchFailed(err error) tea.Msg { return keyValueWatchFailedMsg{err: err} }

func ObjectsLoaded(bucket string, objects []objectstore.ObjectSummary) tea.Msg {
	return objectsLoadedMsg{bucket: bucket, objects: objects}
}

func ObjectsFailed(err error) tea.Msg { return objectsFailedMsg{err: err} }

func ObjectTransferStarted(events <-chan objectstore.TransferEvent) tea.Msg {
	return objectTransferStartedMsg{events: events}
}

func ObjectTransferFailed(err error) tea.Msg { return objectTransferFailedMsg{err: err} }

func StreamTransferStarted(events <-chan streams.TransferEvent) tea.Msg {
	return streamTransferStartedMsg{events: events}
}

func StreamTransferFailed(err error) tea.Msg { return streamTransferFailedMsg{err: err} }

func ObjectEditSucceeded(object objectstore.ObjectSummary) tea.Msg {
	return objectEditSucceededMsg{object: object}
}
func ObjectEditFailed(err error) tea.Msg { return objectEditFailedMsg{err: err} }
func ObjectDeleteSucceeded(bucket, name string) tea.Msg {
	return objectDeleteSucceededMsg{bucket: bucket, name: name}
}
func ObjectDeleteFailed(err error) tea.Msg { return objectDeleteFailedMsg{err: err} }
func ObjectWatchStarted(generation uint64, events <-chan objectstore.ObjectEvent) tea.Msg {
	return objectWatchStartedMsg{generation: generation, events: events}
}
func ObjectWatchFailed(generation uint64, err error) tea.Msg {
	return objectWatchFailedMsg{generation: generation, err: err}
}
func ObjectLinkSucceeded(object objectstore.ObjectSummary) tea.Msg {
	return objectLinkSucceededMsg{object: object}
}
func ObjectLinkFailed(err error) tea.Msg { return objectLinkFailedMsg{err: err} }
func ObjectBucketCreateSucceeded(input ObjectBucketCreateInput) tea.Msg {
	return objectBucketCreateSucceededMsg{input: input}
}
func ObjectBucketCreateFailed(input ObjectBucketCreateInput, err error) tea.Msg {
	return objectBucketCreateFailedMsg{input: input, err: err}
}
func ObjectBucketUpdateSucceeded(input ObjectBucketUpdateInput) tea.Msg {
	return objectBucketUpdateSucceededMsg{input: input}
}
func ObjectBucketUpdateFailed(input ObjectBucketUpdateInput, err error) tea.Msg {
	return objectBucketUpdateFailedMsg{input: input, err: err}
}
func ObjectBucketDeleteSucceeded(input ObjectBucketDeleteInput) tea.Msg {
	return objectBucketDeleteSucceededMsg{input: input}
}
func ObjectBucketDeleteFailed(input ObjectBucketDeleteInput, err error) tea.Msg {
	return objectBucketDeleteFailedMsg{input: input, err: err}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.runtime.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return welcomeDoneMsg{} }),
		textinput.Blink,
	)
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.serverInput.Width = max(min(msg.Width-18, 68), 20)
	case welcomeDoneMsg:
		return m.showConnection()
	case connectedMsg:
		m.profile = msg.profile
		m.snapshot = msg.snapshot
		m.streams = append([]streams.Summary(nil), msg.streams...)
		m.streamCursor = 0
		m.rebuildSubjects()
		m.consumers = append([]consumers.Summary(nil), msg.consumers...)
		m.consumerCursor = 0
		m.keyValueBuckets = append([]kvstore.BucketSummary(nil), msg.buckets...)
		m.keyValueCursor = 0
		m.objectBuckets = append([]objectstore.BucketSummary(nil), msg.objects...)
		m.objectCursor = 0
		m.browsingObjects = false
		m.objectBucket = ""
		m.objects = nil
		m.filteredObjects = nil
		m.objectSearchDocuments = nil
		m.objectObjectCursor = 0
		m.objectSearchQuery = ""
		m.objectSearchMode = false
		m.showObjectDetail = false
		m.objectDetailOffset = 0
		m.loadingObjects = false
		m.objectsErr = nil
		m.objectWatching = false
		m.objectWatchStarting = false
		m.objectWatchPaused = false
		m.objectWatchEvents = nil
		m.objectWatchReceived = 0
		m.objectWatchErr = nil
		m.refreshGeneration++
		m.refreshing = false
		m.refreshErr = nil
		m.err = nil
		m.screen = screenWorkspace
		commands := []tea.Cmd{m.scheduleRefresh(m.refreshGeneration, refreshInterval)}
		if m.alertServices.Load != nil {
			m.alerts.loading = true
			commands = append(commands, m.alertServices.Load(appalerts.Query{Limit: 1000}))
		}
		if m.alertServices.Watch != nil {
			commands = append(commands, m.alertServices.Watch())
		}
		if m.commandBarServices.Load != nil {
			commands = append(commands, m.commandBarServices.Load())
		}
		if m.bookmarkServices.Load != nil {
			m.bookmarks.loading = true
			commands = append(commands, m.bookmarkServices.Load(m.bookmarkInventory()))
		}
		return m, tea.Batch(commands...)
	case connectionFailedMsg:
		m.err = msg.err
		return m.showConnection()
	case messagesLoadedMsg:
		if m.watching && m.stopWatch != nil {
			m.stopWatch()
		}
		m.watching = false
		m.watchStarting = false
		m.watchPaused = false
		m.liveReceived = 0
		m.setMessages(msg.records)
		m.messageStream = msg.stream
		m.watchSubjects = m.subjectsForStream(msg.stream)
		m.messageCursor = 0
		m.messageErr = nil
		m.loadingMessages = false
		m.activeWorkspace = 3
		return m, nil
	case messagesFailedMsg:
		m.messageErr = msg.err
		m.loadingMessages = false
		m.activeWorkspace = 3
		return m, nil
	case watchStartedMsg:
		m.watchStarting = false
		m.watching = true
		m.watchPaused = false
		m.watchEvents = msg.events
		return m, m.waitForWatchBatch(msg.events)
	case watchFailedMsg:
		m.watchStarting = false
		m.watching = false
		m.messageErr = msg.err
		return m, nil
	case watchBatchMsg:
		m.liveReceived += uint64(len(msg.records))
		if m.watchPaused {
			return m, m.waitForWatchBatch(m.watchEvents)
		}
		m.prependLiveMessages(msg.records)
		return m, m.waitForWatchBatch(m.watchEvents)
	case watchEndedMsg:
		m.watchStarting = false
		m.watching = false
		m.watchPaused = false
		m.watchEvents = nil
		return m, nil
	case refreshTickMsg:
		if msg.generation != m.refreshGeneration || m.screen != screenWorkspace {
			return m, nil
		}
		return m.startRefresh()
	case refreshedMsg:
		if msg.generation != m.refreshGeneration || m.screen != screenWorkspace {
			return m, nil
		}
		m.applyRefresh(msg)
		next := m.scheduleRefresh(m.refreshGeneration, refreshInterval)
		if m.activeWorkspace == 9 && m.connectionServices.Load != nil && !m.connections.loading {
			m.connections.loading = true
			return m, tea.Batch(next, m.connectionServices.Load())
		}
		if m.activeWorkspace == 10 && m.operatorServices.Load != nil && !m.operator.loading {
			m.operator.loading = true
			return m, tea.Batch(next, m.operatorServices.Load())
		}
		return m, next
	case refreshFailedMsg:
		if msg.generation != m.refreshGeneration || m.screen != screenWorkspace {
			return m, nil
		}
		m.refreshing = false
		m.refreshErr = msg.err
		return m, m.scheduleRefresh(m.refreshGeneration, refreshRetryDelay)
	case consumerActionSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.consumerActionRunning = false
		m.consumerActionErr = nil
		m.consumerActionNotice = fmt.Sprintf("%s completed for %s", msg.input.Action, msg.input.Name)
		m.applyConsumerAction(msg.input)
		m.refreshGeneration++
		m.refreshing = false
		return m.startRefresh()
	case consumerActionFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.consumerActionRunning = false
		m.consumerActionErr = msg.err
		return m, nil
	case streamActionSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.streamActionRunning = false
		m.streamActionErr = nil
		m.streamActionNotice = fmt.Sprintf("%s completed for %s", msg.input.Action, msg.input.Name)
		m.applyStreamAction(msg.input)
		m.streamForm = nil
		m.confirmStreamDelete = false
		m.showStreamDetail = false
		m.refreshGeneration++
		m.refreshing = false
		return m.startRefresh()
	case streamActionFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.streamActionRunning = false
		m.streamActionErr = msg.err
		return m, nil
	case streamPurgeSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.streamActionRunning, m.streamActionErr, m.streamPurgeForm = false, nil, nil
		m.streamActionNotice = "purge completed for " + msg.input.Command.Stream
		m.refreshGeneration++
		return m.startRefresh()
	case streamPurgeFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.streamActionRunning, m.streamActionErr = false, msg.err
		return m, nil
	case streamTransferStartedMsg:
		m.streamTransferStarting = false
		m.streamTransferActive = true
		m.streamTransferEvents = msg.events
		return m, waitForStreamTransferEvent(msg.events)
	case streamTransferFailedMsg:
		m.streamTransferStarting = false
		m.streamTransferActive = false
		m.streamTransferErr = msg.err
		return m, nil
	case streamTransferEventMsg:
		m.streamTransferProgress = msg.event.Progress
		if msg.event.Done {
			wasCanceling := m.streamTransferCanceling
			m.streamTransferActive, m.streamTransferStarting, m.streamTransferCanceling = false, false, false
			m.streamTransferEvents = nil
			if wasCanceling {
				m.streamTransferErr = nil
				m.streamTransferNotice = "stream transfer canceled"
				return m, nil
			}
			if msg.event.Err != nil {
				m.streamTransferErr = msg.event.Err
				return m, nil
			}
			m.streamTransferErr = nil
			m.streamTransferNotice = m.streamTransferDirection + " completed for " + m.streamTransferName
			m.refreshGeneration++
			m.refreshing = false
			return m.startRefresh()
		}
		return m, waitForStreamTransferEvent(msg.events)
	case streamTransferEndedMsg:
		m.streamTransferStarting = false
		m.streamTransferActive = false
		m.streamTransferEvents = nil
		if m.streamTransferCanceling {
			m.streamTransferCanceling = false
			m.streamTransferNotice = "stream transfer canceled"
		}
		return m, nil
	case keyValueCreateSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueCreateRunning = false
		m.keyValueCreateErr = nil
		m.keyValueCreateNotice = "created " + msg.input.Definition.Name
		m.applyCreatedKeyValueBucket(msg.input.Definition)
		m.keyValueForm = nil
		m.refreshGeneration++
		m.refreshing = false
		return m.startRefresh()
	case keyValueCreateFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueCreateRunning = false
		m.keyValueCreateErr = msg.err
		return m, nil
	case keyValueUpdateSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueCreateRunning = false
		m.keyValueCreateErr = nil
		m.keyValueCreateNotice = "updated " + msg.input.Name
		m.applyUpdatedKeyValueBucket(msg.input.Name, msg.input.Changes)
		m.keyValueForm = nil
		m.refreshGeneration++
		m.refreshing = false
		return m.startRefresh()
	case keyValueUpdateFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueCreateRunning = false
		m.keyValueCreateErr = msg.err
		return m, nil
	case keyValueDeleteSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueCreateRunning = false
		m.keyValueCreateErr = nil
		m.keyValueCreateNotice = "deleted " + msg.input.Name
		m.removeKeyValueBucket(msg.input.Name)
		m.refreshGeneration++
		m.refreshing = false
		return m.startRefresh()
	case keyValueDeleteFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueCreateRunning = false
		m.keyValueCreateErr = msg.err
		return m, nil
	case keyValueKeysLoadedMsg:
		m.keyValueBucket = msg.bucket
		m.keyValueKeys = append([]string(nil), msg.keys...)
		m.filteredKeyValueKeys = nil
		m.keyValueKeyCursor = 0
		m.keyValueKeyQuery = ""
		m.loadingKeyValueKeys = false
		m.keyValueKeysErr = nil
		m.browsingKeyValue = true
		m.keyValueEntry = nil
		m.showKeyValueEntry = false
		m.resolvePendingBookmark()
		return m, nil
	case keyValueKeysFailedMsg:
		m.loadingKeyValueKeys = false
		m.keyValueKeysErr = msg.err
		return m, nil
	case keyValueEntryLoadedMsg:
		entry := msg.entry
		m.keyValueEntry = &entry
		m.loadingKeyValueEntry = false
		m.keyValueEntryErr = nil
		m.showKeyValueEntry = true
		m.keyValueEntryOffset = 0
		return m, nil
	case keyValueEntryFailedMsg:
		m.loadingKeyValueEntry = false
		m.keyValueEntryErr = msg.err
		return m, nil
	case keyValuePutSucceededMsg:
		if msg.generation != m.refreshGeneration {
			return m, nil
		}
		entry := msg.entry
		m.keyValueEntry = &entry
		m.keyValueEntryForm = nil
		m.keyValueEntrySaving = false
		m.keyValueEntryWriteErr = nil
		m.showKeyValueEntry = true
		m.keyValueEntryOffset = 0
		m.rememberKeyValueKey(entry.Key)
		m.keyValueCreateNotice = fmt.Sprintf("stored %s @ %d", entry.Key, entry.Revision)
		m.refreshGeneration++
		m.refreshing = false
		return m.startRefresh()
	case keyValuePutFailedMsg:
		if msg.generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueEntrySaving = false
		m.keyValueEntryWriteErr = msg.err
		return m, nil
	case keyValueRemoveSucceededMsg:
		if msg.generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueEntrySaving = false
		m.keyValueEntryWriteErr = nil
		m.pendingKeyValueEntryRemoval = ""
		m.showKeyValueEntry = false
		m.keyValueEntry = nil
		m.removeKeyValueKey(msg.command.Key)
		m.keyValueCreateNotice = strings.ToLower(string(msg.command.Action)) + "d " + msg.command.Key
		m.refreshGeneration++
		m.refreshing = false
		return m.startRefresh()
	case keyValueRemoveFailedMsg:
		if msg.generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueEntrySaving = false
		m.keyValueEntryWriteErr = msg.err
		m.pendingKeyValueEntryRemoval = ""
		return m, nil
	case keyValueHistoryLoadedMsg:
		m.keyValueHistory = append([]kvstore.Entry(nil), msg.entries...)
		m.keyValueHistoryCursor = max(len(m.keyValueHistory)-1, 0)
		m.loadingKeyValueHistory = false
		m.keyValueHistoryErr = nil
		m.showKeyValueHistory = true
		m.showKeyValueHistoryEntry = false
		m.keyValueHistoryOffset = 0
		return m, nil
	case keyValueHistoryFailedMsg:
		m.loadingKeyValueHistory = false
		m.keyValueHistoryErr = msg.err
		return m, nil
	case keyValueRestoreSucceededMsg:
		if msg.generation != m.refreshGeneration {
			return m, nil
		}
		entry := kvstore.Entry{Bucket: msg.command.Bucket, Key: msg.command.Key, Value: append([]byte(nil), msg.result.Value...), Operation: kvstore.OperationPut, Revision: msg.result.Revision, Created: m.runtime.Now()}
		m.keyValueEntry = &entry
		m.keyValueHistory = append(m.keyValueHistory, entry)
		m.keyValueHistoryCursor = len(m.keyValueHistory) - 1
		m.pendingKeyValueRestore = nil
		m.keyValueEntrySaving = false
		m.showKeyValueHistoryEntry = false
		m.showKeyValueHistory = false
		m.keyValueCreateNotice = fmt.Sprintf("restored %s @ %d", entry.Key, entry.Revision)
		return m, nil
	case keyValueRestoreFailedMsg:
		if msg.generation != m.refreshGeneration {
			return m, nil
		}
		m.keyValueEntrySaving = false
		m.keyValueEntryWriteErr = msg.err
		m.pendingKeyValueRestore = nil
		return m, nil
	case keyValueWatchStartedMsg:
		m.keyValueWatchStarting = false
		m.keyValueWatching = true
		m.keyValueWatchEvents = msg.events
		return m, waitForKeyValueEvent(msg.events)
	case keyValueWatchFailedMsg:
		m.keyValueWatchStarting = false
		m.keyValueWatching = false
		m.keyValueEntryErr = msg.err
		return m, nil
	case keyValueWatchEventMsg:
		entry := msg.entry
		if m.keyValueEntry != nil && entry.Bucket == m.keyValueEntry.Bucket && entry.Key == m.keyValueEntry.Key {
			m.keyValueEntry = &entry
			m.keyValueCreateNotice = fmt.Sprintf("live %s @ %d", strings.ToLower(string(entry.Operation)), entry.Revision)
			if m.showKeyValueHistory {
				m.keyValueHistory = append(m.keyValueHistory, entry)
				m.keyValueHistoryCursor = len(m.keyValueHistory) - 1
			}
		}
		return m, waitForKeyValueEvent(msg.events)
	case keyValueWatchEndedMsg:
		m.keyValueWatchStarting = false
		m.keyValueWatching = false
		m.keyValueWatchEvents = nil
		return m, nil
	case objectsLoadedMsg:
		selectedName := selectedObjectName(m.displayedObjects(), m.objectObjectCursor)
		query := ""
		if m.browsingObjects && m.objectBucket == msg.bucket {
			query = m.objectSearchQuery
		}
		m.loadingObjects = false
		m.objectsErr = nil
		m.objectBucket = msg.bucket
		m.objects = append([]objectstore.ObjectSummary(nil), msg.objects...)
		m.objectObjectCursor = 0
		m.objectSearchQuery = query
		m.objectSearchMode = false
		m.rebuildObjectSearch()
		m.objectObjectCursor = findObject(m.displayedObjects(), selectedName)
		m.browsingObjects = true
		m.resolvePendingBookmark()
		return m, nil
	case objectsFailedMsg:
		m.loadingObjects = false
		m.objectsErr = msg.err
		m.browsingObjects = true
		return m, nil
	case objectTransferStartedMsg:
		m.objectTransferStarting = false
		m.objectTransferActive = true
		m.objectTransferEvents = msg.events
		return m, waitForObjectTransferEvent(msg.events)
	case objectTransferFailedMsg:
		m.objectTransferStarting = false
		m.objectTransferActive = false
		m.objectTransferErr = msg.err
		return m, nil
	case objectTransferEventMsg:
		m.objectTransferProgress = msg.event.Progress
		if msg.event.Done {
			wasCanceling := m.objectTransferCanceling
			m.objectTransferActive = false
			m.objectTransferStarting = false
			m.objectTransferCanceling = false
			m.objectTransferEvents = nil
			if wasCanceling {
				m.objectTransferErr = nil
				m.objectTransferNotice = "transfer canceled"
				return m, nil
			}
			if msg.event.Err != nil {
				m.objectTransferErr = msg.event.Err
				return m, nil
			}
			m.applyObjectTransferResult(msg.event)
			return m, nil
		}
		return m, waitForObjectTransferEvent(msg.events)
	case objectTransferEndedMsg:
		m.objectTransferStarting = false
		m.objectTransferActive = false
		m.objectTransferEvents = nil
		if m.objectTransferCanceling {
			m.objectTransferCanceling = false
			m.objectTransferNotice = "transfer canceled"
		}
		return m, nil
	case objectEditSucceededMsg:
		m.objectOperationRunning = false
		m.objectOperationErr = nil
		m.objectOperationNotice = "updated " + msg.object.Name
		m.upsertObject(msg.object)
		return m, nil
	case objectEditFailedMsg:
		m.objectOperationRunning = false
		m.objectOperationErr = msg.err
		return m, nil
	case objectDeleteSucceededMsg:
		m.objectOperationRunning = false
		m.objectOperationErr = nil
		m.objectOperationNotice = "deleted " + msg.name
		m.removeObject(msg.name)
		return m, nil
	case objectDeleteFailedMsg:
		m.objectOperationRunning = false
		m.objectOperationErr = msg.err
		return m, nil
	case objectWatchStartedMsg:
		if msg.generation != m.objectWatchGeneration {
			return m, nil
		}
		m.objectWatchStarting = false
		m.objectWatching = true
		m.objectWatchPaused = false
		m.objectWatchEvents = msg.events
		m.objectWatchErr = nil
		return m, waitForObjectWatchEvent(msg.generation, msg.events)
	case objectWatchFailedMsg:
		if msg.generation != m.objectWatchGeneration {
			return m, nil
		}
		m.objectWatchStarting = false
		m.objectWatching = false
		m.objectWatchErr = msg.err
		return m, nil
	case objectWatchEventMsg:
		if msg.generation != m.objectWatchGeneration {
			return m, nil
		}
		m.objectWatchReceived++
		if !m.objectWatchPaused {
			m.applyObjectWatchEvent(msg.event)
		}
		return m, waitForObjectWatchEvent(msg.generation, msg.events)
	case objectWatchEndedMsg:
		if msg.generation != m.objectWatchGeneration {
			return m, nil
		}
		m.objectWatchStarting = false
		m.objectWatching = false
		m.objectWatchPaused = false
		m.objectWatchEvents = nil
		return m, nil
	case objectLinkSucceededMsg:
		m.objectOperationRunning = false
		m.objectOperationErr = nil
		m.objectOperationNotice = "linked " + msg.object.Name
		m.objectOperationName = msg.object.Name
		m.upsertObject(msg.object)
		return m, nil
	case objectLinkFailedMsg:
		m.objectOperationRunning = false
		m.objectOperationErr = msg.err
		return m, nil
	case objectBucketCreateSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.objectBucketOperationRunning = false
		m.objectBucketOperationErr = nil
		m.objectBucketOperationNotice = "created " + msg.input.Definition.Name
		m.objectBuckets = append(m.objectBuckets, objectBucketSummary(msg.input.Definition))
		sort.Slice(m.objectBuckets, func(left, right int) bool { return m.objectBuckets[left].Name < m.objectBuckets[right].Name })
		m.objectCursor = findObjectBucket(m.objectBuckets, msg.input.Definition.Name)
		m.objectBucketForm = nil
		return m, nil
	case objectBucketCreateFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.objectBucketOperationRunning = false
		m.objectBucketOperationErr = msg.err
		return m, nil
	case objectBucketUpdateSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.objectBucketOperationRunning = false
		m.objectBucketOperationErr = nil
		m.objectBucketOperationNotice = "updated " + msg.input.Name
		m.applyObjectBucketChanges(msg.input.Name, msg.input.Changes)
		m.objectBucketForm = nil
		return m, nil
	case objectBucketUpdateFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.objectBucketOperationRunning = false
		m.objectBucketOperationErr = msg.err
		return m, nil
	case objectBucketDeleteSucceededMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.objectBucketOperationRunning = false
		m.objectBucketOperationErr = nil
		m.objectBucketOperationNotice = "deleted " + msg.input.Name
		m.removeObjectBucket(msg.input.Name)
		m.confirmObjectBucketDelete = false
		return m, nil
	case objectBucketDeleteFailedMsg:
		if msg.input.Generation != m.refreshGeneration {
			return m, nil
		}
		m.objectBucketOperationRunning = false
		m.objectBucketOperationErr = msg.err
		return m, nil
	case corePublishSucceededMsg:
		m.requestReply.running = false
		m.requestReply.error = nil
		m.requestReply.notice = fmt.Sprintf("Published %d bytes to %s", len(msg.message.Payload()), msg.message.Subject())
		m.requestReply.cliEquivalent = publishCLIEquivalent(msg.message)
		return m, nil
	case corePublishFailedMsg:
		m.requestReply.running = false
		m.requestReply.error = msg.err
		return m, nil
	case coreRequestSucceededMsg:
		m.requestReply.running = false
		m.requestReply.error = nil
		m.requestReply.exchange = &msg.exchange
		m.requestReply.pane = requestResults
		m.requestReply.replyCursor = 0
		m.requestReply.cliEquivalent = requestCLIEquivalent(msg.exchange.Request())
		m.requestReply.history = append([]domainrequestreply.Exchange{msg.exchange}, m.requestReply.history...)
		return m, nil
	case coreRequestFailedMsg:
		m.requestReply.running = false
		m.requestReply.error = msg.err
		return m, nil
	case payloadFileLoadedMsg:
		m.requestReply.running = false
		m.requestReply.payload.SetValue(string(msg.payload))
		m.requestReply.notice = fmt.Sprintf("Loaded %d bytes from %s", len(msg.payload), msg.path)
		m.requestReply.error = nil
		return m, nil
	case payloadFileFailedMsg:
		m.requestReply.running = false
		m.requestReply.error = msg.err
		return m, nil
	case requestHistoryLoadedMsg:
		m.requestReply.loadingState = false
		m.requestReply.history = append([]domainrequestreply.Exchange(nil), msg.history...)
		m.requestReply.historyCursor = 0
		m.requestReply.pane = requestHistory
		return m, nil
	case requestHistoryFailedMsg:
		m.requestReply.loadingState = false
		m.requestReply.error = msg.err
		return m, nil
	case requestTemplatesLoadedMsg:
		m.requestReply.loadingState = false
		m.requestReply.templates = append([]domainrequestreply.Template(nil), msg.templates...)
		m.requestReply.templateCursor = 0
		m.requestReply.pane = requestTemplates
		return m, nil
	case requestTemplatesFailedMsg:
		m.requestReply.loadingState = false
		m.requestReply.error = msg.err
		return m, nil
	case requestTemplateSavedMsg:
		m.requestReply.running = false
		m.requestReply.error = nil
		m.requestReply.notice = "Saved template " + msg.template.Name()
		m.upsertRequestTemplate(msg.template)
		return m, nil
	case requestTemplateSaveFailedMsg:
		m.requestReply.running = false
		m.requestReply.error = msg.err
		return m, nil
	case requestTemplateDeletedMsg:
		m.requestReply.running = false
		m.removeRequestTemplate(msg.name)
		return m, nil
	case requestTemplateDeleteFailedMsg:
		m.requestReply.running = false
		m.requestReply.error = msg.err
		return m, nil
	case topologyLoadedMsg:
		m.topology.loading = false
		m.topology.err = nil
		m.topology.snapshot = msg.snapshot
		m.topology.cursor = min(m.topology.cursor, max(len(msg.snapshot.Servers())-1, 0))
		return m, nil
	case topologyFailedMsg:
		m.topology.loading = false
		m.topology.err = msg.err
		return m, nil
	case topologyOperationSucceededMsg:
		m.topology.loading = false
		m.topology.notice = string(msg.command.Action) + " completed"
		m.topology.err = nil
		if m.topologyServices.Load != nil {
			m.topology.loading = true
			return m, m.topologyServices.Load()
		}
		return m, nil
	case topologyOperationFailedMsg:
		m.topology.loading = false
		m.topology.err = msg.err
		return m, nil
	case connectionsLoadedMsg:
		m.connections.loading = false
		m.connections.err = nil
		m.connections.snapshot = msg.snapshot
		m.connections.cursor = min(m.connections.cursor, max(len(m.displayedConnections())-1, 0))
		m.connections.samples = append(m.connections.samples, connectionSample{at: msg.snapshot.ObservedAt, connections: len(msg.snapshot.Connections), subscriptions: msg.snapshot.SubscriptionCount()})
		if len(m.connections.samples) > 120 {
			m.connections.samples = append([]connectionSample(nil), m.connections.samples[len(m.connections.samples)-120:]...)
		}
		return m, nil
	case connectionsFailedMsg:
		m.connections.loading = false
		m.connections.err = msg.err
		return m, nil
	case operatorLoadedMsg:
		m.operator.loading = false
		m.operator.err = nil
		m.operator.catalog = msg.catalog
		m.operator.cursor = min(m.operator.cursor, max(len(msg.catalog.Accounts())-1, 0))
		return m, nil
	case operatorFailedMsg:
		m.operator.loading = false
		m.operator.err = msg.err
		return m, nil
	case operatorAccountCreatedMsg:
		m.operator.running = false
		m.operator.form = nil
		m.operator.err = nil
		m.operator.notice = "account saved to " + msg.artifacts.JWTPath
		if m.operatorServices.Load != nil {
			m.operator.loading = true
			return m, m.operatorServices.Load()
		}
		return m, nil
	case operatorAccountCreateFailedMsg:
		m.operator.running = false
		m.operator.err = msg.err
		return m, nil
	case operatorUserCreatedMsg:
		m.operator.running = false
		m.operator.userForm = nil
		m.operator.err = nil
		m.operator.notice = "credentials saved to " + msg.artifacts.CredentialsPath
		if m.operatorServices.Load != nil {
			m.operator.loading = true
			return m, m.operatorServices.Load()
		}
		return m, nil
	case operatorUserCreateFailedMsg:
		m.operator.running = false
		m.operator.err = msg.err
		return m, nil
	case operatorMutationSucceededMsg:
		m.operator.running = false
		m.operator.form, m.operator.userForm, m.operator.signingForm, m.operator.confirm = nil, nil, nil, nil
		m.operator.err = nil
		m.operator.notice = msg.notice
		if m.operatorServices.Load != nil {
			m.operator.loading = true
			return m, m.operatorServices.Load()
		}
		return m, nil
	case operatorMutationFailedMsg:
		m.operator.running = false
		m.operator.err = msg.err
		return m, nil
	case connectionClosedMsg:
		m.connections.loading = false
		m.connections.err = nil
		m.connections.notice = fmt.Sprintf("closed %s:%d", msg.server, msg.cid)
		if m.connectionServices.Load != nil {
			m.connections.loading = true
			return m, m.connectionServices.Load()
		}
		return m, nil
	case connectionCloseFailedMsg:
		m.connections.loading = false
		m.connections.err = msg.err
		return m, nil
	case alertsLoadedMsg:
		m.alerts.loading, m.alerts.err = false, nil
		m.alerts.records = append([]appalerts.Record(nil), msg.records...)
		m.alerts.cursor = min(m.alerts.cursor, max(len(m.displayedAlerts())-1, 0))
		return m, nil
	case alertsFailedMsg:
		m.alerts.loading, m.alerts.err = false, msg.err
		return m, nil
	case alertWatchStartedMsg:
		m.alerts.watching, m.alerts.events = true, msg.events
		return m, waitAlertEvent(msg.events)
	case alertWatchEventMsg:
		if !m.alerts.paused {
			m.alerts.records = append([]appalerts.Record{msg.record}, m.alerts.records...)
			if len(m.alerts.records) > 1000 {
				m.alerts.records = m.alerts.records[:1000]
			}
		}
		return m, waitAlertEvent(msg.events)
	case alertWatchEndedMsg:
		m.alerts.watching, m.alerts.events = false, nil
		return m, nil
	case alertAckedMsg:
		m.alerts.loading = false
		m.alerts.err = nil
		if msg.scan {
			m.alerts.notice = fmt.Sprintf("health scan: %d new alerts", msg.count)
		} else if msg.all {
			for i := range m.alerts.records {
				m.alerts.records[i].Acknowledged = true
			}
			m.alerts.notice = fmt.Sprintf("acknowledged %d alerts", msg.count)
		} else {
			for i := range m.alerts.records {
				if m.alerts.records[i].ID == msg.id {
					m.alerts.records[i].Acknowledged = true
				}
			}
			m.alerts.notice = "alert acknowledged"
		}
		if msg.scan && m.alertServices.Load != nil {
			m.alerts.loading = true
			return m, m.alertServices.Load(appalerts.Query{Limit: 1000})
		}
		return m, nil
	case alertAckFailedMsg:
		m.alerts.loading = false
		m.alerts.err = msg.err
		return m, nil
	case commandBarLoadedMsg:
		m.commandBar.custom = append([]appcommandbar.Entry(nil), msg.custom...)
		m.commandBar.history = append([]appcommandbar.HistoryView(nil), msg.history...)
		if m.commandBar.open {
			m.commandBar.entries = builtinAndCustomCommands(m)
			m.commandBar.index = appcommandbar.NewIndex(m.commandBar.entries)
		}
		return m, nil
	case commandBarFailedMsg:
		m.commandBar.err = msg.err
		return m, nil
	case commandRecordedMsg:
		if m.commandBarServices.Load != nil {
			return m, m.commandBarServices.Load()
		}
		return m, nil
	case bookmarksLoadedMsg:
		m.bookmarks.loading, m.bookmarks.err = false, nil
		m.bookmarks.items = append([]appbookmarks.View(nil), msg.items...)
		m.bookmarks.cursor = min(m.bookmarks.cursor, max(len(m.bookmarks.items)-1, 0))
		return m, nil
	case bookmarksFailedMsg:
		m.bookmarks.loading = false
		m.bookmarks.err = msg.err
		return m, nil
	case bookmarkAddedMsg:
		m.bookmarks.err = nil
		m.bookmarks.notice = "Bookmarked " + msg.item.Label
		replaced := false
		for i := range m.bookmarks.items {
			if m.bookmarks.items[i].ID == msg.item.ID {
				m.bookmarks.items[i] = msg.item
				replaced = true
				break
			}
		}
		if !replaced {
			m.bookmarks.items = append(m.bookmarks.items, msg.item)
		}
		return m, nil
	case bookmarkRemovedMsg:
		m.bookmarks.err = nil
		m.bookmarks.notice = "Bookmark removed"
		for i, item := range m.bookmarks.items {
			if item.ID == msg.id {
				m.bookmarks.items = append(m.bookmarks.items[:i], m.bookmarks.items[i+1:]...)
				break
			}
		}
		m.bookmarks.cursor = min(m.bookmarks.cursor, max(len(m.bookmarks.items)-1, 0))
		return m, nil
	case recentSearchSavedMsg:
		return m, nil
	}

	switch m.screen {
	case screenWelcome:
		return m.updateWelcome(message)
	case screenConnection:
		return m.updateConnection(message)
	case screenConnecting:
		return m.updateConnecting(message)
	default:
		return m.updateWorkspace(message)
	}
}

func (m Model) updateWelcome(message tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := message.(tea.KeyMsg); ok {
		return m.showConnection()
	}
	return m, nil
}

func (m Model) updateConnection(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			return m.startConnection()
		case "esc":
			return m, tea.Quit
		}
	}

	var command tea.Cmd
	m.serverInput, command = m.serverInput.Update(message)
	return m, command
}

func (m Model) updateConnecting(message tea.Msg) (tea.Model, tea.Cmd) {
	var command tea.Cmd
	m.spinner, command = m.spinner.Update(message)
	return m, command
}

func (m Model) updateWorkspace(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		if m.loadingMessages || m.watchStarting || m.refreshing || m.streamTransferStarting || m.streamTransferActive || m.keyValueEntrySaving || m.loadingKeyValueHistory || m.loadingKeyValueKeys || m.loadingKeyValueEntry || m.keyValueWatchStarting || m.loadingObjects || m.objectWatchStarting || m.objectBucketOperationRunning || m.requestReply.running || m.requestReply.loadingState || m.topology.loading || m.connections.loading || m.operator.loading || m.operator.running || m.alerts.loading || m.bookmarks.loading {
			var command tea.Cmd
			m.spinner, command = m.spinner.Update(message)
			return m, command
		}
		return m, nil
	}
	if m.commandBar.open {
		return m.updateCommandBar(key)
	}
	if m.bookmarks.open {
		return m.updateBookmarks(key)
	}
	if key.String() == "ctrl+k" || (key.String() == ":" && m.activeWorkspace != 7) {
		m.openCommandBar(commandPalette)
		return m, nil
	}
	if m.activeWorkspace == 7 {
		if model, command, handled := m.updateRequestReplyKey(key); handled {
			return model, command
		}
	}
	if m.activeWorkspace == 8 {
		if model, command, handled := m.updateTopologyKey(key); handled {
			return model, command
		}
	}
	if m.activeWorkspace == 9 {
		if model, command, handled := m.updateConnectionsKey(key); handled {
			return model, command
		}
	}
	if m.activeWorkspace == 10 {
		if model, command, handled := m.updateOperatorKey(key); handled {
			return model, command
		}
	}
	if m.activeWorkspace == 11 {
		if model, command, handled := m.updateAlertsKey(key); handled {
			return model, command
		}
	}
	if m.streamTransferActive || m.streamTransferStarting || m.streamTransferCanceling {
		if key.String() == "esc" && !m.streamTransferCanceling {
			if m.stopStreamTransfer != nil {
				m.stopStreamTransfer()
			}
			m.streamTransferCanceling = true
		}
		return m, nil
	}
	if m.streamTransferForm != nil {
		return m.updateStreamTransferForm(key)
	}
	if m.streamTransferErr != nil && key.String() == "esc" {
		m.streamTransferErr = nil
		return m, nil
	}
	if m.objectTransferActive || m.objectTransferStarting || m.objectTransferCanceling {
		if key.String() == "esc" && !m.objectTransferCanceling {
			if m.stopObjectTransfer != nil {
				m.stopObjectTransfer()
			}
			m.objectTransferCanceling = true
		}
		return m, nil
	}
	if m.activeWorkspace == 5 && m.objectTransferForm != nil {
		return m.updateObjectTransferForm(key)
	}
	if m.activeWorkspace == 5 && m.objectEditForm != nil {
		return m.updateObjectEditForm(key)
	}
	if m.activeWorkspace == 5 && m.objectLinkForm != nil {
		return m.updateObjectLinkForm(key)
	}
	if m.activeWorkspace == 5 && m.objectBucketForm != nil {
		return m.updateObjectBucketForm(key)
	}
	if m.activeWorkspace == 5 && m.confirmObjectDelete {
		return m.updateObjectDeleteConfirmation(key)
	}
	if m.activeWorkspace == 5 && m.confirmObjectBucketDelete {
		return m.updateObjectBucketDeleteConfirmation(key)
	}
	if m.activeWorkspace == 5 && m.objectTransferErr != nil && key.String() == "esc" {
		m.objectTransferErr = nil
		return m, nil
	}
	if m.activeWorkspace == 3 && m.searchMode {
		query := m.searchQuery
		updated := m.updateMessageSearch(key)
		if !updated.searchMode && strings.TrimSpace(query) != "" && m.bookmarkServices.SaveSearch != nil {
			return updated, m.bookmarkServices.SaveSearch("messages", query)
		}
		return updated, nil
	}
	if m.activeWorkspace == 6 && m.subjectSearchMode {
		query := m.subjectSearchQuery
		updated := m.updateSubjectSearch(key)
		if !updated.subjectSearchMode && strings.TrimSpace(query) != "" && m.bookmarkServices.SaveSearch != nil {
			return updated, m.bookmarkServices.SaveSearch("subjects", query)
		}
		return updated, nil
	}
	if m.activeWorkspace == 4 && m.keyValueKeySearchMode {
		query := m.keyValueKeyQuery
		updated := m.updateKeyValueKeySearch(key)
		if !updated.keyValueKeySearchMode && strings.TrimSpace(query) != "" && m.bookmarkServices.SaveSearch != nil {
			return updated, m.bookmarkServices.SaveSearch("kv-keys", query)
		}
		return updated, nil
	}
	if m.activeWorkspace == 5 && m.objectSearchMode {
		query := m.objectSearchQuery
		updated := m.updateObjectSearch(key)
		if !updated.objectSearchMode && strings.TrimSpace(query) != "" && m.bookmarkServices.SaveSearch != nil {
			return updated, m.bookmarkServices.SaveSearch("objects", query)
		}
		return updated, nil
	}
	if m.streamForm != nil {
		return m.updateStreamForm(key)
	}
	if m.streamPurgeForm != nil {
		return m.updateStreamPurgeForm(key)
	}
	if m.keyValueForm != nil {
		return m.updateKeyValueForm(key)
	}
	if m.keyValueEntryForm != nil {
		return m.updateKeyValueEntryForm(key)
	}
	if m.confirmStreamDelete {
		return m.updateStreamDeleteConfirmation(key)
	}
	if m.confirmKeyValueDelete {
		return m.updateKeyValueDeleteConfirmation(key)
	}
	if m.pendingKeyValueEntryRemoval != "" {
		return m.updateKeyValueEntryRemovalConfirmation(key)
	}
	if m.pendingKeyValueRestore != nil {
		return m.updateKeyValueRestoreConfirmation(key)
	}
	if m.pendingConsumerAction != 0 {
		return m.updateConsumerConfirmation(key)
	}
	if m.activeWorkspace == 4 && (m.showKeyValueHistory || m.keyValueHistoryErr != nil) {
		return m.updateKeyValueHistoryNavigation(key)
	}
	if key.String() == ":" {
		m.openCommandBar(commandPalette)
		return m, nil
	}
	if key.String() == "ctrl+r" {
		m.openCommandBar(commandHistory)
		return m, nil
	}
	if key.String() == "B" {
		return m.toggleBookmark()
	}
	if key.String() == "b" {
		m.bookmarks.open = true
		m.bookmarks.notice = ""
		if m.bookmarkServices.Load != nil {
			m.bookmarks.loading = true
			return m, m.bookmarkServices.Load(m.bookmarkInventory())
		}
		return m, nil
	}

	switch key.String() {
	case "esc":
		if m.activeWorkspace == 5 && m.showObjectDetail {
			m.showObjectDetail = false
			m.objectDetailOffset = 0
			return m, nil
		}
		if m.activeWorkspace == 5 && m.browsingObjects {
			m.stopActiveObjectWatch()
			m.browsingObjects = false
			m.objectsErr = nil
			m.objectSearchQuery = ""
			return m, nil
		}
		if m.activeWorkspace == 4 && m.keyValueEntryErr != nil {
			m.keyValueEntryErr = nil
			return m, nil
		}
		if m.activeWorkspace == 4 && m.showKeyValueEntry {
			m.showKeyValueEntry = false
			m.keyValueEntryErr = nil
			return m, nil
		}
		if m.activeWorkspace == 4 && m.browsingKeyValue {
			m.browsingKeyValue = false
			m.keyValueKeysErr = nil
			return m, nil
		}
		if m.showStreamDetail {
			m.showStreamDetail = false
		}
		if m.showMessageDetail {
			m.showMessageDetail = false
		}
		if m.showConsumerDetail {
			m.showConsumerDetail = false
		}
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
	case "r":
		if m.activeWorkspace == 5 && m.browsingObjects && m.loadObjects != nil {
			m.loadingObjects = true
			m.objectsErr = nil
			return m, tea.Batch(m.spinner.Tick, m.loadObjects(m.objectBucket))
		}
		return m.startRefresh()
	case "n":
		if m.activeWorkspace == 1 && !m.streamActionRunning {
			form := newCreateStreamForm()
			m.streamForm = &form
			m.streamActionErr = nil
		}
		if m.activeWorkspace == 4 && m.browsingKeyValue && !m.keyValueEntrySaving {
			form := newCreateKeyValueEntryForm(m.keyValueBucket, m.theme)
			m.keyValueEntryForm = &form
			m.keyValueEntryWriteErr = nil
		}
		if m.activeWorkspace == 4 && !m.browsingKeyValue && !m.keyValueCreateRunning {
			form := newKeyValueForm()
			m.keyValueForm = &form
			m.keyValueCreateErr = nil
		}
		if m.activeWorkspace == 5 && !m.browsingObjects && !m.objectBucketOperationRunning {
			form := newObjectBucketForm()
			m.objectBucketForm = &form
			m.objectBucketOperationErr = nil
			m.objectBucketOperationNotice = ""
		}
	case "u":
		if m.activeWorkspace == 5 && m.browsingObjects {
			if m.rejectObjectMirrorMutation() {
				return m, nil
			}
			form := newObjectUploadForm(m.objectBucket)
			m.objectTransferForm = &form
			m.objectTransferErr = nil
			m.objectTransferNotice = ""
		}
	case "D":
		if m.activeWorkspace == 5 && m.browsingObjects && len(m.displayedObjects()) > 0 {
			object := m.displayedObjects()[m.objectObjectCursor]
			form := newObjectDownloadForm(m.objectBucket, object.Name)
			m.objectTransferForm = &form
			m.objectTransferErr = nil
			m.objectTransferNotice = ""
		}
	case "L":
		if m.activeWorkspace == 5 && m.browsingObjects && !m.objectOperationRunning {
			if m.rejectObjectMirrorMutation() {
				return m, nil
			}
			form := newObjectLinkForm(m.objectBucket)
			m.objectLinkForm = &form
			m.objectOperationErr = nil
			m.objectOperationNotice = ""
		}
	case "e":
		if m.activeWorkspace == 1 && len(m.streams) > 0 && !m.streamActionRunning {
			form := newEditStreamForm(m.streams[m.streamCursor])
			m.streamForm = &form
			m.streamActionErr = nil
		}
		if m.activeWorkspace == 4 && m.showKeyValueEntry && m.keyValueEntry != nil && !m.keyValueEntrySaving {
			form := newEditKeyValueEntryForm(*m.keyValueEntry, m.theme)
			m.keyValueEntryForm = &form
			m.keyValueEntryWriteErr = nil
		}
		if m.activeWorkspace == 4 && !m.browsingKeyValue && len(m.keyValueBuckets) > 0 && !m.keyValueCreateRunning {
			form := newEditKeyValueForm(m.keyValueBuckets[m.keyValueCursor])
			m.keyValueForm = &form
			m.keyValueCreateErr = nil
		}
		if m.activeWorkspace == 5 && m.browsingObjects && len(m.displayedObjects()) > 0 && !m.objectOperationRunning {
			if m.rejectObjectMirrorMutation() {
				return m, nil
			}
			form := newObjectEditForm(m.objectBucket, m.displayedObjects()[m.objectObjectCursor])
			m.objectEditForm = &form
			m.objectOperationErr = nil
		}
		if m.activeWorkspace == 5 && !m.browsingObjects && len(m.objectBuckets) > 0 && !m.objectBucketOperationRunning {
			form := newEditObjectBucketForm(m.objectBuckets[m.objectCursor])
			m.objectBucketForm = &form
			m.objectBucketOperationErr = nil
		}
	case "p":
		if m.canOperateConsumer() {
			action := consumers.ActionPause
			if m.consumers[m.consumerCursor].Status == consumers.StatusPaused {
				action = consumers.ActionResume
			}
			return m.runConsumerAction(action)
		}
	case "x":
		if m.canOperateConsumer() {
			m.pendingConsumerAction = consumers.ActionReset
		}
	case "d":
		if m.activeWorkspace == 5 && m.browsingObjects && len(m.displayedObjects()) > 0 && !m.objectOperationRunning {
			if m.rejectObjectMirrorMutation() {
				return m, nil
			}
			m.confirmObjectDelete = true
			m.objectOperationErr = nil
			return m, nil
		}
		if m.activeWorkspace == 5 && !m.browsingObjects && len(m.objectBuckets) > 0 && !m.objectBucketOperationRunning {
			m.confirmObjectBucketDelete = true
			m.objectBucketOperationErr = nil
			return m, nil
		}
		if m.activeWorkspace == 4 && m.showKeyValueEntry && m.keyValueEntry != nil && !m.keyValueEntrySaving {
			m.pendingKeyValueEntryRemoval = kvstore.RemoveDelete
			m.keyValueEntryWriteErr = nil
			return m, nil
		}
		if m.activeWorkspace == 1 && len(m.streams) > 0 && !m.streamActionRunning {
			m.confirmStreamDelete = true
			m.streamActionErr = nil
			break
		}
		if m.canOperateConsumer() {
			m.pendingConsumerAction = consumers.ActionDelete
		}
		if m.activeWorkspace == 4 && !m.browsingKeyValue && len(m.keyValueBuckets) > 0 && !m.keyValueCreateRunning {
			m.confirmKeyValueDelete = true
			m.keyValueCreateErr = nil
		}
	case "P":
		if m.activeWorkspace == 1 && len(m.streams) > 0 && !m.streamActionRunning {
			form := newStreamPurgeForm(m.streams[m.streamCursor])
			m.streamPurgeForm = &form
			m.streamActionErr = nil
			return m, nil
		}
		if m.activeWorkspace == 4 && m.showKeyValueEntry && m.keyValueEntry != nil && !m.keyValueEntrySaving {
			m.pendingKeyValueEntryRemoval = kvstore.RemovePurge
			m.keyValueEntryWriteErr = nil
		}
	case "S":
		if m.activeWorkspace == 1 && len(m.streams) > 0 && !m.streamActionRunning {
			stream := m.streams[m.streamCursor]
			if stream.Storage != streams.StorageFile {
				m.streamTransferErr = fmt.Errorf("only file-backed streams support snapshots")
				return m, nil
			}
			form := newStreamBackupForm(stream.Name)
			m.streamTransferForm = &form
			m.streamTransferErr = nil
			return m, nil
		}
	case "R":
		if m.activeWorkspace == 1 && !m.streamActionRunning {
			form := newStreamRestoreForm("")
			m.streamTransferForm = &form
			m.streamTransferErr = nil
			return m, nil
		}
	case "W":
		if m.activeWorkspace == 5 && m.browsingObjects {
			if m.objectWatching || m.objectWatchStarting {
				m.stopActiveObjectWatch()
				return m, nil
			}
			if m.startObjectWatch != nil {
				m.objectWatchStarting = true
				m.objectWatchReceived = 0
				m.objectWatchErr = nil
				m.objectWatchGeneration++
				return m, m.startObjectWatch(ObjectWatchInput{Generation: m.objectWatchGeneration, Bucket: m.objectBucket})
			}
		}
		if m.activeWorkspace == 4 && m.showKeyValueEntry && m.keyValueEntry != nil {
			if m.keyValueWatching {
				if m.stopKeyValueWatch != nil {
					m.stopKeyValueWatch()
				}
				m.keyValueWatching = false
				return m, nil
			}
			if m.startKeyValueWatch != nil {
				m.keyValueWatchStarting = true
				return m, tea.Batch(m.spinner.Tick, m.startKeyValueWatch(m.keyValueEntry.Bucket, m.keyValueEntry.Key))
			}
		}
	case "C":
		if m.activeWorkspace == 4 && m.showKeyValueEntry && m.keyValueEntry != nil && m.copyValue != nil {
			if err := m.copyValue(string(m.keyValueEntry.Value)); err != nil {
				m.keyValueEntryWriteErr = fmt.Errorf("copy value: %w", err)
			} else {
				m.keyValueCopyNotice = "Value copied to clipboard"
			}
		}
	case "H":
		if m.activeWorkspace == 4 && m.showKeyValueEntry && m.keyValueEntry != nil && m.loadKeyValueHistory != nil {
			m.loadingKeyValueHistory = true
			m.keyValueHistoryErr = nil
			return m, tea.Batch(m.spinner.Tick, m.loadKeyValueHistory(m.keyValueEntry.Bucket, m.keyValueEntry.Key))
		}
	case "/":
		if m.activeWorkspace == 3 && !m.showMessageDetail {
			m.searchMode = true
		}
		if m.activeWorkspace == 6 {
			m.subjectSearchMode = true
		}
		if m.activeWorkspace == 4 && m.browsingKeyValue {
			m.keyValueKeySearchMode = true
		}
		if m.activeWorkspace == 5 && m.browsingObjects {
			m.objectSearchMode = true
		}
	case "w":
		if m.activeWorkspace == 3 && !m.showMessageDetail {
			return m.toggleMessageWatch()
		}
		if m.activeWorkspace == 6 {
			return m.startSelectedSubjectWatch()
		}
	case " ":
		if m.activeWorkspace == 3 && m.watching {
			m.watchPaused = !m.watchPaused
		}
		if m.activeWorkspace == 5 && m.objectWatching {
			m.objectWatchPaused = !m.objectWatchPaused
			if !m.objectWatchPaused && m.loadObjects != nil {
				m.loadingObjects = true
				return m, m.loadObjects(m.objectBucket)
			}
		}
	case "right", "l", "tab":
		m.activeWorkspace = (m.activeWorkspace + 1) % len(workspaces)
		if m.activeWorkspace == 8 && len(m.topology.snapshot.Servers()) == 0 {
			return m.startTopologyLoad()
		}
		if m.activeWorkspace == 9 && m.connections.snapshot.ObservedAt.IsZero() {
			return m.startConnectionsLoad()
		}
		if m.activeWorkspace == 10 && len(m.operator.catalog.Accounts()) == 0 {
			return m.startOperatorLoad()
		}
		if m.activeWorkspace == 11 && len(m.alerts.records) == 0 {
			return m.startAlerts()
		}
	case "left", "h", "shift+tab":
		m.activeWorkspace = (m.activeWorkspace - 1 + len(workspaces)) % len(workspaces)
		if m.activeWorkspace == 8 && len(m.topology.snapshot.Servers()) == 0 {
			return m.startTopologyLoad()
		}
		if m.activeWorkspace == 9 && m.connections.snapshot.ObservedAt.IsZero() {
			return m.startConnectionsLoad()
		}
		if m.activeWorkspace == 10 && len(m.operator.catalog.Accounts()) == 0 {
			return m.startOperatorLoad()
		}
		if m.activeWorkspace == 11 && len(m.alerts.records) == 0 {
			return m.startAlerts()
		}
	case "c":
		return m.showConnection()
	case "down", "j":
		if m.activeWorkspace == 5 && m.showObjectDetail {
			m.objectDetailOffset++
			return m, nil
		}
		if m.showKeyValueEntry {
			m.keyValueEntryOffset++
			return m, nil
		}
		if m.showMessageDetail {
			m.messageDetailOffset++
			return m, nil
		}
		if m.activeWorkspace == 1 && m.streamCursor < len(m.streams)-1 {
			m.streamCursor++
		}
		if m.activeWorkspace == 2 && m.consumerCursor < len(m.consumers)-1 {
			m.consumerCursor++
		}
		if m.activeWorkspace == 3 && m.messageCursor < len(m.displayedMessages())-1 {
			m.messageCursor++
		}
		if m.activeWorkspace == 4 && !m.browsingKeyValue && m.keyValueCursor < len(m.keyValueBuckets)-1 {
			m.keyValueCursor++
		}
		if m.activeWorkspace == 4 && m.browsingKeyValue && m.keyValueKeyCursor < len(m.displayedKeyValueKeys())-1 {
			m.keyValueKeyCursor++
		}
		if m.activeWorkspace == 5 {
			if m.browsingObjects && m.objectObjectCursor < len(m.displayedObjects())-1 {
				m.objectObjectCursor++
			} else if !m.browsingObjects && m.objectCursor < len(m.objectBuckets)-1 {
				m.objectCursor++
			}
		}
		if m.activeWorkspace == 6 && m.subjectCursor < len(m.displayedSubjects())-1 {
			m.subjectCursor++
		}
	case "up", "k":
		if m.activeWorkspace == 5 && m.showObjectDetail {
			m.objectDetailOffset = max(m.objectDetailOffset-1, 0)
			return m, nil
		}
		if m.showKeyValueEntry {
			m.keyValueEntryOffset = max(m.keyValueEntryOffset-1, 0)
			return m, nil
		}
		if m.showMessageDetail {
			m.messageDetailOffset = max(m.messageDetailOffset-1, 0)
			return m, nil
		}
		if m.activeWorkspace == 1 && m.streamCursor > 0 {
			m.streamCursor--
		}
		if m.activeWorkspace == 2 && m.consumerCursor > 0 {
			m.consumerCursor--
		}
		if m.activeWorkspace == 3 && m.messageCursor > 0 {
			m.messageCursor--
		}
		if m.activeWorkspace == 4 && !m.browsingKeyValue && m.keyValueCursor > 0 {
			m.keyValueCursor--
		}
		if m.activeWorkspace == 4 && m.browsingKeyValue && m.keyValueKeyCursor > 0 {
			m.keyValueKeyCursor--
		}
		if m.activeWorkspace == 5 {
			if m.browsingObjects && m.objectObjectCursor > 0 {
				m.objectObjectCursor--
			} else if !m.browsingObjects && m.objectCursor > 0 {
				m.objectCursor--
			}
		}
		if m.activeWorkspace == 6 && m.subjectCursor > 0 {
			m.subjectCursor--
		}
	case "pgdown", "ctrl+d":
		if m.activeWorkspace == 5 && m.showObjectDetail {
			m.objectDetailOffset += max(m.height/2, 1)
			return m, nil
		}
		if m.showKeyValueEntry {
			m.keyValueEntryOffset += max(m.height/2, 1)
			return m, nil
		}
		if m.showMessageDetail {
			m.messageDetailOffset += max(m.height/2, 1)
			return m, nil
		}
		if m.activeWorkspace == 3 {
			m.messageCursor = min(m.messageCursor+max(m.height/2, 1), max(len(m.displayedMessages())-1, 0))
		}
		if m.activeWorkspace == 2 {
			m.consumerCursor = min(m.consumerCursor+max(m.height/2, 1), max(len(m.consumers)-1, 0))
		}
		if m.activeWorkspace == 6 {
			m.subjectCursor = min(m.subjectCursor+max(m.height/2, 1), max(len(m.displayedSubjects())-1, 0))
		}
		if m.activeWorkspace == 4 {
			if m.browsingKeyValue {
				m.keyValueKeyCursor = min(m.keyValueKeyCursor+max(m.height/2, 1), max(len(m.displayedKeyValueKeys())-1, 0))
			} else {
				m.keyValueCursor = min(m.keyValueCursor+max(m.height/2, 1), max(len(m.keyValueBuckets)-1, 0))
			}
		}
		if m.activeWorkspace == 5 {
			if m.browsingObjects {
				m.objectObjectCursor = min(m.objectObjectCursor+max(m.height/2, 1), max(len(m.displayedObjects())-1, 0))
			} else {
				m.objectCursor = min(m.objectCursor+max(m.height/2, 1), max(len(m.objectBuckets)-1, 0))
			}
		}
	case "pgup", "ctrl+u":
		if m.activeWorkspace == 5 && m.showObjectDetail {
			m.objectDetailOffset = max(m.objectDetailOffset-max(m.height/2, 1), 0)
			return m, nil
		}
		if m.showKeyValueEntry {
			m.keyValueEntryOffset = max(m.keyValueEntryOffset-max(m.height/2, 1), 0)
			return m, nil
		}
		if m.showMessageDetail {
			m.messageDetailOffset = max(m.messageDetailOffset-max(m.height/2, 1), 0)
			return m, nil
		}
		if m.activeWorkspace == 3 {
			m.messageCursor = max(m.messageCursor-max(m.height/2, 1), 0)
		}
		if m.activeWorkspace == 2 {
			m.consumerCursor = max(m.consumerCursor-max(m.height/2, 1), 0)
		}
		if m.activeWorkspace == 6 {
			m.subjectCursor = max(m.subjectCursor-max(m.height/2, 1), 0)
		}
		if m.activeWorkspace == 4 {
			if m.browsingKeyValue {
				m.keyValueKeyCursor = max(m.keyValueKeyCursor-max(m.height/2, 1), 0)
			} else {
				m.keyValueCursor = max(m.keyValueCursor-max(m.height/2, 1), 0)
			}
		}
		if m.activeWorkspace == 5 {
			if m.browsingObjects {
				m.objectObjectCursor = max(m.objectObjectCursor-max(m.height/2, 1), 0)
			} else {
				m.objectCursor = max(m.objectCursor-max(m.height/2, 1), 0)
			}
		}
	case "home", "g":
		if m.activeWorkspace == 5 && m.showObjectDetail {
			m.objectDetailOffset = 0
			return m, nil
		}
		if m.showKeyValueEntry {
			m.keyValueEntryOffset = 0
			return m, nil
		}
		if m.showMessageDetail {
			m.messageDetailOffset = 0
			return m, nil
		}
		if m.activeWorkspace == 3 {
			m.messageCursor = 0
		}
		if m.activeWorkspace == 2 {
			m.consumerCursor = 0
		}
		if m.activeWorkspace == 6 {
			m.subjectCursor = 0
		}
		if m.activeWorkspace == 4 {
			if m.browsingKeyValue {
				m.keyValueKeyCursor = 0
			} else {
				m.keyValueCursor = 0
			}
		}
		if m.activeWorkspace == 5 {
			if m.browsingObjects {
				m.objectObjectCursor = 0
			} else {
				m.objectCursor = 0
			}
		}
	case "end", "G":
		if m.activeWorkspace == 3 && !m.showMessageDetail {
			m.messageCursor = max(len(m.displayedMessages())-1, 0)
		}
		if m.activeWorkspace == 2 && !m.showConsumerDetail {
			m.consumerCursor = max(len(m.consumers)-1, 0)
		}
		if m.activeWorkspace == 6 {
			m.subjectCursor = max(len(m.displayedSubjects())-1, 0)
		}
		if m.activeWorkspace == 4 {
			if m.browsingKeyValue {
				m.keyValueKeyCursor = max(len(m.displayedKeyValueKeys())-1, 0)
			} else {
				m.keyValueCursor = max(len(m.keyValueBuckets)-1, 0)
			}
		}
		if m.activeWorkspace == 5 {
			if m.browsingObjects {
				m.objectObjectCursor = max(len(m.displayedObjects())-1, 0)
			} else {
				m.objectCursor = max(len(m.objectBuckets)-1, 0)
			}
		}
	case "enter":
		if m.activeWorkspace == 5 && m.browsingObjects && len(m.displayedObjects()) > 0 {
			m.showObjectDetail = true
			m.objectDetailOffset = 0
			return m, nil
		}
		if m.activeWorkspace == 5 && !m.browsingObjects && len(m.objectBuckets) > 0 && m.loadObjects != nil {
			bucket := m.objectBuckets[m.objectCursor].Name
			m.loadingObjects = true
			m.objectsErr = nil
			return m, tea.Batch(m.spinner.Tick, m.loadObjects(bucket))
		}
		if m.activeWorkspace == 4 && m.browsingKeyValue && len(m.displayedKeyValueKeys()) > 0 && m.loadKeyValueEntry != nil {
			key := m.displayedKeyValueKeys()[m.keyValueKeyCursor]
			m.loadingKeyValueEntry = true
			m.keyValueEntryErr = nil
			return m, tea.Batch(m.spinner.Tick, m.loadKeyValueEntry(m.keyValueBucket, key))
		}
		if m.activeWorkspace == 4 && !m.browsingKeyValue && len(m.keyValueBuckets) > 0 && m.loadKeyValueKeys != nil {
			bucket := m.keyValueBuckets[m.keyValueCursor].Name
			m.loadingKeyValueKeys = true
			m.keyValueKeysErr = nil
			return m, tea.Batch(m.spinner.Tick, m.loadKeyValueKeys(bucket))
		}
		if m.activeWorkspace == 1 && len(m.streams) > 0 {
			m.showStreamDetail = true
		}
		if m.activeWorkspace == 3 && len(m.displayedMessages()) > 0 {
			m.showMessageDetail = true
			m.messageDetailOffset = 0
		}
		if m.activeWorkspace == 2 && len(m.consumers) > 0 {
			m.showConsumerDetail = true
		}
		if m.activeWorkspace == 6 {
			return m.startSelectedSubjectWatch()
		}
	case "m":
		if m.activeWorkspace == 1 && len(m.streams) > 0 && m.loadMessages != nil {
			m.loadingMessages = true
			m.messageErr = nil
			return m, tea.Batch(m.spinner.Tick, m.loadMessages(m.streams[m.streamCursor].Name))
		}
	default:
		before := m.activeWorkspace
		m.selectWorkspace(key.String())
		if before != m.activeWorkspace && m.activeWorkspace == 8 && len(m.topology.snapshot.Servers()) == 0 {
			return m.startTopologyLoad()
		}
		if before != m.activeWorkspace && m.activeWorkspace == 9 && m.connections.snapshot.ObservedAt.IsZero() {
			return m.startConnectionsLoad()
		}
		if before != m.activeWorkspace && m.activeWorkspace == 10 && len(m.operator.catalog.Accounts()) == 0 {
			return m.startOperatorLoad()
		}
	}
	return m, nil
}

func (m Model) canOperateConsumer() bool {
	return m.activeWorkspace == 2 && len(m.consumers) > 0 && !m.consumerActionRunning
}

func waitForKeyValueEvent(events <-chan kvstore.Entry) tea.Cmd {
	return func() tea.Msg {
		entry, ok := <-events
		if !ok {
			return keyValueWatchEndedMsg{}
		}
		return keyValueWatchEventMsg{entry: entry, events: events}
	}
}

func (m Model) updateConsumerConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "enter":
		action := m.pendingConsumerAction
		m.pendingConsumerAction = 0
		return m.runConsumerAction(action)
	case "n", "esc":
		m.pendingConsumerAction = 0
	}
	return m, nil
}

func (m Model) runConsumerAction(action consumers.Action) (tea.Model, tea.Cmd) {
	if m.consumerAction == nil || len(m.consumers) == 0 {
		m.consumerActionErr = errors.New("consumer operations are unavailable")
		return m, nil
	}
	consumer := m.consumers[m.consumerCursor]
	input := ConsumerActionInput{Generation: m.refreshGeneration, Stream: consumer.Stream, Name: consumer.Name, Action: action}
	m.consumerActionRunning = true
	m.consumerActionErr = nil
	m.consumerActionNotice = ""
	return m, m.consumerAction(input)
}

func (m *Model) applyConsumerAction(input ConsumerActionInput) {
	if m.consumerCursor < 0 || m.consumerCursor >= len(m.consumers) {
		return
	}
	switch input.Action {
	case consumers.ActionPause:
		m.consumers[m.consumerCursor].Status = consumers.StatusPaused
	case consumers.ActionResume:
		m.consumers[m.consumerCursor].Status = consumers.StatusIdle
	case consumers.ActionDelete:
		m.consumers = append(m.consumers[:m.consumerCursor], m.consumers[m.consumerCursor+1:]...)
		m.consumerCursor = min(m.consumerCursor, max(len(m.consumers)-1, 0))
	}
}

func (m Model) updateMessageSearch(key tea.KeyMsg) Model {
	switch key.String() {
	case "esc", "enter":
		m.searchMode = false
	case "backspace":
		runes := []rune(m.searchQuery)
		if len(runes) > 0 {
			m.searchQuery = string(runes[:len(runes)-1])
			m.applyMessageSearch()
		}
	case "ctrl+u":
		m.searchQuery = ""
		m.applyMessageSearch()
	default:
		if key.Type == tea.KeyRunes {
			m.searchQuery += string(key.Runes)
			m.applyMessageSearch()
		}
	}
	return m
}

func (m *Model) setMessages(records []messages.Record) {
	m.messages = append([]messages.Record(nil), records...)
	m.messageSearch = messages.NewSearchIndex(m.messages)
	m.applyMessageSearch()
}

func (m *Model) applyMessageSearch() {
	m.messageCursor = 0
	if strings.TrimSpace(m.searchQuery) == "" {
		m.filteredMessages = nil
		return
	}
	m.filteredMessages = m.messageSearch.SearchInto(m.filteredMessages, m.searchQuery)
}

func (m Model) displayedMessages() []messages.Record {
	if strings.TrimSpace(m.searchQuery) == "" {
		return m.messages
	}
	return m.filteredMessages
}

func (m Model) showConnection() (tea.Model, tea.Cmd) {
	if m.stopWatch != nil {
		m.stopWatch()
	}
	m.watching = false
	m.watchStarting = false
	if m.stopKeyValueWatch != nil {
		m.stopKeyValueWatch()
	}
	m.keyValueWatching = false
	m.keyValueWatchStarting = false
	m.stopActiveObjectWatch()
	m.refreshGeneration++
	m.refreshing = false
	m.screen = screenConnection
	m.serverInput.Focus()
	return m, textinput.Blink
}

func (m *Model) stopActiveObjectWatch() {
	wasActive := m.objectWatching || m.objectWatchStarting
	if (m.objectWatching || m.objectWatchStarting) && m.stopObjectWatch != nil {
		m.stopObjectWatch()
	}
	if wasActive {
		m.objectWatchGeneration++
	}
	m.objectWatching = false
	m.objectWatchStarting = false
	m.objectWatchPaused = false
	m.objectWatchEvents = nil
}

func (m Model) startRefresh() (tea.Model, tea.Cmd) {
	if m.refresh == nil || m.refreshing || m.screen != screenWorkspace {
		return m, nil
	}
	m.refreshing = true
	return m, m.refresh(m.refreshGeneration)
}

func (m Model) scheduleRefresh(generation uint64, delay time.Duration) tea.Cmd {
	return m.runtime.Tick(delay, func(time.Time) tea.Msg { return refreshTickMsg{generation: generation} })
}

func (m *Model) applyRefresh(msg refreshedMsg) {
	selectedSubject := selectedSubjectName(m.displayedSubjects(), m.subjectCursor)
	selectedStream := selectedStreamName(m.streams, m.streamCursor)
	selectedConsumer := selectedConsumerID(m.consumers, m.consumerCursor)
	selectedBucket := selectedBucketName(m.keyValueBuckets, m.keyValueCursor)
	m.snapshot = msg.snapshot
	m.streams = append(m.streams[:0], msg.streams...)
	m.consumers = append(m.consumers[:0], msg.consumers...)
	m.keyValueBuckets = append(m.keyValueBuckets[:0], msg.buckets...)
	m.objectBuckets = append(m.objectBuckets[:0], msg.objects...)
	m.streamCursor = findStream(m.streams, selectedStream)
	m.consumerCursor = findConsumer(m.consumers, selectedConsumer)
	m.keyValueCursor = findBucket(m.keyValueBuckets, selectedBucket)
	m.rebuildSubjects()
	m.subjectCursor = findSubject(m.displayedSubjects(), selectedSubject)
	m.refreshing = false
	m.refreshErr = nil
}

func selectedStreamName(streamCatalog []streams.Summary, cursor int) string {
	if cursor < 0 || cursor >= len(streamCatalog) {
		return ""
	}
	return streamCatalog[cursor].Name
}

type consumerID struct{ stream, name string }

func selectedConsumerID(consumerCatalog []consumers.Summary, cursor int) consumerID {
	if cursor < 0 || cursor >= len(consumerCatalog) {
		return consumerID{}
	}
	return consumerID{stream: consumerCatalog[cursor].Stream, name: consumerCatalog[cursor].Name}
}

func findStream(streamCatalog []streams.Summary, name string) int {
	for index, stream := range streamCatalog {
		if stream.Name == name {
			return index
		}
	}
	return 0
}

func findConsumer(consumerCatalog []consumers.Summary, id consumerID) int {
	for index, consumer := range consumerCatalog {
		if consumer.Stream == id.stream && consumer.Name == id.name {
			return index
		}
	}
	return 0
}

func (m Model) toggleMessageWatch() (tea.Model, tea.Cmd) {
	if m.watching || m.watchStarting {
		if m.stopWatch != nil {
			m.stopWatch()
		}
		m.watching = false
		m.watchStarting = false
		m.watchPaused = false
		return m, nil
	}
	if m.startWatch == nil || m.messageStream == "" {
		m.messageErr = errors.New("select a stream before starting live watch")
		return m, nil
	}
	patterns := append([]string(nil), m.watchSubjects...)
	if len(patterns) == 0 {
		patterns = m.subjectsForStream(m.messageStream)
	}
	if len(patterns) == 0 {
		m.messageErr = errors.New("selected stream has no configured subjects")
		return m, nil
	}
	m.messageErr = nil
	m.watchStarting = true
	return m, tea.Batch(m.spinner.Tick, m.startWatch(m.messageStream, patterns))
}

func (m Model) subjectsForStream(name string) []string {
	for _, stream := range m.streams {
		if stream.Name == name {
			return append([]string(nil), stream.Subjects...)
		}
	}
	return nil
}

func (m Model) waitForWatchBatch(events <-chan messages.Record) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-events
		if !ok {
			return watchEndedMsg{}
		}
		timer := m.runtime.After(watchBatchInterval)
		batch := []messages.Record{first}
		for len(batch) < watchBatchLimit {
			select {
			case record, open := <-events:
				if !open {
					return watchBatchMsg{records: batch}
				}
				batch = append(batch, record)
			case <-timer:
				return watchBatchMsg{records: batch}
			}
		}
		return watchBatchMsg{records: batch}
	}
}

func (m *Model) prependLiveMessages(records []messages.Record) {
	if len(records) == 0 {
		return
	}
	before := m.displayedMessages()
	var selected messages.Record
	keepSelection := m.messageCursor > 0 && m.messageCursor < len(before)
	if keepSelection {
		selected = before[m.messageCursor]
	}
	m.messageSearch.PrependBatch(records, messageBufferLimit)
	m.messages = m.messageSearch.SearchInto(m.messages, "")
	if strings.TrimSpace(m.searchQuery) != "" {
		m.filteredMessages = m.messageSearch.SearchInto(m.filteredMessages, m.searchQuery)
	}
	if keepSelection {
		m.messageCursor = findMessage(m.displayedMessages(), selected)
	}
}

func findMessage(records []messages.Record, selected messages.Record) int {
	for index, record := range records {
		if record.Sequence == selected.Sequence && record.Subject == selected.Subject && record.Time.Equal(selected.Time) {
			return index
		}
	}
	return 0
}

func (m Model) startConnection() (tea.Model, tea.Cmd) {
	server := strings.TrimSpace(m.serverInput.Value())
	if server == "" {
		m.err = errors.New("server URL is required")
		return m, nil
	}
	if m.connect == nil {
		m.err = errors.New("connection service is unavailable")
		return m, nil
	}

	command := m.withMinimumDuration(
		m.connect(ConnectionInput{Profile: m.profile, Server: server}),
		minimumConnectionDisplay,
	)
	m.err = nil
	m.screen = screenConnecting
	m.serverInput.Blur()
	return m, tea.Batch(m.spinner.Tick, command)
}

func (m Model) withMinimumDuration(command tea.Cmd, duration time.Duration) tea.Cmd {
	return func() tea.Msg {
		startedAt := m.runtime.Now()
		message := command()
		remaining := duration - m.runtime.Now().Sub(startedAt)
		if remaining > 0 {
			<-m.runtime.After(remaining)
		}
		return message
	}
}

func (m *Model) selectWorkspace(key string) {
	if key == "!" && len(workspaces) >= 12 {
		m.activeWorkspace = 11
		return
	}
	if key == "a" && len(workspaces) >= 11 {
		m.activeWorkspace = 10
		return
	}
	if key == "0" && len(workspaces) >= 10 {
		m.activeWorkspace = 9
		return
	}
	if len(key) != 1 || key[0] < '1' || key[0] > byte('0'+len(workspaces)) {
		return
	}
	m.activeWorkspace = int(key[0] - '1')
}

func (m Model) View() string {
	width, height := m.viewport()
	switch m.screen {
	case screenWelcome:
		return m.welcomeView(width, height)
	case screenConnection:
		return m.connectionView(width, height)
	case screenConnecting:
		return m.connectingView(width, height)
	default:
		return m.workspaceView(width, height)
	}
}

func (m Model) viewport() (int, int) {
	if m.width <= 0 || m.height <= 0 {
		return 80, 24
	}
	return m.width, m.height
}

func (m Model) welcomeView(width, height int) string {
	logo := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.accent).
		Render("NATS TUI")
	tagline := lipgloss.NewStyle().
		Foreground(m.theme.secondary).
		Render("JetStream, beautifully operational.")
	hint := lipgloss.NewStyle().
		Foreground(m.theme.muted).
		Render("Starting workspace…  press any key")
	content := logo + "\n\n" + tagline + "\n\n\n" + hint
	return m.centeredCanvas(width, height, content)
}

func (m Model) connectionView(width, height int) string {
	cardWidth := min(max(width-10, 34), 76)
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.primary).
		Render("Connect to NATS")
	subtitle := lipgloss.NewStyle().
		Foreground(m.theme.secondary).
		Render("Paste a server URL. Authentication is optional.")
	label := lipgloss.NewStyle().
		Foreground(m.theme.muted).
		Render("Server URL")
	auth := lipgloss.NewStyle().
		Foreground(m.theme.mint).
		Render("● No authentication")
	actions := lipgloss.NewStyle().
		Foreground(m.theme.muted).
		Render("enter connect   esc quit")

	parts := []string{title, subtitle, "", label, m.serverInputView(), "", auth}
	if m.err != nil {
		parts = append(parts, "", lipgloss.NewStyle().Foreground(m.theme.red).Render("Connection failed: "+m.err.Error()))
	}
	parts = append(parts, "", actions)

	card := lipgloss.NewStyle().
		Width(max(cardWidth-2, 1)).
		Padding(2, 3).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.border).
		Render(strings.Join(parts, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) connectingView(width, height int) string {
	server := lipgloss.NewStyle().
		Foreground(m.theme.secondary).
		Render(m.serverInput.Value())
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.primary).
		Render(m.spinner.View() + " Connecting")
	hint := lipgloss.NewStyle().
		Foreground(m.theme.muted).
		Render("Opening NATS and JetStream contexts…")
	return m.centeredCanvas(width, height, title+"\n\n"+server+"\n\n"+hint)
}

func (m Model) serverInputView() string {
	value := []rune(m.serverInput.Value())
	position := min(max(m.serverInput.Position(), 0), len(value))
	before := string(value[:position])
	after := string(value[position:])
	caret := "▏"
	if position < len(value) {
		caret = lipgloss.NewStyle().
			Foreground(m.theme.accent).
			Underline(true).
			Render(string(value[position]))
		after = string(value[position+1:])
	}

	prompt := lipgloss.NewStyle().Foreground(m.theme.accent).Render("› ")
	return prompt + before + caret + after
}

func (m Model) centeredCanvas(width, height int, content string) string {
	placed := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Foreground(m.theme.primary).
		Render(placed)
}

func (m Model) workspaceView(width, height int) string {
	if m.bookmarks.open {
		return m.bookmarksView(width, height)
	}
	if m.commandBar.open {
		return m.commandBarView(width, height)
	}
	if m.streamTransferForm != nil || m.streamTransferStarting || m.streamTransferActive || m.streamTransferCanceling || m.streamTransferErr != nil {
		return m.streamTransferView(width, height)
	}
	if m.keyValueEntryForm != nil {
		return m.keyValueEntryFormView(width, height)
	}
	if m.keyValueForm != nil {
		return m.keyValueFormView(width, height)
	}
	if m.streamForm != nil {
		return m.streamFormView(width, height)
	}
	if m.streamPurgeForm != nil {
		return m.streamPurgeFormView(width, height)
	}
	if m.confirmStreamDelete && len(m.streams) > 0 {
		return m.streamDeleteConfirmationView(width, height)
	}
	if m.confirmKeyValueDelete && len(m.keyValueBuckets) > 0 {
		return m.keyValueDeleteConfirmationView(width, height)
	}
	if m.pendingKeyValueEntryRemoval != "" && m.keyValueEntry != nil {
		return m.keyValueEntryRemovalConfirmationView(width, height)
	}
	if m.pendingKeyValueRestore != nil {
		return m.keyValueRestoreConfirmationView(width, height)
	}
	if m.showHelp {
		return m.helpView(width, height)
	}
	if m.pendingConsumerAction != 0 {
		return m.consumerConfirmationView(width, height)
	}

	contentWidth := max(width-4, 1)
	contentHeight := max(height-2, 1)
	header := m.header(contentWidth)
	tabs := m.workspaceTabs(contentWidth)
	contentAvailable := max(contentHeight-lipgloss.Height(header)-lipgloss.Height(tabs)-4, 1)
	content := m.workspaceContent(contentWidth, contentAvailable)
	footer := lipgloss.NewStyle().
		Foreground(m.theme.muted).
		Render(workspaceFooter(contentWidth))

	top := strings.Join([]string{header, tabs, "", content}, "\n")
	availableGap := max(contentHeight-lipgloss.Height(top)-lipgloss.Height(footer), 0)
	body := top + strings.Repeat("\n", availableGap) + footer

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Foreground(m.theme.primary).
		Padding(1, 2).
		Render(body)
}

func workspaceFooter(width int) string {
	switch {
	case width >= 104:
		return ": commands   B save   b bookmarks   ←/→ navigate   r refresh   c profiles   ? help   q quit"
	case width >= 66:
		return ": cmd   B save   b list   ←/→ navigate   r refresh   ? help   q quit"
	default:
		return ": cmd   B save   b list   ? help   q quit"
	}
}

func (m Model) consumerConfirmationView(width, height int) string {
	if m.consumerCursor < 0 || m.consumerCursor >= len(m.consumers) {
		return m.centeredCanvas(width, height, "Consumer is no longer available")
	}
	consumer := m.consumers[m.consumerCursor]
	action := m.pendingConsumerAction
	title := strings.ToUpper(action.String()[:1]) + action.String()[1:] + " consumer?"
	color := m.theme.yellow
	detail := "The consumer delivery state will be reset to ack floor + 1."
	if action == consumers.ActionDelete {
		color = m.theme.red
		detail = "This removes the consumer and its delivery state. This cannot be undone."
	}
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(color).Render(title),
		"",
		lipgloss.NewStyle().Foreground(m.theme.primary).Render(consumer.Stream + " / " + consumer.Name),
		"",
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(detail),
		"",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("y confirm   n/esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().
		Width(min(max(width-12, 32), 76)).
		Padding(2, 3).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Render(content)
	return m.centeredCanvas(width, height, card)
}

func (m Model) header(width int) string {
	status := m.connectionStatusView()
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render("NATS")
	profile := lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.profile)
	left := fmt.Sprintf("%s  /  %s", title, profile)
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(status), 1)
	return lipgloss.NewStyle().
		Width(width).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(m.theme.border).
		Render(left + strings.Repeat(" ", gap) + status)
}

func (m Model) connectionStatusView() string {
	switch {
	case m.refreshErr != nil:
		return lipgloss.NewStyle().Foreground(m.theme.red).Render("● Stale · retrying")
	case m.refreshing:
		return lipgloss.NewStyle().Foreground(m.theme.yellow).Render("↻ Refreshing")
	default:
		return lipgloss.NewStyle().Foreground(m.theme.mint).Render("● Connected")
	}
}

type workspace struct {
	tab         string
	short       string
	title       string
	description string
}

var workspaces = []workspace{
	{tab: "Dashboard", short: "Dash", title: "Dashboard"},
	{tab: "Streams", short: "Str", title: "Stream Management", description: "Inspect, create and operate JetStream streams."},
	{tab: "Consumers", short: "Con", title: "Consumer Management", description: "Monitor consumer health, pending messages and lag."},
	{tab: "Messages", short: "Msg", title: "Message Monitor", description: "Browse message history and watch subjects live."},
	{tab: "KV", short: "KV", title: "Key-Value Stores", description: "Explore buckets, entries and revision history."},
	{tab: "Objects", short: "Obj", title: "Object Stores", description: "Manage buckets, metadata and object transfers."},
	{tab: "Subjects", short: "Sub", title: "Subject Explorer", description: "Discover configured and observed subject traffic."},
	{tab: "Publish", short: "Pub", title: "Publish / Request Lab", description: "Publish messages, test request/reply and replay saved exchanges."},
	{tab: "Topology", short: "Topo", title: "Server Topology", description: "Inspect servers, links, JetStream resources and cluster health."},
	{tab: "Connections", short: "Conn", title: "Connections & Subscriptions", description: "Inspect live clients, subscriptions, traffic, pending bytes and slow consumers."},
	{tab: "Accounts", short: "Acc", title: "Accounts & Users", description: "Inspect operator accounts, claims, limits, permissions, imports and exports."},
	{tab: "Alerts", short: "!", title: "Alerts & Advisories", description: "Watch advisories, health thresholds and acknowledged incidents."},
}

func (m Model) workspaceTabs(width int) string {
	tabs := make([]string, 0, len(workspaces))
	for index, item := range workspaces {
		label := item.tab
		style := lipgloss.NewStyle().Padding(0, 1).Foreground(m.theme.muted)
		if width < 160 {
			label = item.short
			style = style.Padding(0)
		}
		if index == m.activeWorkspace {
			style = style.Foreground(m.theme.accent).Bold(true).Underline(true)
		}
		shortcut := fmt.Sprintf("%d", index+1)
		if index == 10 {
			shortcut = "A"
		}
		if index == 11 {
			shortcut = "!"
		}
		tabs = append(tabs, style.Render(shortcut+" "+label))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(tabs, "  "))
}

func (m Model) workspaceContent(width, height int) string {
	if m.activeWorkspace == 0 {
		return m.dashboardView(width)
	}
	if m.activeWorkspace == 1 {
		return m.streamsView(width)
	}
	if m.activeWorkspace == 2 {
		return m.consumerView(width, height)
	}
	if m.activeWorkspace == 3 {
		return m.messagesView(width, height)
	}
	if m.activeWorkspace == 4 {
		return m.keyValueView(width, height)
	}
	if m.activeWorkspace == 5 {
		return m.objectStoreView(width, height)
	}
	if m.activeWorkspace == 6 {
		return m.subjectsView(width, height)
	}
	if m.activeWorkspace == 7 {
		return m.requestReplyView(width, height)
	}
	if m.activeWorkspace == 8 {
		return m.topologyView(width, height)
	}
	if m.activeWorkspace == 9 {
		return m.connectionsView(width, height)
	}
	if m.activeWorkspace == 10 {
		return m.operatorView(width, height)
	}
	if m.activeWorkspace == 11 {
		return m.alertsView(width, height)
	}
	item := workspaces[m.activeWorkspace]
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(item.title)
	description := lipgloss.NewStyle().Foreground(m.theme.secondary).Render(item.description)
	comingSoon := lipgloss.NewStyle().Foreground(m.theme.accent).Render("Workspace ready for its first vertical slice")
	return title + "\n\n" + description + "\n\n" + comingSoon
}

func (m Model) consumerView(width, height int) string {
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Dashboard  ›  Consumers")
	feedback := m.consumerActionFeedback()
	if len(m.consumers) == 0 {
		return heading + feedback + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No consumers found in this JetStream account.")
	}
	if m.showConsumerDetail {
		return heading + feedback + "\n\n" + m.consumerDetail(width, m.consumers[m.consumerCursor], true)
	}

	paneHeight := max(height-2, 3)
	if width < 110 {
		return heading + feedback + "\n\n" + m.consumerListPane(width, paneHeight)
	}
	leftWidth := width * 58 / 100
	rightWidth := max(width-leftWidth-3, 24)
	list := m.consumerListPane(leftWidth, paneHeight)
	detail := m.consumerDetail(rightWidth, m.consumers[m.consumerCursor], false)
	dividerHeight := max(lipgloss.Height(list), lipgloss.Height(detail))
	divider := lipgloss.NewStyle().Foreground(m.theme.border).Render(strings.TrimSuffix(strings.Repeat("│\n", dividerHeight), "\n"))
	return heading + feedback + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, list, " ", divider, " ", detail)
}

func (m Model) consumerActionFeedback() string {
	switch {
	case m.consumerActionRunning:
		return "  " + lipgloss.NewStyle().Foreground(m.theme.yellow).Render("↻ applying operation")
	case m.consumerActionErr != nil:
		return "  " + lipgloss.NewStyle().Foreground(m.theme.red).Render("operation failed: "+m.consumerActionErr.Error())
	case m.consumerActionNotice != "":
		return "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+m.consumerActionNotice)
	default:
		return ""
	}
}

func (m Model) consumerListPane(width, height int) string {
	totalLag := uint64(0)
	for _, consumer := range m.consumers {
		totalLag += consumer.Lag()
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("CONSUMERS")
	stats := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d  •  %s lag", len(m.consumers), compactNumber(totalLag)))
	gap := max(width-lipgloss.Width(title)-lipgloss.Width(stats), 1)
	header := title + strings.Repeat(" ", gap) + stats
	columns := lipgloss.NewStyle().Foreground(m.theme.muted).Render("  STATUS    NAME" + strings.Repeat(" ", max(width-41, 1)) + "STREAM       LAG")
	capacity := max(height-4, 1)
	start, end := visibleWindow(len(m.consumers), m.consumerCursor, capacity)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		rows = append(rows, m.consumerRow(m.consumers[index], index == m.consumerCursor, width))
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  p pause/resume  •  x reset  •  d delete")
	return strings.Join([]string{header, columns, strings.Join(rows, "\n"), hint}, "\n")
}

func (m Model) consumerRow(consumer consumers.Summary, selected bool, width int) string {
	marker := "  "
	if selected {
		marker = "› "
	}
	nameWidth := max((width-39)*2/3, 10)
	streamWidth := max(width-39-nameWidth, 8)
	line := fmt.Sprintf("%s%-9s %-*s %-*s %7s", marker, consumer.Status, nameWidth, truncate(consumer.Name, nameWidth), streamWidth, truncate(consumer.Stream, streamWidth), compactNumber(consumer.Lag()))
	style := lipgloss.NewStyle().Foreground(m.consumerStatusColor(consumer.Status))
	if selected {
		style = style.Bold(true).Underline(true)
	}
	return style.Render(truncate(line, width))
}

func (m Model) consumerDetail(width int, consumer consumers.Summary, full bool) string {
	title := "CONSUMER DETAIL"
	if full {
		title = consumer.Name
	}
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	status := lipgloss.NewStyle().Bold(true).Foreground(m.consumerStatusColor(consumer.Status)).Render(string(consumer.Status))
	mode := "pull"
	if consumer.Push {
		mode = "push"
	}
	lifecycle := "ephemeral"
	if consumer.Durable {
		lifecycle = "durable"
	}
	lastActive := "never"
	if !consumer.LastActiveAt.IsZero() {
		lastActive = consumer.LastActiveAt.Local().Format("2006-01-02 15:04:05")
	}
	filters := strings.Join(consumer.FilterSubjects, ", ")
	if filters == "" {
		filters = "all stream subjects"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title),
		"",
		label.Render("Name:         ") + value.Render(truncate(consumer.Name, max(width-14, 8))),
		label.Render("Stream:       ") + value.Render(consumer.Stream),
		label.Render("Status:       ") + status + value.Render("  •  "+mode+"  •  "+lifecycle),
		"",
		label.Render("Work:         ") + value.Render(fmt.Sprintf("%s lag  •  %s pending  •  %d ack", compactNumber(consumer.Lag()), compactNumber(consumer.Pending), consumer.AckPending)),
		label.Render("Delivery:     ") + value.Render(fmt.Sprintf("%d redelivered  •  %d waiting", consumer.Redelivered, consumer.Waiting)),
		label.Render("Progress:     ") + value.Render(fmt.Sprintf("delivered #%d  •  ack floor #%d", consumer.DeliveredStream, consumer.AckFloorStream)),
		label.Render("Last active:  ") + value.Render(lastActive),
		label.Render("Filters:      ") + value.Render(truncate(filters, max(width-14, 8))),
	}
	if full {
		lines = append(lines, "", label.Render("esc back to consumers"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) consumerStatusColor(status consumers.Status) lipgloss.Color {
	switch status {
	case consumers.StatusActive:
		return m.theme.mint
	case consumers.StatusPaused:
		return m.theme.yellow
	case consumers.StatusLagging:
		return m.theme.red
	default:
		return m.theme.secondary
	}
}

func (m Model) messagesView(width, height int) string {
	records := m.displayedMessages()
	breadcrumb := "Streams  ›  Monitor"
	if m.messageStream != "" {
		breadcrumb = "Streams  ›  " + m.messageStream + "  ›  Monitor"
	}
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render(breadcrumb)
	if m.loadingMessages {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.spinner.View()+" Loading recent messages…")
	}
	if m.messageErr != nil {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render("Could not load messages: "+m.messageErr.Error())
	}
	if len(records) == 0 && m.searchQuery == "" {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("Select a stream and press enter to inspect its messages.")
	}
	search := m.messageSearchView(len(records))
	if len(records) == 0 {
		return heading + "\n" + search + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No messages match this search.")
	}
	if m.showMessageDetail {
		return m.messageDetailView(width, height, records[m.messageCursor])
	}

	paneHeight := max(height-3, 3)
	if width < 110 {
		return heading + "\n" + search + "\n\n" + m.messageListPane(records, width, paneHeight)
	}
	leftWidth := width * 55 / 100
	rightWidth := max(width-leftWidth-3, 24)
	list := m.messageListPane(records, leftWidth, paneHeight)
	preview := m.messagePreviewPane(rightWidth, paneHeight, records[m.messageCursor])
	divider := lipgloss.NewStyle().Foreground(m.theme.border).Render(strings.Repeat("│\n", max(lipgloss.Height(list), lipgloss.Height(preview))))
	panes := lipgloss.JoinHorizontal(lipgloss.Top, list, " ", divider, " ", preview)
	return heading + "\n" + search + "\n\n" + panes
}

func (m Model) messageSearchView(resultCount int) string {
	status := "  " + m.watchStatusView()
	query := m.searchQuery
	if query == "" && !m.searchMode {
		return lipgloss.NewStyle().Foreground(m.theme.muted).Render("/ search subject, payload or headers") + status
	}
	caret := ""
	if m.searchMode {
		caret = lipgloss.NewStyle().Foreground(m.theme.accent).Render("▏")
	}
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("  %d matches", resultCount))
	return lipgloss.NewStyle().Foreground(m.theme.accent).Render("/ ") + query + caret + count + status
}

func (m Model) watchStatusView() string {
	switch {
	case m.watchStarting:
		return lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View() + " STARTING LIVE")
	case m.watching && m.watchPaused:
		return lipgloss.NewStyle().Foreground(m.theme.yellow).Render(fmt.Sprintf("Ⅱ PAUSED  %d received", m.liveReceived))
	case m.watching:
		return lipgloss.NewStyle().Foreground(m.theme.mint).Render(fmt.Sprintf("● LIVE  %d received", m.liveReceived))
	default:
		return lipgloss.NewStyle().Foreground(m.theme.muted).Render("○ HISTORY  •  w watch")
	}
}

func (m Model) messageListPane(records []messages.Record, width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("MESSAGES")
	rowCapacity := max(height-4, 1)
	start, end := visibleWindow(len(records), m.messageCursor, rowCapacity)
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d-%d / %d", start+1, end, len(records)))
	gap := max(width-lipgloss.Width(title)-lipgloss.Width(count), 1)
	header := title + strings.Repeat(" ", gap) + count
	columns := lipgloss.NewStyle().Foreground(m.theme.muted).Render("  SEQ      TIME          SUBJECT" + strings.Repeat(" ", max(width-46, 1)) + "SIZE")
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		rows = append(rows, m.messageRow(records[index], index == m.messageCursor, width))
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter detail  •  w watch  •  space pause")
	return strings.Join([]string{header, columns, strings.Join(rows, "\n"), hint}, "\n")
}

func (m Model) messageRow(record messages.Record, selected bool, width int) string {
	marker := "  "
	style := lipgloss.NewStyle().Foreground(m.theme.secondary)
	if selected {
		marker = "› "
		style = style.Foreground(m.theme.accent).Bold(true).Underline(true)
	}
	timestamp := record.Time.Local().Format("15:04:05.000")
	subjectWidth := max(width-41, 10)
	sequence := fmt.Sprintf("#%d", record.Sequence)
	if record.Sequence == 0 {
		sequence = "LIVE"
	}
	line := fmt.Sprintf("%s%-8s %-13s %-*s %7s", marker, sequence, timestamp, subjectWidth, truncate(record.Subject, subjectWidth), humanBytes(uint64(len(record.Payload))))
	return style.Render(truncate(line, width))
}

func visibleWindow(total, cursor, capacity int) (int, int) {
	capacity = max(capacity, 1)
	start := max(cursor-capacity+1, 0)
	end := min(start+capacity, total)
	return start, end
}

func (m Model) messagePreviewPane(width, height int, record messages.Record) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("MESSAGE DETAIL")
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	timestamp := "—"
	if !record.Time.IsZero() {
		timestamp = record.Time.Local().Format("2006-01-02 15:04:05.000")
	}
	metadata := []string{
		label.Render("Subject:  ") + value.Render(truncate(record.Subject, max(width-11, 8))),
		label.Render("Time:     ") + value.Render(timestamp),
		label.Render("Size:     ") + value.Render(humanBytes(uint64(len(record.Payload)))),
		label.Render("Stream:   ") + value.Render(m.messageStream),
		label.Render("Sequence: ") + value.Render(fmt.Sprintf("%d", record.Sequence)),
	}
	payloadHeight := max(height-len(metadata)-4, 1)
	lines := wrapLines(prettyPayload(record.Payload), max(width, 1))
	lines = lines[:min(len(lines), payloadHeight)]
	for index, line := range lines {
		lines[index] = m.highlightJSONLine(line)
	}
	payloadLabel := label.Render("Payload:")
	return title + "\n" + strings.Join(metadata, "\n") + "\n\n" + payloadLabel + "\n" + strings.Join(lines, "\n")
}

func (m Model) messageDetailView(width, height int, record messages.Record) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(fmt.Sprintf("#%d  %s", record.Sequence, record.Subject))
	meta := lipgloss.NewStyle().Foreground(m.theme.muted).Render(record.Time.Local().Format(time.RFC3339Nano))
	lines := wrapLines(prettyPayload(record.Payload), max(width-2, 1))
	visibleHeight := max(height-5, 1)
	maxOffset := max(len(lines)-visibleHeight, 0)
	offset := min(m.messageDetailOffset, maxOffset)
	end := min(offset+visibleHeight, len(lines))
	visible := make([]string, 0, end-offset)
	for _, line := range lines[offset:end] {
		visible = append(visible, m.highlightJSONLine(line))
	}
	payload := strings.Join(visible, "\n")
	position := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("lines %d-%d/%d", offset+1, end, len(lines)))
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ scroll   esc back")
	return title + "\n" + meta + "\n\n" + payload + "\n" + position + "  " + hint
}

func (m Model) highlightJSONLine(line string) string {
	var result strings.Builder
	for index := 0; index < len(line); {
		if line[index] == '"' {
			end := index + 1
			for end < len(line) {
				if line[end] == '"' && line[end-1] != '\\' {
					end++
					break
				}
				end++
			}
			token := line[index:end]
			rest := strings.TrimLeft(line[end:], " \t")
			color := m.theme.mint
			if strings.HasPrefix(rest, ":") {
				color = m.theme.cyan
			}
			result.WriteString(lipgloss.NewStyle().Foreground(color).Render(token))
			index = end
			continue
		}

		end := index + 1
		for end < len(line) && line[end] != '"' {
			end++
		}
		result.WriteString(m.highlightJSONScalar(line[index:end]))
		index = end
	}
	return result.String()
}

func (m Model) highlightJSONScalar(value string) string {
	replacer := strings.NewReplacer(
		"true", lipgloss.NewStyle().Foreground(m.theme.yellow).Render("true"),
		"false", lipgloss.NewStyle().Foreground(m.theme.yellow).Render("false"),
		"null", lipgloss.NewStyle().Foreground(m.theme.red).Render("null"),
	)
	return lipgloss.NewStyle().Foreground(m.theme.secondary).Render(replacer.Replace(value))
}

func wrapLines(value string, width int) []string {
	result := make([]string, 0)
	for _, line := range strings.Split(value, "\n") {
		runes := []rune(line)
		if len(runes) == 0 {
			result = append(result, "")
			continue
		}
		for len(runes) > width {
			result = append(result, string(runes[:width]))
			runes = runes[width:]
		}
		result = append(result, string(runes))
	}
	return result
}

func prettyPayload(payload []byte) string {
	var formatted bytes.Buffer
	if json.Indent(&formatted, payload, "", "  ") == nil {
		return formatted.String()
	}
	return string(payload)
}

func (m Model) streamsView(width int) string {
	if m.showStreamDetail && len(m.streams) > 0 {
		return m.streamDetailView(width, m.streams[m.streamCursor])
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render("Stream Management")
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d streams", len(m.streams)))
	heading := title + "  " + count + m.streamActionFeedback()
	if len(m.streams) == 0 {
		empty := lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No streams found in this JetStream account.")
		return heading + "\n\n" + empty
	}

	rows := make([]string, 0, len(m.streams)+1)
	if width >= 90 {
		rows = append(rows, m.streamRow("", "NAME", "SUBJECTS", "MESSAGES", "STORAGE", "CONSUMERS", width, false))
	}
	for index, stream := range m.streams {
		marker := "  "
		if index == m.streamCursor {
			marker = "› "
		}
		rows = append(rows, m.streamRow(
			marker,
			stream.Name,
			strings.Join(stream.Subjects, ", "),
			compactNumber(stream.Messages),
			humanBytes(stream.Bytes),
			fmt.Sprintf("%d", stream.Consumers),
			width,
			index == m.streamCursor,
		))
	}

	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter detail  •  m messages  •  n new  •  e edit  •  S backup  •  R restore  •  d delete")
	return heading + "\n\n" + strings.Join(rows, "\n") + "\n\n" + hint
}

func (m Model) streamActionFeedback() string {
	switch {
	case m.streamTransferNotice != "":
		return "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+m.streamTransferNotice)
	case m.streamActionRunning:
		return "  " + lipgloss.NewStyle().Foreground(m.theme.yellow).Render("↻ applying")
	case m.streamActionErr != nil:
		return "  " + lipgloss.NewStyle().Foreground(m.theme.red).Render("operation failed: "+m.streamActionErr.Error())
	case m.streamActionNotice != "":
		return "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+m.streamActionNotice)
	default:
		return ""
	}
}

func (m Model) streamRow(marker, name, subjects, messages, bytes, consumers string, width int, selected bool) string {
	color := m.theme.secondary
	if selected {
		color = m.theme.accent
	}
	style := lipgloss.NewStyle().Foreground(color)
	if width < 90 {
		line := fmt.Sprintf("%s%s  %s", marker, name, messages)
		detail := fmt.Sprintf("  %s  •  %s  •  %s consumers", subjects, bytes, consumers)
		return style.Bold(selected).Render(line) + "\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render(detail)
	}

	nameWidth := max(width/5, 14)
	subjectWidth := max(width-nameWidth-39, 16)
	return style.Bold(selected).Render(fmt.Sprintf("%-2s%-*s %-*s %10s %11s %9s", marker, nameWidth, truncate(name, nameWidth), subjectWidth, truncate(subjects, subjectWidth), messages, bytes, consumers))
}

func truncate(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return string([]rune(value)[:width-1]) + "…"
}

func (m Model) dashboardView(width int) string {
	columns := dashboardColumns(width)
	cardWidth := (width - (columns - 1)) / columns
	cards := m.dashboardCards(cardWidth, columns == 1)
	meta := lipgloss.NewStyle().Foreground(m.theme.secondary).Render(
		strings.Join(nonEmpty(m.snapshot.ServerName, "NATS "+m.snapshot.Version, m.snapshot.Cluster), "  •  "),
	)
	return meta + "\n\n" + renderCardGrid(cards, columns)
}

func (m Model) dashboardCards(width int, compact bool) []string {
	return []string{
		m.card("MESSAGES", compactNumber(m.snapshot.Messages), "stored", m.theme.accent, width, compact),
		m.card("STORAGE", humanBytes(m.snapshot.Bytes), "JetStream", m.theme.cyan, width, compact),
		m.card("STREAMS", fmt.Sprintf("%d", m.snapshot.Streams), "active", m.theme.mint, width, compact),
		m.card("CONSUMERS", fmt.Sprintf("%d", m.snapshot.Consumers), "attached", m.theme.yellow, width, compact),
	}
}

func (m Model) card(label, value, caption string, color lipgloss.Color, width int, compact bool) string {
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.muted)
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(color)
	captionStyle := lipgloss.NewStyle().Foreground(m.theme.secondary)

	body := labelStyle.Render(label) + "\n\n" + valueStyle.Render(value) + "\n" + captionStyle.Render(caption)
	height := 4
	if compact {
		gap := max(width-6-lipgloss.Width(label)-lipgloss.Width(value), 1)
		body = labelStyle.Render(label) + strings.Repeat(" ", gap) + valueStyle.Render(value) + "\n" + captionStyle.Render(caption)
		height = 2
	}

	return lipgloss.NewStyle().
		Width(max(width-1, 1)).
		Height(height).
		Padding(0, 2).
		BorderStyle(lipgloss.ThickBorder()).
		BorderLeft(true).
		BorderForeground(color).
		Render(body)
}

func (m Model) helpView(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Keyboard")
	keys := "  ?         close help\n  q         quit\n  ←/→       switch workspace\n  ↑/↓ j/k   move or scroll\n  pgup/dn   jump through lists\n  g/G       first/last item (g toggles topology graph)\n  enter     open selected item\n  esc       close detail/search\n  1-9/0/A/! jump to workspace\n  r         refresh now\n  c         connection profiles\n  /         reactive search\n  w         start/stop live watch\n  space     pause/resume live watch\n  p         pause/resume consumer\n  x         reset consumer / close client\n  n         new stream/KV/object bucket\n  e         edit selected resource\n  m         stream messages\n  d         delete selected resource\n  :/ctrl+k  command bar\n  ctrl+r    command history\n  B         toggle bookmark\n  b         open bookmarks"
	return lipgloss.NewStyle().
		Width(max(width-8, 1)).
		Height(max(height-4, 1)).
		Foreground(m.theme.primary).
		Padding(2, 4).
		Render(title + "\n\n" + keys)
}

func dashboardColumns(width int) int {
	switch {
	case width >= 116:
		return 4
	case width >= 56:
		return 2
	default:
		return 1
	}
}

func renderCardGrid(cards []string, columns int) string {
	rows := make([]string, 0, (len(cards)+columns-1)/columns)
	for start := 0; start < len(cards); start += columns {
		end := min(start+columns, len(cards))
		items := make([]string, 0, (end-start)*2-1)
		for index, card := range cards[start:end] {
			if index > 0 {
				items = append(items, " ")
			}
			items = append(items, card)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, items...))
	}
	return strings.Join(rows, "\n\n")
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func compactNumber(value uint64) string {
	switch {
	case value >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(value)/1_000_000_000)
	case value >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%.1fK", float64(value)/1_000)
	default:
		return fmt.Sprintf("%d", value)
	}
}

func humanBytes(value uint64) string {
	const (
		kb = 1_000
		mb = 1_000_000
		gb = 1_000_000_000
	)
	switch {
	case value >= gb:
		return fmt.Sprintf("%.1f GB", float64(value)/gb)
	case value >= mb:
		return fmt.Sprintf("%.1f MB", float64(value)/mb)
	case value >= kb:
		return fmt.Sprintf("%.1f KB", float64(value)/kb)
	default:
		return fmt.Sprintf("%d B", value)
	}
}
