package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fd1az/nats-tui/internal/app/connections"
	"github.com/fd1az/nats-tui/internal/app/consumers"
	"github.com/fd1az/nats-tui/internal/app/kvstore"
	"github.com/fd1az/nats-tui/internal/app/messages"
	"github.com/fd1az/nats-tui/internal/app/objectstore"
	"github.com/fd1az/nats-tui/internal/app/streams"
	"github.com/fd1az/nats-tui/internal/app/subjects"
	"github.com/fd1az/nats-tui/internal/config"
	domainrequestreply "github.com/fd1az/nats-tui/internal/domain/requestreply"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Runner interface {
	Run(context.Context, config.Settings) error
}

type KeyValueCreator interface {
	CreateKeyValueBucket(context.Context, config.Settings, kvstore.Definition) error
}

type KeyValueUpdater interface {
	UpdateKeyValueBucket(context.Context, config.Settings, string, kvstore.Changes) error
}

type KeyValueDeleter interface {
	DeleteKeyValueBucket(context.Context, config.Settings, string) error
}

type KeyValueKeyLister interface {
	ListKeyValueKeys(context.Context, config.Settings, string) ([]string, error)
}

type KeyValueEntryReader interface {
	GetKeyValueEntry(context.Context, config.Settings, string, string) (kvstore.Entry, error)
}

type KeyValueEntryWriter interface {
	PutKeyValueEntry(context.Context, config.Settings, kvstore.WriteEntry) (uint64, error)
}

type KeyValueEntryRemover interface {
	RemoveKeyValueEntry(context.Context, config.Settings, kvstore.RemoveEntry) error
}

type KeyValueHistoryReader interface {
	ListKeyValueHistory(context.Context, config.Settings, string, string) ([]kvstore.Entry, error)
}

type KeyValueRestorer interface {
	RestoreKeyValueEntry(context.Context, config.Settings, kvstore.RestoreEntry) (kvstore.RestoredEntry, error)
}

type KeyValueWatcher interface {
	WatchKeyValue(context.Context, config.Settings, string, string) (<-chan kvstore.Entry, error)
}

type ObjectTransfer interface {
	UploadObjectFile(context.Context, config.Settings, objectstore.UploadFile, objectstore.ProgressFunc) (objectstore.ObjectSummary, error)
	DownloadObjectFile(context.Context, config.Settings, objectstore.DownloadFile, objectstore.ProgressFunc) (objectstore.DownloadResult, error)
}

type ObjectEditor interface {
	EditObject(context.Context, config.Settings, objectstore.EditObject) (objectstore.ObjectSummary, error)
}

type ObjectDeleter interface {
	DeleteObject(context.Context, config.Settings, string, string) error
}

type ObjectLinkCreator interface {
	CreateObjectLink(context.Context, config.Settings, objectstore.LinkDefinition) (objectstore.ObjectSummary, error)
}

type ObjectBucketCreator interface {
	CreateObjectStoreBucket(context.Context, config.Settings, objectstore.Definition) error
}

type ObjectBucketUpdater interface {
	UpdateObjectStoreBucket(context.Context, config.Settings, string, objectstore.Changes) error
}

type ObjectBucketDeleter interface {
	DeleteObjectStoreBucket(context.Context, config.Settings, string) error
}

type StreamManager interface {
	ListStreamCatalog(context.Context, config.Settings) ([]streams.Summary, error)
	OperateStreamConfig(context.Context, config.Settings, streams.Command) error
	PurgeStreamMessages(context.Context, config.Settings, streams.PurgeCommand) error
	DeleteStoredStreamMessage(context.Context, config.Settings, streams.MessageDeleteCommand) error
	BackupStream(context.Context, config.Settings, streams.BackupFile, streams.TransferProgressFunc) (streams.BackupResult, error)
	RestoreStream(context.Context, config.Settings, streams.RestoreFile, streams.TransferProgressFunc) (streams.RestoreResult, error)
}

type ConsumerManager interface {
	ListConsumerCatalog(context.Context, config.Settings) ([]consumers.Summary, error)
	OperateConsumerConfig(context.Context, config.Settings, consumers.ConfigCommand) error
	OperateConsumer(context.Context, config.Settings, consumers.Command) error
}

type CorePublisher interface {
	PublishCoreMessage(context.Context, config.Settings, domainrequestreply.Message) error
}

type MessageManager interface {
	BrowseStoredMessages(context.Context, config.Settings, messages.HistoryQuery) ([]messages.Record, error)
	ExportStoredMessages(context.Context, config.Settings, messages.ExportRequest) (messages.ExportResult, error)
}

type SubjectExplorer interface {
	ExploreSubjects(context.Context, config.Settings) ([]subjects.Node, error)
}

type ProfileManager interface {
	SaveConnectionProfile(context.Context, connections.ProfileInput) error
	ListConnectionProfiles(context.Context) ([]connections.ProfileSummary, error)
	DeleteConnectionProfile(context.Context, string) error
	TestConnectionProfile(context.Context, string) (connections.ProfileTestResult, error)
	ResolveConnectionProfile(context.Context, string, config.Settings) (config.Settings, error)
	ImportNATSContexts(context.Context, string, bool) (connections.ImportResult, error)
}

