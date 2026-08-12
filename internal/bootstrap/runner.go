package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/clcsolutions/nats-tui/internal/adapters/jwtauthority"
	"github.com/clcsolutions/nats-tui/internal/adapters/localfiles"
	"github.com/clcsolutions/nats-tui/internal/adapters/localstate"
	adaptercontext "github.com/clcsolutions/nats-tui/internal/adapters/natscontext"
	"github.com/clcsolutions/nats-tui/internal/adapters/natsmonitor"
	"github.com/clcsolutions/nats-tui/internal/app/alerts"
	appbookmarks "github.com/clcsolutions/nats-tui/internal/app/bookmarks"
	appcommandbar "github.com/clcsolutions/nats-tui/internal/app/commandbar"
	"github.com/clcsolutions/nats-tui/internal/app/connections"
	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	"github.com/clcsolutions/nats-tui/internal/app/operator"
	"github.com/clcsolutions/nats-tui/internal/app/requestreply"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/clcsolutions/nats-tui/internal/app/subjects"
	"github.com/clcsolutions/nats-tui/internal/app/topology"
	"github.com/clcsolutions/nats-tui/internal/config"
	domainauthority "github.com/clcsolutions/nats-tui/internal/domain/authority"
	"github.com/clcsolutions/nats-tui/internal/domain/connection"
	domainrequestreply "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
	domaintopology "github.com/clcsolutions/nats-tui/internal/domain/topology"
	"github.com/clcsolutions/nats-tui/internal/ports/tui"
	"github.com/nats-io/nuid"
)

type Runner struct {
	objectFiles        *localfiles.ObjectTransfer
	streamArchives     *localfiles.StreamArchive
	messageFiles       *localfiles.MessageFileWriter
	connect            Connector
	copyValue          tui.CopyValueFunc
	timeout            TimeoutFactory
	requestHistory     requestreply.HistoryRepository
	requestTemplates   requestreply.TemplateRepository
	requestStateMu     *sync.Mutex
	requestStates      map[string]*localstate.RequestRepository
	clock              requestreply.Clock
	ids                requestreply.IDGenerator
	alertRepository    alertStore
	commandRepository  appcommandbar.Repository
	bookmarkRepository appbookmarks.Repository
	bookmarkStateMu    *sync.Mutex
	bookmarkStates     map[string]*localstate.BookmarkRepository
	profileRepository  connections.ProfileRepository
	contextSource      connections.ContextSource
	readPayload        func(string) ([]byte, error)
	authoritySigner    func([]byte) (operator.AuthoritySigner, error)
	accountSigner      func([]byte) (operator.AuthoritySigner, error)
	authorityStore     func(string) (operator.AuthorityRepository, error)
}

type alertStore interface {
	alerts.AlertRepository
	alerts.RuleRepository
}

type Session interface {
	dashboard.Monitor
	streams.Catalog
	streams.Operator
	streams.Maintenance
	consumers.Catalog
	consumers.Operator
	consumers.ConfigOperator
	connections.Source
	connections.Operator
	messages.Reader
	messages.Browser
	messages.Watcher
	messages.OrderedWatcher
	subjects.Observer
	kvstore.BucketCatalog
	kvstore.BucketCreator
	kvstore.BucketUpdater
	kvstore.BucketDeleter
	kvstore.KeyCatalog
	kvstore.EntryReader
	kvstore.EntryWriter
	kvstore.EntryRemover
	kvstore.HistoryReader
	kvstore.WatchSource
	objectstore.BucketCatalog
	objectstore.ObjectCatalog
	objectstore.TransferStore
	objectstore.ObjectReader
	objectstore.ObjectUpdater
	objectstore.ObjectDeleter
	objectstore.ObjectWatchSource
	objectstore.LinkCreator
	objectstore.BucketCreator
	objectstore.BucketUpdater
	objectstore.BucketDeleter
	operator.Source
	operator.ClaimPublisher
	requestreply.Publisher
	requestreply.Requester
	alerts.AdvisorySource
	alerts.HealthSource
	topology.Source
	topology.Operator
	Close()
}

type Connector func(config.Settings) (Session, error)
type TimeoutFactory func(context.Context, time.Duration) (context.Context, context.CancelFunc)

func NewRunner() Runner {
	files := localfiles.NewOSObjectTransfer()
	archives := localfiles.NewOSStreamArchive()
	messageFiles := localfiles.NewOSMessageFileWriter()
	return Runner{
		objectFiles: &files, streamArchives: &archives, messageFiles: &messageFiles, connect: func(settings config.Settings) (Session, error) { return natsmonitor.Connect(settings) },
		copyValue: clipboard.WriteAll, timeout: context.WithTimeout,
		requestStateMu: &sync.Mutex{}, requestStates: make(map[string]*localstate.RequestRepository),
		bookmarkStateMu: &sync.Mutex{}, bookmarkStates: make(map[string]*localstate.BookmarkRepository),
		clock: wallClock{}, ids: nuidGenerator{},
		readPayload:     os.ReadFile,
		authoritySigner: func(seed []byte) (operator.AuthoritySigner, error) { return jwtauthority.NewDefault(seed) },
		accountSigner:   func(seed []byte) (operator.AuthoritySigner, error) { return jwtauthority.NewDefaultFromAccount(seed) },
		authorityStore: func(directory string) (operator.AuthorityRepository, error) {
			return localfiles.NewAuthorityStore(directory)
		},
	}
}

func defaultAlertRepository(profile string) *localstate.AlertRepository {
	directory, err := os.UserConfigDir()
	if err != nil {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			directory = filepath.Join(home, ".config")
		} else {
			directory = "."
		}
	}
	profile = localStateProfile(profile)
	return localstate.NewAlertRepository(localstate.OSStateFile{Path: filepath.Join(directory, "nats-tui", "alerts-"+profile+".json")}, 1000)
}

func defaultCommandRepository(profile string) *localstate.CommandBarRepository {
	directory, err := os.UserConfigDir()
	if err != nil {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			directory = filepath.Join(home, ".config")
		} else {
			directory = "."
		}
	}
	profile = localStateProfile(profile)
	return localstate.NewCommandBarRepository(localstate.OSStateFile{Path: filepath.Join(directory, "nats-tui", "commands-"+profile+".json")}, 200)
}

func defaultBookmarkRepository(profile string) *localstate.BookmarkRepository {
	directory, err := os.UserConfigDir()
	if err != nil {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			directory = filepath.Join(home, ".config")
		} else {
			directory = "."
		}
	}
	profile = localStateProfile(profile)
	return localstate.NewBookmarkRepository(localstate.OSStateFile{Path: filepath.Join(directory, "nats-tui", "bookmarks-"+profile+".json")})
}

func defaultProfileRepository() *localstate.ProfileRepository {
	directory, err := os.UserConfigDir()
	if err != nil {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			directory = filepath.Join(home, ".config")
		} else {
			directory = "."
		}
	}
	return localstate.NewProfileRepository(localstate.OSStateFile{Path: filepath.Join(directory, "nats-tui", "profiles.json")})
}

func (r Runner) profileService() connections.ProfileService {
	repository := r.profileRepository
	if repository == nil {
		repository = defaultProfileRepository()
	}
	contextSource := r.contextSource
	if contextSource == nil {
		contextSource = adaptercontext.NewImporter(adaptercontext.OSFileSystem{})
	}
	return connections.NewProfileImportService(repository, r.clock, contextSource)
}

func (r Runner) WithProfileDependencies(repository connections.ProfileRepository, clock requestreply.Clock) Runner {
	if repository == nil || clock == nil {
		panic("profile dependencies cannot be nil")
	}
	r.profileRepository, r.clock = repository, clock
	return r
}

func (r Runner) WithContextImportDependency(source connections.ContextSource) Runner {
	if source == nil {
		panic("context source cannot be nil")
	}
	r.contextSource = source
	return r
}

func (r Runner) WithAuthorityDependencies(operatorSigner, accountSigner func([]byte) (operator.AuthoritySigner, error), store func(string) (operator.AuthorityRepository, error)) Runner {
	if operatorSigner == nil || accountSigner == nil || store == nil {
		panic("authority dependencies cannot be nil")
	}
	r.authoritySigner, r.accountSigner, r.authorityStore = operatorSigner, accountSigner, store
	return r
}

// NewRunnerWithDependencies is the deterministic composition seam used by tests
// and alternative adapters.
func NewRunnerWithDependencies(files localfiles.ObjectTransfer, connect Connector, copyValue tui.CopyValueFunc, timeout TimeoutFactory) Runner {
	if connect == nil {
		panic("nil NATS connector")
	}
	if timeout == nil {
		panic("nil timeout factory")
	}
	runner := NewRunner()
	runner.objectFiles, runner.connect, runner.copyValue, runner.timeout = &files, connect, copyValue, timeout
	return runner
}

func (r Runner) WithStreamArchiveDependencies(archives localfiles.StreamArchive) Runner {
	r.streamArchives = &archives
	return r
}

