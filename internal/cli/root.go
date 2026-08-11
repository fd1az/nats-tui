package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/clcsolutions/nats-tui/internal/app/subjects"
	"github.com/clcsolutions/nats-tui/internal/config"
	domainrequestreply "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
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
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return config.Configure(values, configFile)
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
	flags.String("authority-dir", "", "private directory for locally managed account and user credentials")
	flags.String("authority-operator-seed", "", "operator seed file enabling account mutations")
	flags.Duration("refresh", 2*time.Second, "dashboard refresh interval")

	for _, name := range []string{"profile", "server", "monitor-url", "credentials", "token", "authority-dir", "authority-operator-seed", "refresh"} {
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
	root.AddCommand(newRequestCommand(values, requestServices{requester: deps.CoreRequester, history: deps.RequestHistory, templates: deps.RequestTemplates}, files))
	root.AddCommand(newTopologyCommand(values, deps.TopologyReader, deps.TopologyOperator))
	root.AddCommand(newConnectionsCommand(values, deps.ConnectionReader, deps.ConnectionCloser))
	root.AddCommand(newAccountsCommand(values, deps.OperatorReader, deps.OperatorAccountCreator, deps.OperatorUserCreator, deps.OperatorMutator))
	root.AddCommand(newAlertsCommand(values, deps.Alerts))
	root.AddCommand(newCommandsCommand(values, deps.Commands))
	root.AddCommand(newBookmarksCommand(values, deps.Bookmarks))
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