type CoreRequester interface {
	SendCoreRequest(context.Context, config.Settings, domainrequestreply.Request) (domainrequestreply.Exchange, error)
}

type RequestHistory interface {
	ListRequestHistory(context.Context, config.Settings) ([]domainrequestreply.Exchange, error)
	ReplayCoreRequest(context.Context, config.Settings, string) (domainrequestreply.Exchange, error)
}

type RequestTemplates interface {
	SaveRequestTemplate(context.Context, config.Settings, domainrequestreply.Template) error
	ListRequestTemplates(context.Context, config.Settings) ([]domainrequestreply.Template, error)
	GetRequestTemplate(context.Context, config.Settings, string) (domainrequestreply.Template, error)
	DeleteRequestTemplate(context.Context, config.Settings, string) error
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type FileSystem interface {
	ReadFile(string) ([]byte, error)
	OpenFile(string, int, os.FileMode) (io.WriteCloser, error)
}

type osFileSystem struct{}

func (osFileSystem) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }
func (osFileSystem) OpenFile(name string, flags int, mode os.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(name, flags, mode)
}

type Dependencies struct {
	Runner                 Runner
	KeyValueCreator        KeyValueCreator
	KeyValueUpdater        KeyValueUpdater
	KeyValueDeleter        KeyValueDeleter
	KeyValueKeyLister      KeyValueKeyLister
	KeyValueEntryReader    KeyValueEntryReader
	KeyValueEntryWriter    KeyValueEntryWriter
	KeyValueEntryRemover   KeyValueEntryRemover
	KeyValueHistoryReader  KeyValueHistoryReader
	KeyValueRestorer       KeyValueRestorer
	KeyValueWatcher        KeyValueWatcher
	ObjectTransfer         ObjectTransfer
	ObjectEditor           ObjectEditor
	ObjectDeleter          ObjectDeleter
	ObjectLinkCreator      ObjectLinkCreator
	ObjectBucketCreator    ObjectBucketCreator
	ObjectBucketUpdater    ObjectBucketUpdater
	ObjectBucketDeleter    ObjectBucketDeleter
	Streams                StreamManager
	Consumers              ConsumerManager
	CorePublisher          CorePublisher
	Messages               MessageManager
	Subjects               SubjectExplorer
	Profiles               ProfileManager
	CoreRequester          CoreRequester
	RequestHistory         RequestHistory
	RequestTemplates       RequestTemplates
	TopologyReader         TopologyReader
	TopologyOperator       TopologyOperator
	ConnectionReader       ConnectionReader
	ConnectionCloser       ConnectionCloser
	OperatorReader         OperatorReader
	OperatorAccountCreator OperatorAccountCreator
	OperatorUserCreator    OperatorUserCreator
	OperatorMutator        OperatorMutator
	Alerts                 AlertManager
	Commands               CommandManager
	Bookmarks              BookmarkManager
	Dashboard              DashboardManager
	Clock                  Clock
	FileSystem             FileSystem
	Version                BuildInfo
}