func (r Runner) WithRequestDependencies(history requestreply.HistoryRepository, templates requestreply.TemplateRepository, clock requestreply.Clock, ids requestreply.IDGenerator) Runner {
	if history == nil || templates == nil || clock == nil || ids == nil {
		panic("request/reply dependencies cannot be nil")
	}
	r.requestHistory, r.requestTemplates, r.clock, r.ids = history, templates, clock, ids
	return r
}

func (r Runner) WithAlertDependencies(repository alertStore, clock requestreply.Clock, ids requestreply.IDGenerator) Runner {
	if repository == nil || clock == nil || ids == nil {
		panic("alert dependencies cannot be nil")
	}
	r.alertRepository, r.clock, r.ids = repository, clock, ids
	return r
}

func (r Runner) WithCommandBarDependencies(repository appcommandbar.Repository, clock requestreply.Clock, ids requestreply.IDGenerator) Runner {
	if repository == nil || clock == nil || ids == nil {
		panic("command bar dependencies cannot be nil")
	}
	r.commandRepository, r.clock, r.ids = repository, clock, ids
	return r
}

func (r Runner) WithBookmarkDependencies(repository appbookmarks.Repository, clock requestreply.Clock, ids requestreply.IDGenerator) Runner {
	if repository == nil || clock == nil || ids == nil {
		panic("bookmark dependencies cannot be nil")
	}
	r.bookmarkRepository, r.clock, r.ids = repository, clock, ids
	return r
}

func (r Runner) WithPayloadReader(read func(string) ([]byte, error)) Runner {
	if read == nil {
		panic("nil payload reader")
	}
	r.readPayload = read
	return r
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

type nuidGenerator struct{}

func (nuidGenerator) Next() string { return nuid.Next() }

func defaultRequestRepository(profile string) *localstate.RequestRepository {
	directory, err := os.UserConfigDir()
	if err != nil {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			directory = filepath.Join(home, ".config")
		} else {
			directory = "."
		}
	}
	profile = localStateProfile(profile)
	file := localstate.OSStateFile{Path: filepath.Join(directory, "nats-tui", "request-reply-"+profile+".json")}
	return localstate.NewRequestRepository(file, 100)
}

func localStateProfile(profile string) string {
	original := profile
	profile = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, profile)
	if profile == "" {
		return "local"
	}
	if profile != original {
		digest := sha256.Sum256([]byte(original))
		profile += "-" + hex.EncodeToString(digest[:4])
	}
	return profile
}

func (r Runner) requestRepositories(profile string) (requestreply.HistoryRepository, requestreply.TemplateRepository) {
	if r.requestHistory != nil && r.requestTemplates != nil {
		return r.requestHistory, r.requestTemplates
	}
	profile = localStateProfile(profile)
	if r.requestStateMu == nil || r.requestStates == nil {
		state := defaultRequestRepository(profile)
		return state, state
	}
	r.requestStateMu.Lock()
	defer r.requestStateMu.Unlock()
	state, ok := r.requestStates[profile]
	if !ok {
		state = defaultRequestRepository(profile)
		r.requestStates[profile] = state
	}
	return state, state
}

func (r Runner) connector() Connector {
	if r.connect != nil {
		return r.connect
	}
	return func(settings config.Settings) (Session, error) { return natsmonitor.Connect(settings) }
}

func (r Runner) copier() tui.CopyValueFunc {
	if r.copyValue != nil {
		return r.copyValue
	}
	return clipboard.WriteAll
}

func (r Runner) withTimeout(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
	if r.timeout != nil {
		return r.timeout(ctx, duration)
	}
	return context.WithTimeout(ctx, duration)
}

const refreshTimeout = 3 * time.Second
const consumerActionTimeout = 5 * time.Second
const streamActionTimeout = 5 * time.Second

func (r Runner) GetTopology(ctx context.Context, settings config.Settings) (domaintopology.Snapshot, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return domaintopology.Snapshot{}, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
	defer cancel()
	return topology.NewGetHandler(monitor).Handle(queryCtx)
}

func (r Runner) OperateTopology(ctx context.Context, settings config.Settings, command topology.Command) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return topology.NewOperateHandler(monitor).Handle(actionCtx, command)
}

func (r Runner) ListNATSConnections(ctx context.Context, settings config.Settings) (connections.Snapshot, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return connections.Snapshot{}, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
	defer cancel()
	return connections.NewListHandler(monitor).Handle(queryCtx)
}

func (r Runner) ListOperatorCatalog(ctx context.Context, settings config.Settings) (operator.Catalog, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return operator.Catalog{}, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
	defer cancel()
	return r.operatorCatalog(queryCtx, monitor, settings)
}

func (r Runner) operatorCatalog(ctx context.Context, source operator.Source, settings config.Settings) (operator.Catalog, error) {
	network, err := operator.NewListHandler(source).Handle(ctx)
	if err != nil {
		return operator.Catalog{}, err
	}
	if settings.AuthorityDir == "" {
		return network, nil
	}
	repository, err := r.authorityStore(settings.AuthorityDir)
	if err != nil {
		return operator.Catalog{}, err
	}
	managed, err := operator.NewListHandler(repository).Handle(ctx)
	if err != nil {
		return operator.Catalog{}, err
	}
	capabilities := managed.Capabilities()
	capabilities.ClaimsWrite = settings.OperatorSeed != "" && network.Capabilities().SystemAccount
	managed = operator.NewCatalog(capabilities, managed.Accounts(), managed.Users())
	return operator.MergeCatalog(network, managed), nil
}

func (r Runner) CreateOperatorAccount(ctx context.Context, settings config.Settings, operatorSeedPath, outputDirectory string, definition domainauthority.AccountDefinition) (operator.AccountArtifacts, error) {
	seed, err := r.payloadReader()(operatorSeedPath)
	if err != nil {
		return operator.AccountArtifacts{}, fmt.Errorf("read operator seed: %w", err)
	}
	signer, err := r.authoritySigner(seed)
	if err != nil {
		return operator.AccountArtifacts{}, err
	}
	store, err := r.authorityStore(outputDirectory)
	if err != nil {
		return operator.AccountArtifacts{}, err
	}
	monitor, err := r.connector()(settings)
	if err != nil {
		return operator.AccountArtifacts{}, err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return operator.NewCreateAccountHandler(signer, store, monitor).Handle(actionCtx, definition)
}

func (r Runner) CreateOperatorUser(ctx context.Context, accountSeedPath, outputDirectory string, definition domainauthority.UserDefinition) (operator.UserArtifacts, error) {
	seed, err := r.payloadReader()(accountSeedPath)
	if err != nil {
		return operator.UserArtifacts{}, fmt.Errorf("read account seed: %w", err)
	}
	// The signer validates that this is an account seed before it ever reaches storage.
	signer, err := r.accountSigner(seed)
	if err != nil {
		return operator.UserArtifacts{}, err
	}
	store, err := r.authorityStore(outputDirectory)
	if err != nil {
		return operator.UserArtifacts{}, err
	}
	return operator.NewCreateUserHandler(signer, store).Handle(ctx, seed, definition)
}

func (r Runner) UpdateOperatorAccount(ctx context.Context, settings config.Settings, operatorSeedPath, outputDirectory, accountID string, definition domainauthority.AccountDefinition) (operator.AccountArtifacts, error) {
	signer, repository, monitor, err := r.operatorMutationDependencies(settings, operatorSeedPath, outputDirectory)
	if err != nil {
		return operator.AccountArtifacts{}, err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return operator.NewUpdateAccountHandler(signer, repository, monitor).Handle(actionCtx, accountID, definition)
}

func (r Runner) DeleteOperatorAccount(ctx context.Context, settings config.Settings, operatorSeedPath, outputDirectory, accountID string) (string, error) {
	signer, repository, monitor, err := r.operatorMutationDependencies(settings, operatorSeedPath, outputDirectory)
	if err != nil {
		return "", err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return operator.NewDeleteAccountHandler(signer, repository, monitor).Handle(actionCtx, accountID)
}

func (r Runner) UpdateOperatorUser(ctx context.Context, outputDirectory, userID string, definition domainauthority.UserDefinition) (operator.UserArtifacts, error) {
	repository, err := r.authorityStore(outputDirectory)
	if err != nil {
		return operator.UserArtifacts{}, err
	}
	accountSeed, err := repository.AccountSeed(ctx, definition.Account())
	if err != nil {
		return operator.UserArtifacts{}, err
	}
	signer, err := r.accountSigner(accountSeed)
	if err != nil {
		return operator.UserArtifacts{}, err
	}
	return operator.NewUpdateUserHandler(signer, repository).Handle(ctx, userID, definition)
}

func (r Runner) RevokeOperatorUser(ctx context.Context, settings config.Settings, operatorSeedPath, outputDirectory, accountID, userID string) (operator.AccountArtifacts, error) {
	signer, repository, monitor, err := r.operatorMutationDependencies(settings, operatorSeedPath, outputDirectory)
	if err != nil {
		return operator.AccountArtifacts{}, err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return operator.NewRevokeUserHandler(signer, repository, monitor).Handle(actionCtx, accountID, userID, r.clock.Now())
}

func (r Runner) DeleteOperatorUser(ctx context.Context, settings config.Settings, operatorSeedPath, outputDirectory, accountID, userID string) (string, error) {
	signer, repository, monitor, err := r.operatorMutationDependencies(settings, operatorSeedPath, outputDirectory)
	if err != nil {
		return "", err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return operator.NewDeleteUserHandler(signer, repository, monitor).Handle(actionCtx, accountID, userID, r.clock.Now())
}

func (r Runner) AddOperatorSigningKey(ctx context.Context, settings config.Settings, operatorSeedPath, outputDirectory, accountID string, definition domainauthority.SigningKeyDefinition) (operator.SigningKeyArtifacts, error) {
	signer, repository, monitor, err := r.operatorMutationDependencies(settings, operatorSeedPath, outputDirectory)
	if err != nil {
		return operator.SigningKeyArtifacts{}, err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return operator.NewAddSigningKeyHandler(signer, repository, monitor).Handle(actionCtx, accountID, definition)
}

func (r Runner) operatorMutationDependencies(settings config.Settings, seedPath, directory string) (operator.AuthoritySigner, operator.AuthorityRepository, Session, error) {
	seed, err := r.payloadReader()(seedPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read operator seed: %w", err)
	}
	signer, err := r.authoritySigner(seed)
	if err != nil {
		return nil, nil, nil, err
	}
	repository, err := r.authorityStore(directory)
	if err != nil {
		return nil, nil, nil, err
	}
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, nil, nil, err
	}
	return signer, repository, monitor, nil
}

func (r Runner) payloadReader() func(string) ([]byte, error) {
	if r.readPayload != nil {
		return r.readPayload
	}
	return os.ReadFile
}
func (r Runner) CloseNATSConnection(ctx context.Context, settings config.Settings, serverID string, cid uint64) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return connections.NewCloseHandler(monitor).Handle(actionCtx, serverID, cid)
}

func (r Runner) PublishCoreMessage(ctx context.Context, settings config.Settings, message domainrequestreply.Message) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return requestreply.NewPublishHandler(monitor).Handle(actionCtx, message)
}

func (r Runner) SendCoreRequest(ctx context.Context, settings config.Settings, request domainrequestreply.Request) (domainrequestreply.Exchange, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return domainrequestreply.Exchange{}, err
	}
	defer monitor.Close()
	history, _ := r.requestRepositories(settings.Profile)
	return r.requestHandler(monitor, history).Handle(ctx, request)
}

func (r Runner) ListRequestHistory(ctx context.Context, settings config.Settings) ([]domainrequestreply.Exchange, error) {
	history, _ := r.requestRepositories(settings.Profile)
	return requestreply.NewListHistoryHandler(history).Handle(ctx)
}

func (r Runner) ReplayCoreRequest(ctx context.Context, settings config.Settings, id string) (domainrequestreply.Exchange, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return domainrequestreply.Exchange{}, err
	}
	defer monitor.Close()
	history, _ := r.requestRepositories(settings.Profile)
	return requestreply.NewReplayHandler(history, r.requestHandler(monitor, history)).Handle(ctx, id)
}

func (r Runner) SaveRequestTemplate(ctx context.Context, settings config.Settings, template domainrequestreply.Template) error {
	_, templates := r.requestRepositories(settings.Profile)
	return requestreply.NewSaveTemplateHandler(templates).Handle(ctx, template)
}

func (r Runner) ListRequestTemplates(ctx context.Context, settings config.Settings) ([]domainrequestreply.Template, error) {
	_, templates := r.requestRepositories(settings.Profile)
	return requestreply.NewListTemplatesHandler(templates).Handle(ctx)
}

func (r Runner) GetRequestTemplate(ctx context.Context, settings config.Settings, name string) (domainrequestreply.Template, error) {
	_, templates := r.requestRepositories(settings.Profile)
	return requestreply.NewGetTemplateHandler(templates).Handle(ctx, name)
}

func (r Runner) DeleteRequestTemplate(ctx context.Context, settings config.Settings, name string) error {
	_, templates := r.requestRepositories(settings.Profile)
	return requestreply.NewDeleteTemplateHandler(templates).Handle(ctx, name)
}

func (r Runner) requestHandler(requester requestreply.Requester, history requestreply.HistoryRepository) requestreply.SendRequestHandler {
	return requestreply.NewSendRequestHandler(requester, history, r.clock, r.ids, r.withTimeout)
}

func (r Runner) CreateKeyValueBucket(ctx context.Context, settings config.Settings, definition kvstore.Definition) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	operationCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewCreateBucketHandler(monitor).Handle(operationCtx, definition)
}

func (r Runner) UpdateKeyValueBucket(ctx context.Context, settings config.Settings, name string, changes kvstore.Changes) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	operationCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewUpdateBucketHandler(monitor).Handle(operationCtx, name, changes)
}

func (r Runner) DeleteKeyValueBucket(ctx context.Context, settings config.Settings, name string) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	operationCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewDeleteBucketHandler(monitor).Handle(operationCtx, name)
}

func (r Runner) ListKeyValueKeys(ctx context.Context, settings config.Settings, bucket string) ([]string, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewListKeysHandler(monitor).Handle(queryCtx, bucket)
}

func (r Runner) GetKeyValueEntry(ctx context.Context, settings config.Settings, bucket, key string) (kvstore.Entry, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return kvstore.Entry{}, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewGetEntryHandler(monitor).Handle(queryCtx, bucket, key)
}

func (r Runner) PutKeyValueEntry(ctx context.Context, settings config.Settings, command kvstore.WriteEntry) (uint64, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return 0, err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewPutEntryHandler(monitor).Handle(actionCtx, command)
}

func (r Runner) RemoveKeyValueEntry(ctx context.Context, settings config.Settings, command kvstore.RemoveEntry) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewRemoveEntryHandler(monitor).Handle(actionCtx, command)
}

func (r Runner) ListKeyValueHistory(ctx context.Context, settings config.Settings, bucket, key string) ([]kvstore.Entry, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewListHistoryHandler(monitor).Handle(queryCtx, bucket, key)
}

func (r Runner) RestoreKeyValueEntry(ctx context.Context, settings config.Settings, command kvstore.RestoreEntry) (kvstore.RestoredEntry, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return kvstore.RestoredEntry{}, err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return kvstore.NewRestoreHandler(monitor, monitor).Handle(actionCtx, command)
}

func (r Runner) WatchKeyValue(ctx context.Context, settings config.Settings, bucket, key string) (<-chan kvstore.Entry, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	events, err := kvstore.NewWatchHandler(monitor).Handle(ctx, bucket, key)
	if err != nil {
		monitor.Close()
		return nil, err
	}
	forwarded := make(chan kvstore.Entry)
	go func() {
		defer close(forwarded)
		defer monitor.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case entry, ok := <-events:
				if !ok {
					return
				}
				select {
				case forwarded <- entry:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return forwarded, nil
}

func (r Runner) UploadObjectFile(ctx context.Context, settings config.Settings, command objectstore.UploadFile, progress objectstore.ProgressFunc) (objectstore.ObjectSummary, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return objectstore.ObjectSummary{}, err
	}
	defer monitor.Close()
	return r.objectTransfer().UploadObject(ctx, monitor, command, progress)
}

func (r Runner) DownloadObjectFile(ctx context.Context, settings config.Settings, command objectstore.DownloadFile, progress objectstore.ProgressFunc) (objectstore.DownloadResult, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return objectstore.DownloadResult{}, err
	}
	defer monitor.Close()
	return r.objectTransfer().DownloadObject(ctx, monitor, command, progress)
}

func (r Runner) EditObject(ctx context.Context, settings config.Settings, patch objectstore.EditObject) (objectstore.ObjectSummary, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return objectstore.ObjectSummary{}, err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return objectstore.NewEditObjectHandler(monitor, monitor).Handle(actionCtx, patch)
}

func (r Runner) DeleteObject(ctx context.Context, settings config.Settings, bucket, name string) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return objectstore.NewDeleteObjectHandler(monitor).Handle(actionCtx, bucket, name)
}

func (r Runner) CreateObjectLink(ctx context.Context, settings config.Settings, definition objectstore.LinkDefinition) (objectstore.ObjectSummary, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return objectstore.ObjectSummary{}, err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return objectstore.NewCreateLinkHandler(monitor).Handle(actionCtx, definition)
}

func (r Runner) CreateObjectStoreBucket(ctx context.Context, settings config.Settings, definition objectstore.Definition) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return objectstore.NewCreateBucketHandler(monitor).Handle(actionCtx, definition)
}

func (r Runner) UpdateObjectStoreBucket(ctx context.Context, settings config.Settings, name string, changes objectstore.Changes) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return objectstore.NewUpdateBucketHandler(monitor).Handle(actionCtx, name, changes)
}