func NewRoot(deps Dependencies) *cobra.Command {
	values := viper.New()
	var configFile string

	root := &cobra.Command{
		Use:           "nats-tui",
		Short:         "A beautiful terminal workspace for NATS JetStream",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := config.Configure(values, configFile); err != nil {
				return err
			}
			profileFlag := cmd.Flag("profile")
			serverFlag := cmd.Flag("server")
			isProfileCommand := cmd.Name() == "profile" || cmd.Parent() != nil && cmd.Parent().Name() == "profile"
			if deps.Profiles == nil || isProfileCommand || profileFlag == nil || !profileFlag.Changed || serverFlag != nil && serverFlag.Changed {
				return nil
			}
			resolved, err := deps.Profiles.ResolveConnectionProfile(cmd.Context(), values.GetString("profile"), config.Load(values))
			if err != nil {
				return err
			}
			for key, value := range map[string]any{"profile": resolved.Profile, "server": resolved.Server, "monitor-url": resolved.MonitorURL, "credentials": resolved.Credentials, "token": resolved.Token, "username": resolved.Username, "password": resolved.Password, "nkey-seed": resolved.NKeySeed, "user-jwt": resolved.UserJWT, "user-seed": resolved.UserSeed, "tls-ca": resolved.TLSCA, "tls-cert": resolved.TLSCert, "tls-key": resolved.TLSKey, "tls-server-name": resolved.TLSServerName, "tls-insecure": resolved.TLSInsecure, "refresh": resolved.Refresh} {
				values.Set(key, value)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.Runner == nil {
				return errors.New("TUI runner is not configured")
			}
			return deps.Runner.Run(cmd.Context(), config.Load(values))
		},
	}

	flags := root.PersistentFlags()
	flags.StringVar(&configFile, "config", "", "config file")
	flags.String("profile", "local", "connection profile name")
	flags.String("server", "nats://127.0.0.1:4222", "NATS server URL")
	flags.String("monitor-url", "", "optional NATS HTTP monitoring URL (for example http://127.0.0.1:8222)")
	flags.String("credentials", "", "NATS credentials file")
	flags.String("token", "", "NATS authentication token")
	flags.String("username", "", "NATS authentication username")
	flags.String("password", "", "NATS authentication password")
	flags.String("nkey-seed", "", "NKey seed file")
	flags.String("user-jwt", "", "user JWT file")
	flags.String("user-seed", "", "user NKey seed file")
	flags.String("tls-ca", "", "TLS CA certificate file")
	flags.String("tls-cert", "", "TLS client certificate file")
	flags.String("tls-key", "", "TLS client private key file")
	flags.String("tls-server-name", "", "TLS server name override")
	flags.Bool("tls-insecure", false, "skip TLS certificate verification (unsafe)")
	flags.String("authority-dir", "", "private directory for locally managed account and user credentials")
	flags.String("authority-operator-seed", "", "operator seed file enabling account mutations")
	flags.Duration("refresh", 2*time.Second, "dashboard refresh interval")
	flags.Bool("mouse", false, "enable terminal mouse navigation and scrolling")
	flags.String("theme", "midnight", "color theme: midnight, accessible, or mono")
	flags.String("key-quit", "q", "global quit key")
	flags.String("key-help", "?", "global help key")
	flags.String("key-refresh", "r", "global refresh key")
	flags.String("key-commands", ":", "global command bar key")
	flags.String("key-bookmarks", "b", "global bookmarks key")
	flags.String("key-activity", "ctrl+g", "global activity log key")

	for _, name := range []string{"profile", "server", "monitor-url", "credentials", "token", "username", "password", "nkey-seed", "user-jwt", "user-seed", "tls-ca", "tls-cert", "tls-key", "tls-server-name", "tls-insecure", "authority-dir", "authority-operator-seed", "refresh", "mouse", "theme", "key-quit", "key-help", "key-refresh", "key-commands", "key-bookmarks", "key-activity"} {
		if err := values.BindPFlag(name, flags.Lookup(name)); err != nil {
			panic(fmt.Sprintf("bind flag %s: %v", name, err))
		}
	}

	root.AddCommand(newVersionCommand(deps.Version))
	root.AddCommand(newCompletionCommand(root))
	files := deps.FileSystem
	if files == nil {
		files = osFileSystem{}
	}
	root.AddCommand(newKeyValueCommand(values, deps.KeyValueCreator, deps.KeyValueUpdater, deps.KeyValueDeleter, deps.KeyValueKeyLister, deps.KeyValueEntryReader, deps.KeyValueEntryWriter, deps.KeyValueEntryRemover, deps.KeyValueHistoryReader, deps.KeyValueRestorer, deps.KeyValueWatcher, files))
	clock := deps.Clock
	if clock == nil {
		clock = systemClock{}
	}
	root.AddCommand(newObjectCommand(values, objectServices{
		transfer: deps.ObjectTransfer, editor: deps.ObjectEditor, deleter: deps.ObjectDeleter, linkCreator: deps.ObjectLinkCreator,
		bucketCreator: deps.ObjectBucketCreator, bucketUpdater: deps.ObjectBucketUpdater, bucketDeleter: deps.ObjectBucketDeleter,
	}, clock))
	root.AddCommand(newStreamCommand(values, deps.Streams, clock))
	root.AddCommand(newConsumerCommand(values, deps.Consumers))
	root.AddCommand(newPublishCommand(values, deps.CorePublisher, files))
	root.AddCommand(newMessageCommand(values, deps.Messages, deps.Streams, deps.CorePublisher))
	root.AddCommand(newSubjectCommand(values, deps.Subjects, deps.CorePublisher, deps.CoreRequester, files))
	root.AddCommand(newProfileCommand(deps.Profiles))
	root.AddCommand(newRequestCommand(values, requestServices{requester: deps.CoreRequester, history: deps.RequestHistory, templates: deps.RequestTemplates}, files))
	root.AddCommand(newTopologyCommand(values, deps.TopologyReader, deps.TopologyOperator))
	root.AddCommand(newConnectionsCommand(values, deps.ConnectionReader, deps.ConnectionCloser))
	root.AddCommand(newAccountsCommand(values, deps.OperatorReader, deps.OperatorAccountCreator, deps.OperatorUserCreator, deps.OperatorMutator))
	root.AddCommand(newAlertsCommand(values, deps.Alerts))
	root.AddCommand(newCommandsCommand(values, deps.Commands))
	root.AddCommand(newBookmarksCommand(values, deps.Bookmarks))
	root.AddCommand(newDashboardCommand(values, deps.Dashboard))
	return root
}

func newVersionCommand(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"nats-tui %s (commit: %s, built: %s)\n",
				valueOr(info.Version, "dev"),
				valueOr(info.Commit, "unknown"),
				valueOr(info.Date, "unknown"),
			)
			return err
		},
	}
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return root.GenPowerShellCompletion(cmd.OutOrStdout())
			}
		},
	}
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