func (r Runner) DeleteObjectStoreBucket(ctx context.Context, settings config.Settings, name string) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return objectstore.NewDeleteBucketHandler(monitor).Handle(actionCtx, name)
}

func (r Runner) objectTransfer() localfiles.ObjectTransfer {
	if r.objectFiles == nil {
		return localfiles.NewOSObjectTransfer()
	}
	return *r.objectFiles
}

func (r Runner) streamArchive() localfiles.StreamArchive {
	if r.streamArchives == nil {
		return localfiles.NewOSStreamArchive()
	}
	return *r.streamArchives
}

type workspaceState struct {
	snapshot  dashboard.Snapshot
	streams   []streams.Summary
	consumers []consumers.Summary
	buckets   []kvstore.BucketSummary
	objects   []objectstore.BucketSummary
}

func (r Runner) Run(ctx context.Context, settings config.Settings) error {
	sessions := &sessionStore{objectFiles: r.objectTransfer(), streamArchive: r.streamArchive()}
	defer sessions.Close()
	var alertCancel context.CancelFunc
	var subjectObservationMu sync.Mutex
	var subjectObservationCancel context.CancelFunc
	activeSettings := settings
	defer func() {
		if alertCancel != nil {
			alertCancel()
		}
	}()

	connect := func(input tui.ConnectionInput) tea.Cmd {
		return func() tea.Msg {
			connectionSettings := settings
			var profile connection.Profile
			var err error
			if input.UseProfile {
				profile, err = r.profileService().Find(ctx, input.Profile)
				if err == nil {
					connectionSettings = settingsFromProfile(profile)
				}
			} else {
				profile, err = connection.NewDirectProfile(input.Profile, input.Server)
				connectionSettings.Profile, connectionSettings.Server = profile.Name(), profile.Server()
			}
			if err != nil {
				return tui.ConnectionFailed(err)
			}
			monitor, err := r.connector()(connectionSettings)
			if err != nil {
				return tui.ConnectionFailed(err)
			}

			state, err := loadWorkspace(ctx, monitor)
			if err != nil {
				monitor.Close()
				return tui.ConnectionFailed(err)
			}

			sessions.Replace(monitor)
			activeSettings = connectionSettings
			if input.UseProfile {
				_ = r.profileService().MarkUsed(ctx, profile.Name())
			}
			return tui.ConnectedWithObjects(profile.Name(), state.snapshot, state.streams, state.consumers, state.buckets, state.objects)
		}
	}
	loadProfiles := func() tea.Cmd {
		return func() tea.Msg {
			items, err := r.profileService().List(ctx)
			if err != nil {
				return tui.ConnectionProfilesFailed(err)
			}
			return tui.ConnectionProfilesLoaded(items)
		}
	}
	loadMessages := func(stream string) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.MessagesFailed(fmt.Errorf("no active NATS connection"))
			}
			handler := messages.NewLoadRecentHandler(monitor)
			records, err := handler.Handle(ctx, stream, 50)
			if err != nil {
				return tui.MessagesFailed(err)
			}
			return tui.MessagesLoaded(stream, records)
		}
	}
	loadMessageHistory := func(query messages.HistoryQuery) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.MessagesFailed(fmt.Errorf("no active NATS connection"))
			}
			queryCtx, cancel := r.withTimeout(ctx, 15*time.Second)
			defer cancel()
			records, err := messages.NewBrowseHandler(monitor).Handle(queryCtx, query)
			if err != nil {
				return tui.MessagesFailed(err)
			}
			return tui.MessagesLoaded(query.Stream, records)
		}
	}
	startSubjectObservation := func() tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.SubjectObservationFailed(fmt.Errorf("no active NATS connection"))
			}
			observationCtx, cancel := context.WithCancel(ctx)
			events, err := subjects.NewObserveHandler(monitor).Handle(observationCtx)
			if err != nil {
				cancel()
				return tui.SubjectObservationFailed(err)
			}
			subjectObservationMu.Lock()
			if subjectObservationCancel != nil {
				subjectObservationCancel()
			}
			subjectObservationCancel = cancel
			subjectObservationMu.Unlock()
			return tui.SubjectObservationStarted(events)
		}
	}
	stopSubjectObservation := func() {
		subjectObservationMu.Lock()
		defer subjectObservationMu.Unlock()
		if subjectObservationCancel != nil {
			subjectObservationCancel()
			subjectObservationCancel = nil
		}
	}
	exportMessages := func(request messages.ExportRequest) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.MessageExportFailed(fmt.Errorf("no active NATS connection"))
			}
			writer := r.messageFiles
			if writer == nil {
				value := localfiles.NewOSMessageFileWriter()
				writer = &value
			}
			exportCtx, cancel := r.withTimeout(ctx, 30*time.Second)
			defer cancel()
			result, err := messages.NewExportHandler(monitor, writer).Handle(exportCtx, request)
			if err != nil {
				return tui.MessageExportFailed(err)
			}
			return tui.MessageExportSucceeded(result)
		}
	}
	deleteMessage := func(command streams.MessageDeleteCommand) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.MessageDeleteFailed(fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := streams.NewMaintenanceHandler(monitor).DeleteMessage(actionCtx, command); err != nil {
				return tui.MessageDeleteFailed(err)
			}
			return tui.MessageDeleteSucceeded(command.Sequence)
		}
	}
	loadKeyValueKeys := func(bucket string) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValueKeysFailed(fmt.Errorf("no active NATS connection"))
			}
			queryCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			keys, err := kvstore.NewListKeysHandler(monitor).Handle(queryCtx, bucket)
			if err != nil {
				return tui.KeyValueKeysFailed(err)
			}
			return tui.KeyValueKeysLoaded(bucket, keys)
		}
	}
	loadObjects := func(bucket string) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ObjectsFailed(fmt.Errorf("no active NATS connection"))
			}
			queryCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			objects, err := objectstore.NewListObjectsHandler(monitor).Handle(queryCtx, bucket)
			if err != nil {
				return tui.ObjectsFailed(err)
			}
			return tui.ObjectsLoaded(bucket, objects)
		}
	}
	startObjectUpload := func(command objectstore.UploadFile) tea.Cmd {
		return func() tea.Msg {
			events, err := sessions.StartObjectUpload(ctx, command)
			if err != nil {
				return tui.ObjectTransferFailed(err)
			}
			return tui.ObjectTransferStarted(events)
		}
	}
	startObjectDownload := func(command objectstore.DownloadFile) tea.Cmd {
		return func() tea.Msg {
			events, err := sessions.StartObjectDownload(ctx, command)
			if err != nil {
				return tui.ObjectTransferFailed(err)
			}
			return tui.ObjectTransferStarted(events)
		}
	}
	stopObjectTransfer := func() { sessions.StopObjectTransfer() }
	startStreamBackup := func(command streams.BackupFile) tea.Cmd {
		return func() tea.Msg {
			events, err := sessions.StartStreamBackup(ctx, command)
			if err != nil {
				return tui.StreamTransferFailed(err)
			}
			return tui.StreamTransferStarted(events)
		}
	}
	startStreamRestore := func(command streams.RestoreFile) tea.Cmd {
		return func() tea.Msg {
			events, err := sessions.StartStreamRestore(ctx, command)
			if err != nil {
				return tui.StreamTransferFailed(err)
			}
			return tui.StreamTransferStarted(events)
		}
	}
	stopStreamTransfer := func() { sessions.StopStreamTransfer() }
	startObjectWatch := func(input tui.ObjectWatchInput) tea.Cmd {
		return func() tea.Msg {
			events, err := sessions.StartObjectWatch(ctx, input.Bucket)
			if err != nil {
				return tui.ObjectWatchFailed(input.Generation, err)
			}
			return tui.ObjectWatchStarted(input.Generation, events)
		}
	}
	stopObjectWatch := func() { sessions.StopObjectWatch() }
	editObject := func(patch objectstore.EditObject) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ObjectEditFailed(fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			object, err := objectstore.NewEditObjectHandler(monitor, monitor).Handle(actionCtx, patch)
			if err != nil {
				return tui.ObjectEditFailed(err)
			}
			return tui.ObjectEditSucceeded(object)
		}
	}
	deleteObject := func(bucket, name string) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ObjectDeleteFailed(fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := objectstore.NewDeleteObjectHandler(monitor).Handle(actionCtx, bucket, name); err != nil {
				return tui.ObjectDeleteFailed(err)
			}
			return tui.ObjectDeleteSucceeded(bucket, name)
		}
	}
	createObjectLink := func(definition objectstore.LinkDefinition) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ObjectLinkFailed(fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			object, err := objectstore.NewCreateLinkHandler(monitor).Handle(actionCtx, definition)
			if err != nil {
				return tui.ObjectLinkFailed(err)
			}
			return tui.ObjectLinkSucceeded(object)
		}
	}
	loadKeyValueEntry := func(bucket, key string) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValueEntryFailed(fmt.Errorf("no active NATS connection"))
			}
			queryCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			entry, err := kvstore.NewGetEntryHandler(monitor).Handle(queryCtx, bucket, key)
			if err != nil {
				return tui.KeyValueEntryFailed(err)
			}
			return tui.KeyValueEntryLoaded(entry)
		}
	}
	putKeyValueEntry := func(input tui.KeyValuePutInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValuePutFailed(input.Generation, input.Command, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if _, err := kvstore.NewPutEntryHandler(monitor).Handle(actionCtx, input.Command); err != nil {
				return tui.KeyValuePutFailed(input.Generation, input.Command, err)
			}
			entry, err := kvstore.NewGetEntryHandler(monitor).Handle(actionCtx, input.Command.Bucket, input.Command.Key)
			if err != nil {
				return tui.KeyValuePutFailed(input.Generation, input.Command, err)
			}
			return tui.KeyValuePutSucceeded(input.Generation, entry)
		}
	}
	removeKeyValueEntry := func(input tui.KeyValueRemoveInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValueRemoveFailed(input.Generation, input.Command, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := kvstore.NewRemoveEntryHandler(monitor).Handle(actionCtx, input.Command); err != nil {
				return tui.KeyValueRemoveFailed(input.Generation, input.Command, err)
			}
			return tui.KeyValueRemoveSucceeded(input.Generation, input.Command)
		}
	}
	loadKeyValueHistory := func(bucket, key string) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValueHistoryFailed(fmt.Errorf("no active NATS connection"))
			}
			queryCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			entries, err := kvstore.NewListHistoryHandler(monitor).Handle(queryCtx, bucket, key)
			if err != nil {
				return tui.KeyValueHistoryFailed(err)
			}
			return tui.KeyValueHistoryLoaded(bucket, key, entries)
		}
	}
	restoreKeyValue := func(input tui.KeyValueRestoreInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValueRestoreFailed(input.Generation, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			result, err := kvstore.NewRestoreHandler(monitor, monitor).Handle(actionCtx, input.Command)
			if err != nil {
				return tui.KeyValueRestoreFailed(input.Generation, err)
			}
			return tui.KeyValueRestoreSucceeded(input.Generation, input.Command, result)
		}
	}
	startKeyValueWatch := func(bucket, key string) tea.Cmd {
		return func() tea.Msg {
			events, err := sessions.StartKeyValueWatch(ctx, bucket, key)
			if err != nil {
				return tui.KeyValueWatchFailed(err)
			}
			return tui.KeyValueWatchStarted(events)
		}
	}
	stopKeyValueWatch := func() { sessions.StopKeyValueWatch() }
	startWatch := func(_ string, subjects []string) tea.Cmd {
		return func() tea.Msg {
			events, err := sessions.StartWatch(ctx, subjects)
			if err != nil {
				return tui.WatchFailed(err)
			}
			return tui.WatchStarted(events)
		}
	}
	startOrderedWatch := func(stream string, subjects []string) tea.Cmd {
		return func() tea.Msg {
			events, err := sessions.StartOrderedWatch(ctx, stream, subjects)
			if err != nil {
				return tui.WatchFailed(err)
			}
			return tui.WatchStarted(events)
		}
	}
	stopWatch := func() { sessions.StopWatch() }
	refresh := func(generation uint64) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.RefreshFailed(generation, fmt.Errorf("no active NATS connection"))
			}
			refreshCtx, cancel := r.withTimeout(ctx, refreshTimeout)
			defer cancel()
			state, err := loadWorkspace(refreshCtx, monitor)
			if err != nil {
				return tui.RefreshFailed(generation, err)
			}
			return tui.RefreshedWithObjects(generation, state.snapshot, state.streams, state.consumers, state.buckets, state.objects)
		}
	}
	consumerAction := func(input tui.ConsumerActionInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ConsumerActionFailed(input.Generation, input, fmt.Errorf("no active NATS connection"))
			}
			ref, err := consumers.NewRef(input.Stream, input.Name)
			if err != nil {
				return tui.ConsumerActionFailed(input.Generation, input, err)
			}
			actionCtx, cancel := r.withTimeout(ctx, consumerActionTimeout)
			defer cancel()
			command := consumers.Command{Consumer: ref, Action: input.Action}
			if err := consumers.NewOperateHandler(monitor).Handle(actionCtx, command); err != nil {
				return tui.ConsumerActionFailed(input.Generation, input, err)
			}
			return tui.ConsumerActionSucceeded(input.Generation, input)
		}
	}
	consumerConfigure := func(input tui.ConsumerConfigInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ConsumerConfigFailed(input, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, consumerActionTimeout)
			defer cancel()
			command := consumers.ConfigCommand{Action: input.Action, Definition: input.Definition}
			if err := consumers.NewConfigureHandler(monitor).Handle(actionCtx, command); err != nil {
				return tui.ConsumerConfigFailed(input, err)
			}
			return tui.ConsumerConfigSucceeded(input)
		}
	}
	streamAction := func(input tui.StreamActionInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.StreamActionFailed(input.Generation, input, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			command := streams.Command{Action: input.Action, Name: input.Name, Definition: input.Definition}
			if err := streams.NewOperateHandler(monitor).Handle(actionCtx, command); err != nil {
				return tui.StreamActionFailed(input.Generation, input, err)
			}
			return tui.StreamActionSucceeded(input.Generation, input)
		}
	}
	streamPurge := func(input tui.StreamPurgeInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.StreamPurgeFailed(input, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := streams.NewMaintenanceHandler(monitor).Purge(actionCtx, input.Command); err != nil {
				return tui.StreamPurgeFailed(input, err)
			}
			return tui.StreamPurgeSucceeded(input)
		}
	}
	createKeyValue := func(input tui.KeyValueCreateInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValueCreateFailed(input.Generation, input.Definition, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := kvstore.NewCreateBucketHandler(monitor).Handle(actionCtx, input.Definition); err != nil {
				return tui.KeyValueCreateFailed(input.Generation, input.Definition, err)
			}
			return tui.KeyValueCreateSucceeded(input.Generation, input.Definition)
		}
	}
	updateKeyValue := func(input tui.KeyValueUpdateInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValueUpdateFailed(input.Generation, input.Name, input.Changes, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := kvstore.NewUpdateBucketHandler(monitor).Handle(actionCtx, input.Name, input.Changes); err != nil {
				return tui.KeyValueUpdateFailed(input.Generation, input.Name, input.Changes, err)
			}
			return tui.KeyValueUpdateSucceeded(input.Generation, input.Name, input.Changes)
		}
	}
	deleteKeyValue := func(input tui.KeyValueDeleteInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.KeyValueDeleteFailed(input.Generation, input.Name, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := kvstore.NewDeleteBucketHandler(monitor).Handle(actionCtx, input.Name); err != nil {
				return tui.KeyValueDeleteFailed(input.Generation, input.Name, err)
			}
			return tui.KeyValueDeleteSucceeded(input.Generation, input.Name)
		}
	}
	createObjectBucket := func(input tui.ObjectBucketCreateInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ObjectBucketCreateFailed(input, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := objectstore.NewCreateBucketHandler(monitor).Handle(actionCtx, input.Definition); err != nil {
				return tui.ObjectBucketCreateFailed(input, err)
			}
			return tui.ObjectBucketCreateSucceeded(input)
		}
	}
	updateObjectBucket := func(input tui.ObjectBucketUpdateInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ObjectBucketUpdateFailed(input, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := objectstore.NewUpdateBucketHandler(monitor).Handle(actionCtx, input.Name, input.Changes); err != nil {
				return tui.ObjectBucketUpdateFailed(input, err)
			}
			return tui.ObjectBucketUpdateSucceeded(input)
		}
	}
	deleteObjectBucket := func(input tui.ObjectBucketDeleteInput) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ObjectBucketDeleteFailed(input, fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := objectstore.NewDeleteBucketHandler(monitor).Handle(actionCtx, input.Name); err != nil {
				return tui.ObjectBucketDeleteFailed(input, err)
			}
			return tui.ObjectBucketDeleteSucceeded(input)
		}
	}
	publishCore := func(message domainrequestreply.Message) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.CorePublishFailed(fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := requestreply.NewPublishHandler(monitor).Handle(actionCtx, message); err != nil {
				return tui.CorePublishFailed(err)
			}
			return tui.CorePublishSucceeded(message)
		}
	}
	sendCoreRequest := func(request domainrequestreply.Request) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.CoreRequestFailed(fmt.Errorf("no active NATS connection"))
			}
			history, _ := r.requestRepositories(activeSettings.Profile)
			exchange, err := r.requestHandler(monitor, history).Handle(ctx, request)
			if err != nil {
				return tui.CoreRequestFailed(err)
			}
			return tui.CoreRequestSucceeded(exchange)
		}
	}
	loadPayload := func(path string) tea.Cmd {
		return func() tea.Msg {
			read := r.readPayload
			if read == nil {
				read = os.ReadFile
			}
			payload, err := read(path)
			if err != nil {
				return tui.PayloadFileFailed(path, err)
			}
			return tui.PayloadFileLoaded(path, payload)
		}
	}
	loadRequestHistory := func() tea.Cmd {
		return func() tea.Msg {
			historyStore, _ := r.requestRepositories(activeSettings.Profile)
			history, err := requestreply.NewListHistoryHandler(historyStore).Handle(ctx)
			if err != nil {
				return tui.RequestHistoryFailed(err)
			}
			return tui.RequestHistoryLoaded(history)
		}
	}
	loadRequestTemplates := func() tea.Cmd {
		return func() tea.Msg {
			_, templateStore := r.requestRepositories(activeSettings.Profile)
			templates, err := requestreply.NewListTemplatesHandler(templateStore).Handle(ctx)
			if err != nil {
				return tui.RequestTemplatesFailed(err)
			}
			return tui.RequestTemplatesLoaded(templates)
		}
	}
	saveRequestTemplate := func(template domainrequestreply.Template) tea.Cmd {
		return func() tea.Msg {
			_, templateStore := r.requestRepositories(activeSettings.Profile)
			if err := requestreply.NewSaveTemplateHandler(templateStore).Handle(ctx, template); err != nil {
				return tui.RequestTemplateSaveFailed(err)
			}
			return tui.RequestTemplateSaved(template)
		}
	}
	deleteRequestTemplate := func(name string) tea.Cmd {
		return func() tea.Msg {
			_, templateStore := r.requestRepositories(activeSettings.Profile)
			if err := requestreply.NewDeleteTemplateHandler(templateStore).Handle(ctx, name); err != nil {
				return tui.RequestTemplateDeleteFailed(err)
			}
			return tui.RequestTemplateDeleted(name)
		}
	}
	loadTopology := func() tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.TopologyFailed(fmt.Errorf("no active NATS connection"))
			}
			queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
			defer cancel()
			snapshot, err := topology.NewGetHandler(monitor).Handle(queryCtx)
			if err != nil {
				return tui.TopologyFailed(err)
			}
			return tui.TopologyLoaded(snapshot)
		}
	}
	operateTopology := func(command topology.Command) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.TopologyOperationFailed(fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := topology.NewOperateHandler(monitor).Handle(actionCtx, command); err != nil {
				return tui.TopologyOperationFailed(err)
			}
			return tui.TopologyOperationSucceeded(command)
		}
	}
	loadConnections := func() tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ConnectionsFailed(fmt.Errorf("no active NATS connection"))
			}
			queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
			defer cancel()
			snapshot, err := connections.NewListHandler(monitor).Handle(queryCtx)
			if err != nil {
				return tui.ConnectionsFailed(err)
			}
			return tui.ConnectionsLoaded(snapshot)
		}
	}
	loadOperator := func() tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.OperatorFailed(fmt.Errorf("no active NATS connection"))
			}
			queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
			defer cancel()
			catalog, err := r.operatorCatalog(queryCtx, monitor, settings)
			if err != nil {
				return tui.OperatorFailed(err)
			}
			return tui.OperatorLoaded(catalog)
		}
	}
	createOperatorAccount := func(definition domainauthority.AccountDefinition) tea.Cmd {
		return func() tea.Msg {
			if settings.AuthorityDir == "" || settings.OperatorSeed == "" {
				return tui.OperatorAccountCreateFailed(fmt.Errorf("configure --authority-dir and --authority-operator-seed"))
			}
			monitor := sessions.Current()
			if monitor == nil {
				return tui.OperatorAccountCreateFailed(fmt.Errorf("no active NATS connection"))
			}
			seed, err := r.payloadReader()(settings.OperatorSeed)
			if err != nil {
				return tui.OperatorAccountCreateFailed(fmt.Errorf("read operator seed: %w", err))
			}
			signer, err := r.authoritySigner(seed)
			if err != nil {
				return tui.OperatorAccountCreateFailed(err)
			}
			store, err := r.authorityStore(settings.AuthorityDir)
			if err != nil {
				return tui.OperatorAccountCreateFailed(err)
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			artifacts, err := operator.NewCreateAccountHandler(signer, store, monitor).Handle(actionCtx, definition)
			if err != nil {
				return tui.OperatorAccountCreateFailed(err)
			}
			return tui.OperatorAccountCreated(artifacts)
		}
	}
	createOperatorUser := func(definition domainauthority.UserDefinition) tea.Cmd {
		return func() tea.Msg {
			if settings.AuthorityDir == "" {
				return tui.OperatorUserCreateFailed(fmt.Errorf("configure --authority-dir"))
			}
			repository, err := r.authorityStore(settings.AuthorityDir)
			if err != nil {
				return tui.OperatorUserCreateFailed(err)
			}
			seed, err := repository.AccountSeed(ctx, definition.Account())
			if err != nil {
				return tui.OperatorUserCreateFailed(err)
			}
			signer, err := r.accountSigner(seed)
			if err != nil {
				return tui.OperatorUserCreateFailed(err)
			}
			artifacts, err := operator.NewCreateUserHandler(signer, repository).Handle(ctx, seed, definition)
			if err != nil {
				return tui.OperatorUserCreateFailed(err)
			}
			return tui.OperatorUserCreated(artifacts)
		}
	}
	operatorMutationDeps := func() (operator.AuthoritySigner, operator.AuthorityRepository, Session, error) {
		if settings.AuthorityDir == "" || settings.OperatorSeed == "" {
			return nil, nil, nil, fmt.Errorf("configure --authority-dir and --authority-operator-seed")
		}
		monitor := sessions.Current()
		if monitor == nil {
			return nil, nil, nil, fmt.Errorf("no active NATS connection")
		}
		seed, err := r.payloadReader()(settings.OperatorSeed)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read operator seed: %w", err)
		}
		signer, err := r.authoritySigner(seed)
		if err != nil {
			return nil, nil, nil, err
		}
		repository, err := r.authorityStore(settings.AuthorityDir)
		if err != nil {
			return nil, nil, nil, err
		}
		return signer, repository, monitor, nil
	}
	updateOperatorAccount := func(accountID string, definition domainauthority.AccountDefinition) tea.Cmd {
		return func() tea.Msg {
			signer, repository, monitor, err := operatorMutationDeps()
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			artifacts, err := operator.NewUpdateAccountHandler(signer, repository, monitor).Handle(actionCtx, accountID, definition)
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			return tui.OperatorMutationSucceeded("account claim saved to " + artifacts.JWTPath)
		}
	}
	deleteOperatorAccount := func(accountID string) tea.Cmd {
		return func() tea.Msg {
			signer, repository, monitor, err := operatorMutationDeps()
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			trash, err := operator.NewDeleteAccountHandler(signer, repository, monitor).Handle(actionCtx, accountID)
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			return tui.OperatorMutationSucceeded("account deleted; recoverable at " + trash)
		}
	}
	updateOperatorUser := func(userID string, definition domainauthority.UserDefinition) tea.Cmd {
		return func() tea.Msg {
			if settings.AuthorityDir == "" {
				return tui.OperatorMutationFailed(fmt.Errorf("configure --authority-dir"))
			}
			repository, err := r.authorityStore(settings.AuthorityDir)
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			seed, err := repository.AccountSeed(ctx, definition.Account())
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			signer, err := r.accountSigner(seed)
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			artifacts, err := operator.NewUpdateUserHandler(signer, repository).Handle(ctx, userID, definition)
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			return tui.OperatorMutationSucceeded("user credentials saved to " + artifacts.CredentialsPath)
		}
	}
	revokeOperatorUser := func(accountID, userID string) tea.Cmd {
		return func() tea.Msg {
			signer, repository, monitor, err := operatorMutationDeps()
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if _, err := operator.NewRevokeUserHandler(signer, repository, monitor).Handle(actionCtx, accountID, userID, r.clock.Now()); err != nil {
				return tui.OperatorMutationFailed(err)
			}
			return tui.OperatorMutationSucceeded("user revoked")
		}
	}
	deleteOperatorUser := func(accountID, userID string) tea.Cmd {
		return func() tea.Msg {
			signer, repository, monitor, err := operatorMutationDeps()
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			trash, err := operator.NewDeleteUserHandler(signer, repository, monitor).Handle(actionCtx, accountID, userID, r.clock.Now())
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			return tui.OperatorMutationSucceeded("user deleted; recoverable at " + trash)
		}
	}
	addOperatorSigningKey := func(accountID string, definition domainauthority.SigningKeyDefinition) tea.Cmd {
		return func() tea.Msg {
			signer, repository, monitor, err := operatorMutationDeps()
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			artifacts, err := operator.NewAddSigningKeyHandler(signer, repository, monitor).Handle(actionCtx, accountID, definition)
			if err != nil {
				return tui.OperatorMutationFailed(err)
			}
			return tui.OperatorMutationSucceeded("signing seed saved to " + artifacts.SeedPath)
		}
	}
	closeConnection := func(serverID string, cid uint64) tea.Cmd {
		return func() tea.Msg {
			monitor := sessions.Current()
			if monitor == nil {
				return tui.ConnectionCloseFailed(fmt.Errorf("no active NATS connection"))
			}
			actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
			defer cancel()
			if err := connections.NewCloseHandler(monitor).Handle(actionCtx, serverID, cid); err != nil {
				return tui.ConnectionCloseFailed(err)
			}
			return tui.ConnectionClosed(serverID, cid)
		}
	}
	loadAlerts := func(query alerts.Query) tea.Cmd {
		return func() tea.Msg {
			records, err := r.ListAlerts(ctx, activeSettings, query)
			if err != nil {
				return tui.AlertsFailed(err)
			}
			return tui.AlertsLoaded(records)
		}
	}
	watchAlerts := func() tea.Cmd {
		return func() tea.Msg {
			if alertCancel != nil {
				alertCancel()
			}
			watchCtx, cancel := context.WithCancel(ctx)
			alertCancel = cancel
			records, failures, err := r.WatchAlerts(watchCtx, activeSettings)
			if err != nil {
				cancel()
				return tui.AlertsFailed(err)
			}
			visible := make(chan alerts.Record, 256)
			go func() {
				defer close(visible)
				for records != nil || failures != nil {
					select {
					case <-watchCtx.Done():
						return
					case record, ok := <-records:
						if !ok {
							records = nil
							continue
						}
						select {
						case visible <- record:
						case <-watchCtx.Done():
							return
						}
					case watchErr, ok := <-failures:
						if !ok {
							failures = nil
							continue
						}
						_ = watchErr // malformed advisories do not terminate the inbox
					}
				}
			}()
			return tui.AlertWatchStarted(visible)
		}
	}
	ackAlert := func(id string) tea.Cmd {
		return func() tea.Msg {
			if err := r.AcknowledgeAlert(ctx, activeSettings, id); err != nil {
				return tui.AlertActionFailed(err)
			}
			return tui.AlertAcked(id)
		}
	}
	ackAllAlerts := func() tea.Cmd {
		return func() tea.Msg {
			count, err := r.AcknowledgeAllAlerts(ctx, activeSettings)
			if err != nil {
				return tui.AlertActionFailed(err)
			}
			return tui.AlertsAcked(count)
		}
	}
	scanAlerts := func() tea.Cmd {
		return func() tea.Msg {
			results, err := r.ScanAlerts(ctx, activeSettings)
			if err != nil {
				return tui.AlertActionFailed(err)
			}
			count := 0
			for _, result := range results {
				if result.Stored {
					count++
				}
			}
			return tui.AlertsScanned(count)
		}
	}
	loadCommandBar := func() tea.Cmd {
		return func() tea.Msg {
			custom, err := r.ListCustomCommands(ctx, activeSettings)
			if err != nil {
				return tui.CommandBarFailed(err)
			}
			history, err := r.ListCommandHistory(ctx, activeSettings, 100)
			if err != nil {
				return tui.CommandBarFailed(err)
			}
			return tui.CommandBarLoaded(custom, history)
		}
	}
	recordCommand := func(id, title string, started, completed time.Time, executionErr error) tea.Cmd {
		return func() tea.Msg {
			if err := r.RecordCommandExecution(ctx, activeSettings, id, title, started, completed, executionErr); err != nil {
				return tui.CommandBarFailed(err)
			}
			return tui.CommandRecorded()
		}
	}
	loadBookmarks := func(available []appbookmarks.TargetView) tea.Cmd {
		return func() tea.Msg {
			items, err := r.ListBookmarks(ctx, activeSettings, available)
			if err != nil {
				return tui.BookmarksFailed(err)
			}
			return tui.BookmarksLoaded(items)
		}
	}
	addBookmark := func(command appbookmarks.AddCommand) tea.Cmd {
		return func() tea.Msg {
			item, err := r.AddBookmark(ctx, activeSettings, command)
			if err != nil {
				return tui.BookmarksFailed(err)
			}
			return tui.BookmarkAdded(item)
		}
	}
	removeBookmark := func(id string) tea.Cmd {
		return func() tea.Msg {
			if err := r.DeleteBookmark(ctx, activeSettings, id); err != nil {
				return tui.BookmarksFailed(err)
			}
			return tui.BookmarkRemoved(id)
		}
	}
	saveSearch := func(scope, query string) tea.Cmd {
		return func() tea.Msg {
			if err := r.SaveRecentSearch(ctx, activeSettings, scope, query); err != nil {
				return tui.BookmarksFailed(err)
			}
			return tui.RecentSearchSaved()
		}
	}

	model := tui.NewModel(settings.Profile, settings.Server, connect, loadMessages).
		WithMessageWatch(startWatch, stopWatch).
		WithOrderedMessageWatch(startOrderedWatch).
		WithMessageHistory(loadMessageHistory).
		WithMessageOperations(exportMessages, deleteMessage).
		WithSubjectObservation(startSubjectObservation, stopSubjectObservation).
		WithRefresh(refresh).
		WithConsumerActions(consumerAction).
		WithConsumerConfiguration(consumerConfigure).
		WithStreamActions(streamAction).
		WithStreamPurge(streamPurge).
		WithStreamTransfers(startStreamBackup, startStreamRestore, stopStreamTransfer).
		WithKeyValueCreate(createKeyValue).
		WithKeyValueUpdate(updateKeyValue).
		WithKeyValueDelete(deleteKeyValue).
		WithKeyValueKeys(loadKeyValueKeys).
		WithKeyValueEntry(loadKeyValueEntry).
		WithKeyValuePut(putKeyValueEntry).
		WithKeyValueRemove(removeKeyValueEntry).
		WithKeyValueHistory(loadKeyValueHistory).
		WithKeyValueRestore(restoreKeyValue).
		WithKeyValueWatch(startKeyValueWatch, stopKeyValueWatch).
		WithCopyValue(r.copier()).
		WithObjectCatalog(loadObjects).
		WithObjectTransfers(startObjectUpload, startObjectDownload, stopObjectTransfer).
		WithObjectOperations(editObject, deleteObject).
		WithObjectWatch(startObjectWatch, stopObjectWatch).
		WithObjectLinks(createObjectLink).
		WithObjectBucketCreate(createObjectBucket).
		WithObjectBucketUpdate(updateObjectBucket).
		WithObjectBucketDelete(deleteObjectBucket).
		WithRequestReply(tui.RequestReplyServices{
			Publish: publishCore, Request: sendCoreRequest, LoadPayload: loadPayload,
			LoadHistory: loadRequestHistory, LoadTemplates: loadRequestTemplates,
			SaveTemplate: saveRequestTemplate, DeleteTemplate: deleteRequestTemplate,
		}).
		WithTopology(tui.TopologyServices{Load: loadTopology, Operate: operateTopology})
	model = model.WithConnections(tui.ConnectionServices{Load: loadConnections, Close: closeConnection})
	model = model.WithConnectionProfiles(tui.ProfileServices{Load: loadProfiles})
	model = model.WithOperator(tui.OperatorServices{Load: loadOperator, CreateAccount: createOperatorAccount, CreateUser: createOperatorUser, UpdateAccount: updateOperatorAccount, DeleteAccount: deleteOperatorAccount, UpdateUser: updateOperatorUser, RevokeUser: revokeOperatorUser, DeleteUser: deleteOperatorUser, AddSigningKey: addOperatorSigningKey})
	model = model.WithAlerts(tui.AlertServices{Load: loadAlerts, Watch: watchAlerts, Stop: func() {
		if alertCancel != nil {
			alertCancel()
		}
	}, Ack: ackAlert, AckAll: ackAllAlerts, Scan: scanAlerts})
	model = model.WithCommandBar(tui.CommandBarServices{Load: loadCommandBar, Record: recordCommand})
	model = model.WithBookmarks(tui.BookmarkServices{Load: loadBookmarks, Add: addBookmark, Remove: removeBookmark, SaveSearch: saveSearch})
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run terminal UI: %w", err)
	}
	return nil
}

func loadWorkspace(ctx context.Context, monitor Session) (workspaceState, error) {
	snapshot, err := dashboard.NewGetSnapshotHandler(monitor).Handle(ctx)
	if err != nil {
		return workspaceState{}, err
	}
	streamCatalog, err := streams.NewListStreamsHandler(monitor).Handle(ctx)
	if err != nil {
		return workspaceState{}, err
	}
	consumerCatalog, err := consumers.NewListConsumersHandler(monitor).Handle(ctx)
	if err != nil {
		return workspaceState{}, err
	}
	bucketCatalog, err := kvstore.NewListBucketsHandler(monitor).Handle(ctx)
	if err != nil {
		return workspaceState{}, err
	}
	objectCatalog, err := objectstore.NewListBucketsHandler(monitor).Handle(ctx)
	if err != nil {
		return workspaceState{}, err
	}
	return workspaceState{snapshot: snapshot, streams: streamCatalog, consumers: consumerCatalog, buckets: bucketCatalog, objects: objectCatalog}, nil
}

type sessionStore struct {
	mu                   sync.Mutex
	monitor              Session
	watchCancel          context.CancelFunc
	keyValueWatchCancel  context.CancelFunc
	objectTransferCancel context.CancelFunc
	streamTransferCancel context.CancelFunc
	objectWatchCancel    context.CancelFunc
	objectFiles          localfiles.ObjectTransfer
	streamArchive        localfiles.StreamArchive
}

func (s *sessionStore) Replace(monitor Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.watchCancel != nil {
		s.watchCancel()
		s.watchCancel = nil
	}
	if s.keyValueWatchCancel != nil {
		s.keyValueWatchCancel()
		s.keyValueWatchCancel = nil
	}
	if s.objectTransferCancel != nil {
		s.objectTransferCancel()
		s.objectTransferCancel = nil
	}
	if s.streamTransferCancel != nil {
		s.streamTransferCancel()
		s.streamTransferCancel = nil
	}
	if s.objectWatchCancel != nil {
		s.objectWatchCancel()
		s.objectWatchCancel = nil
	}
	if s.monitor != nil {
		s.monitor.Close()
	}
	s.monitor = monitor
}

func (s *sessionStore) Current() Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.monitor
}

func (s *sessionStore) StartWatch(ctx context.Context, subjects []string) (<-chan messages.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monitor == nil {
		return nil, fmt.Errorf("no active NATS connection")
	}
	if s.watchCancel != nil {
		s.watchCancel()
	}
	watchCtx, cancel := context.WithCancel(ctx)
	handler := messages.NewWatchHandler(s.monitor)
	events, err := handler.Handle(watchCtx, subjects)
	if err != nil {
		cancel()
		return nil, err
	}
	s.watchCancel = cancel
	return events, nil
}

func (s *sessionStore) StartOrderedWatch(ctx context.Context, stream string, subjects []string) (<-chan messages.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monitor == nil {
		return nil, fmt.Errorf("no active NATS connection")
	}
	if s.watchCancel != nil {
		s.watchCancel()
	}
	watchCtx, cancel := context.WithCancel(ctx)
	events, err := s.monitor.WatchOrdered(watchCtx, stream, subjects)
	if err != nil {
		cancel()
		return nil, err
	}
	s.watchCancel = cancel
	return events, nil
}

func (s *sessionStore) StopWatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watchCancel != nil {
		s.watchCancel()
		s.watchCancel = nil
	}
}

func (s *sessionStore) StartKeyValueWatch(ctx context.Context, bucket, key string) (<-chan kvstore.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monitor == nil {
		return nil, fmt.Errorf("no active NATS connection")
	}
	if s.keyValueWatchCancel != nil {
		s.keyValueWatchCancel()
	}
	watchCtx, cancel := context.WithCancel(ctx)
	events, err := kvstore.NewWatchHandler(s.monitor).Handle(watchCtx, bucket, key)
	if err != nil {
		cancel()
		return nil, err
	}
	s.keyValueWatchCancel = cancel
	return events, nil
}

func (s *sessionStore) StopKeyValueWatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keyValueWatchCancel != nil {
		s.keyValueWatchCancel()
		s.keyValueWatchCancel = nil
	}
}

func (s *sessionStore) StartObjectWatch(ctx context.Context, bucket string) (<-chan objectstore.ObjectEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monitor == nil {
		return nil, fmt.Errorf("no active NATS connection")
	}
	if s.objectWatchCancel != nil {
		s.objectWatchCancel()
	}
	watchCtx, cancel := context.WithCancel(ctx)
	events, err := objectstore.NewWatchObjectsHandler(s.monitor).Handle(watchCtx, bucket)
	if err != nil {
		cancel()
		return nil, err
	}
	s.objectWatchCancel = cancel
	return events, nil
}

func (s *sessionStore) StopObjectWatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objectWatchCancel != nil {
		s.objectWatchCancel()
		s.objectWatchCancel = nil
	}
}

func (s *sessionStore) StartObjectUpload(ctx context.Context, command objectstore.UploadFile) (<-chan objectstore.TransferEvent, error) {
	return s.startObjectTransfer(ctx, func(transferCtx context.Context, monitor Session, progress objectstore.ProgressFunc) objectstore.TransferEvent {
		object, err := s.objectFiles.UploadObject(transferCtx, monitor, command, progress)
		return objectstore.TransferEvent{Object: object, Err: err, Done: true}
	})
}

func (s *sessionStore) StartObjectDownload(ctx context.Context, command objectstore.DownloadFile) (<-chan objectstore.TransferEvent, error) {
	return s.startObjectTransfer(ctx, func(transferCtx context.Context, monitor Session, progress objectstore.ProgressFunc) objectstore.TransferEvent {
		result, err := s.objectFiles.DownloadObject(transferCtx, monitor, command, progress)
		return objectstore.TransferEvent{Object: result.Object, Destination: command.Path, Verified: result.Verified, Err: err, Done: true}
	})
}

func (s *sessionStore) startObjectTransfer(ctx context.Context, run func(context.Context, Session, objectstore.ProgressFunc) objectstore.TransferEvent) (<-chan objectstore.TransferEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monitor == nil {
		return nil, fmt.Errorf("no active NATS connection")
	}
	if s.objectTransferCancel != nil {
		s.objectTransferCancel()
	}
	transferCtx, cancel := context.WithCancel(ctx)
	s.objectTransferCancel = cancel
	events := make(chan objectstore.TransferEvent, 32)
	monitor := s.monitor
	go func() {
		defer close(events)
		progress := func(update objectstore.Progress) {
			select {
			case events <- objectstore.TransferEvent{Progress: update}:
			case <-transferCtx.Done():
			}
		}
		terminal := run(transferCtx, monitor, progress)
		select {
		case events <- terminal:
		case <-ctx.Done():
		}
	}()
	return events, nil
}

func (s *sessionStore) StopObjectTransfer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objectTransferCancel != nil {
		s.objectTransferCancel()
		s.objectTransferCancel = nil
	}
}

func (s *sessionStore) StartStreamBackup(ctx context.Context, command streams.BackupFile) (<-chan streams.TransferEvent, error) {
	return s.startStreamTransfer(ctx, func(transferCtx context.Context, gateway streams.SnapshotGateway, progress streams.TransferProgressFunc) streams.TransferEvent {
		result, err := s.streamArchive.Backup(transferCtx, gateway, command, progress)
		return streams.TransferEvent{Backup: result, Directory: command.Directory, Err: err, Done: true}
	})
}

func (s *sessionStore) StartStreamRestore(ctx context.Context, command streams.RestoreFile) (<-chan streams.TransferEvent, error) {
	return s.startStreamTransfer(ctx, func(transferCtx context.Context, gateway streams.SnapshotGateway, progress streams.TransferProgressFunc) streams.TransferEvent {
		result, err := s.streamArchive.Restore(transferCtx, gateway, command, progress)
		return streams.TransferEvent{Restore: result, Directory: command.Directory, Err: err, Done: true}
	})
}

func (s *sessionStore) startStreamTransfer(ctx context.Context, run func(context.Context, streams.SnapshotGateway, streams.TransferProgressFunc) streams.TransferEvent) (<-chan streams.TransferEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monitor == nil {
		return nil, fmt.Errorf("no active NATS connection")
	}
	gateway, ok := s.monitor.(streams.SnapshotGateway)
	if !ok {
		return nil, fmt.Errorf("NATS session does not support stream snapshots")
	}
	if s.streamTransferCancel != nil {
		s.streamTransferCancel()
	}
	transferCtx, cancel := context.WithCancel(ctx)
	s.streamTransferCancel = cancel
	events := make(chan streams.TransferEvent, 32)
	go func() {
		defer close(events)
		progress := func(update streams.TransferProgress) {
			select {
			case events <- streams.TransferEvent{Progress: update}:
			case <-transferCtx.Done():
			}
		}
		terminal := run(transferCtx, gateway, progress)
		select {
		case events <- terminal:
		case <-ctx.Done():
		}
	}()
	return events, nil
}

func (s *sessionStore) StopStreamTransfer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamTransferCancel != nil {
		s.streamTransferCancel()
		s.streamTransferCancel = nil
	}
}

func (s *sessionStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.monitor != nil {
		if s.watchCancel != nil {
			s.watchCancel()
			s.watchCancel = nil
		}
		if s.keyValueWatchCancel != nil {
			s.keyValueWatchCancel()
			s.keyValueWatchCancel = nil
		}
		if s.objectTransferCancel != nil {
			s.objectTransferCancel()
			s.objectTransferCancel = nil
		}
		if s.streamTransferCancel != nil {
			s.streamTransferCancel()
			s.streamTransferCancel = nil
		}
		if s.objectWatchCancel != nil {
			s.objectWatchCancel()
			s.objectWatchCancel = nil
		}
		s.monitor.Close()
		s.monitor = nil
	}
}
