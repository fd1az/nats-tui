package natsmonitor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	appconnections "github.com/clcsolutions/nats-tui/internal/app/connections"
	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	appoperator "github.com/clcsolutions/nats-tui/internal/app/operator"
	apprequestreply "github.com/clcsolutions/nats-tui/internal/app/requestreply"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/clcsolutions/nats-tui/internal/config"
	domainrequestreply "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
	domaintopology "github.com/clcsolutions/nats-tui/internal/domain/topology"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Monitor struct {
	connection         *nats.Conn
	jetstream          jetstream.JetStream
	clock              Clock
	topologyFallback   appTopologySource
	connectionFallback appConnectionSource
	operatorFallback   appOperatorSource
}

type appTopologySource interface {
	Topology(context.Context) (domaintopology.Snapshot, error)
}
type appConnectionSource interface {
	ListConnections(context.Context) (appconnections.Snapshot, error)
}
type appOperatorSource interface {
	OperatorCatalog(context.Context) (appoperator.Catalog, error)
}

type Clock interface{ Now() time.Time }
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

const effectivelyIndefinitePause = 100 * 365 * 24 * time.Hour

func (m *Monitor) now() time.Time {
	if m.clock == nil {
		panic("NATS monitor clock is not configured")
	}
	return m.clock.Now()
}

func (m *Monitor) Publish(ctx context.Context, message domainrequestreply.Message) error {
	outbound := &nats.Msg{Subject: message.Subject().String(), Data: message.Payload(), Header: nats.Header(message.Headers())}
	if err := m.connection.PublishMsg(outbound); err != nil {
		return fmt.Errorf("publish Core NATS message: %w", err)
	}
	if err := m.connection.FlushWithContext(ctx); err != nil {
		return fmt.Errorf("flush Core NATS publish: %w", err)
	}
	return nil
}

func (m *Monitor) Request(ctx context.Context, request domainrequestreply.Request) ([]domainrequestreply.Reply, error) {
	inbox := nats.NewInbox()
	subscription, err := m.connection.SubscribeSync(inbox)
	if err != nil {
		return nil, fmt.Errorf("subscribe to request inbox: %w", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()
	if err := subscription.AutoUnsubscribe(request.MaxReplies()); err != nil {
		return nil, fmt.Errorf("limit request replies: %w", err)
	}
	message := request.Message()
	outbound := &nats.Msg{Subject: message.Subject().String(), Reply: inbox, Data: message.Payload(), Header: nats.Header(message.Headers())}
	startedAt := m.now()
	if err := m.connection.PublishMsg(outbound); err != nil {
		return nil, fmt.Errorf("publish Core NATS request: %w", err)
	}
	if err := m.connection.FlushWithContext(ctx); err != nil {
		return nil, fmt.Errorf("flush Core NATS request: %w", err)
	}
	replies := make([]domainrequestreply.Reply, 0, request.MaxReplies())
	for len(replies) < request.MaxReplies() {
		response, err := subscription.NextMsgWithContext(ctx)
		if err != nil {
			if len(replies) > 0 && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, nats.ErrTimeout)) {
				return replies, nil
			}
			if errors.Is(err, nats.ErrNoResponders) {
				return nil, apprequestreply.ErrNoResponders
			}
			return nil, fmt.Errorf("receive Core NATS reply: %w", err)
		}
		receivedAt := m.now()
		reply, err := domainrequestreply.NewReply(response.Data, map[string][]string(response.Header), receivedAt, receivedAt.Sub(startedAt))
		if err != nil {
			return nil, fmt.Errorf("map Core NATS reply: %w", err)
		}
		replies = append(replies, reply)
	}
	return replies, nil
}

func Connect(settings config.Settings) (*Monitor, error) {
	return ConnectWithClock(settings, systemClock{})
}

func ConnectWithClock(settings config.Settings, clock Clock) (*Monitor, error) {
	if clock == nil {
		panic("nil NATS monitor clock")
	}
	options := []nats.Option{
		nats.Name("nats-tui"),
		nats.Timeout(5 * time.Second),
		nats.MaxReconnects(-1),
	}
	if settings.Credentials != "" {
		options = append(options, nats.UserCredentials(settings.Credentials))
	}
	if settings.Token != "" {
		options = append(options, nats.Token(settings.Token))
	}

	connection, err := nats.Connect(settings.Server, options...)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := jetstream.New(connection)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("open JetStream context: %w", err)
	}

	monitor := &Monitor{connection: connection, jetstream: js, clock: clock}
	if settings.MonitorURL != "" {
		fallback, fallbackErr := newHTTPTopology(settings.MonitorURL, nil)
		if fallbackErr != nil {
			connection.Close()
			return nil, fallbackErr
		}
		monitor.topologyFallback = fallback
		monitor.connectionFallback = fallback
		monitor.operatorFallback = fallback
	}
	return monitor, nil
}

func (m *Monitor) Snapshot(ctx context.Context) (dashboard.Snapshot, error) {
	account, err := m.jetstream.AccountInfo(ctx)
	if err != nil {
		return dashboard.Snapshot{}, fmt.Errorf("load JetStream account info: %w", err)
	}

	var messages uint64
	streams := m.jetstream.ListStreams(ctx)
	for info := range streams.Info() {
		messages += info.State.Msgs
	}
	if err := streams.Err(); err != nil {
		return dashboard.Snapshot{}, fmt.Errorf("load JetStream stream state: %w", err)
	}

	return dashboard.Snapshot{
		ServerName: m.connection.ConnectedServerName(),
		Cluster:    m.connection.ConnectedClusterName(),
		Version:    m.connection.ConnectedServerVersion(),
		Streams:    account.Streams,
		Consumers:  account.Consumers,
		Messages:   messages,
		Bytes:      account.Memory + account.Store,
		ObservedAt: m.now(),
	}, nil
}

func (m *Monitor) ListStreams(ctx context.Context) ([]streams.Summary, error) {
	lister := m.jetstream.ListStreams(ctx)
	result := make([]streams.Summary, 0)
	for info := range lister.Info() {
		result = append(result, streams.Summary{
			Name:        info.Config.Name,
			Description: info.Config.Description,
			Subjects:    append([]string(nil), info.Config.Subjects...),
			Messages:    info.State.Msgs,
			Bytes:       info.State.Bytes,
			Consumers:   info.State.Consumers,
			Storage:     streamStorage(info.Config.Storage),
			Retention:   streamRetention(info.Config.Retention),
			Replicas:    info.Config.Replicas,
			MaxMessages: info.Config.MaxMsgs,
			MaxBytes:    info.Config.MaxBytes,
			MaxAge:      info.Config.MaxAge, MaxMessageSize: info.Config.MaxMsgSize,
			MaxMessagesPerSubject: info.Config.MaxMsgsPerSubject, DuplicateWindow: info.Config.Duplicates,
			MaxConsumers: info.Config.MaxConsumers, Discard: streamDiscard(info.Config.Discard),
			DiscardNewPerSubject: info.Config.DiscardNewPerSubject, Compression: info.Config.Compression != jetstream.NoCompression,
			AllowDirect: info.Config.AllowDirect, MirrorDirect: info.Config.MirrorDirect, AllowRollup: info.Config.AllowRollup,
			DenyDelete: info.Config.DenyDelete, DenyPurge: info.Config.DenyPurge, Sealed: info.Config.Sealed,
			NoAck: info.Config.NoAck, AllowMessageTTL: info.Config.AllowMsgTTL,
			PlacementCluster: streamPlacementCluster(info.Config.Placement), PlacementTags: streamPlacementTags(info.Config.Placement),
			Metadata: cloneStringMap(info.Config.Metadata), SubjectTransform: streamSubjectTransform(info.Config.SubjectTransform),
			Republish: streamRepublish(info.Config.RePublish), Mirror: streamSourceFromNATS(info.Config.Mirror), Sources: streamSourcesFromNATS(info.Config.Sources),
			Cluster: streamClusterName(info.Cluster), Leader: streamLeader(info.Cluster), ReplicaHealth: streamReplicaHealth(info.Cluster),
			MirrorHealth: streamSourceHealth(info.Mirror), SourceHealth: streamSourcesHealth(info.Sources),
			FirstAt: info.State.FirstTime,
			LastAt:  info.State.LastTime,
		})
	}
	if err := lister.Err(); err != nil {
		return nil, fmt.Errorf("list JetStream streams: %w", err)
	}
	return result, nil
}

func (m *Monitor) ListKeyValueBuckets(ctx context.Context) ([]kvstore.BucketSummary, error) {
	lister := m.jetstream.KeyValueStores(ctx)
	result := make([]kvstore.BucketSummary, 0)
	seen := make(map[string]struct{})
	for status := range lister.Status() {
		config := status.Config()
		storage := kvstore.StorageFile
		if config.Storage == jetstream.MemoryStorage {
			storage = kvstore.StorageMemory
		}
		result = append(result, kvstore.BucketSummary{
			Name: status.Bucket(), Description: config.Description, Values: status.Values(), Bytes: status.Bytes(),
			History: status.History(), TTL: status.TTL(), Storage: storage, Replicas: config.Replicas, Compressed: status.IsCompressed(),
			MaxBytes: config.MaxBytes, MaxValueSize: config.MaxValueSize,
			Mirror: keyValueMirror(config.Mirror), Sources: keyValueSources(config.Sources),
		})
		seen[status.Bucket()] = struct{}{}
	}
	if err := lister.Error(); err != nil {
		return nil, fmt.Errorf("list JetStream key-value buckets: %w", err)
	}
	// The NATS KV status lister filters by KV subjects, so pure mirrors (which
	// have no subjects of their own) are absent. Include those backing streams
	// explicitly to keep the operator catalog complete.
	streams := m.jetstream.ListStreams(ctx)
	for info := range streams.Info() {
		if !strings.HasPrefix(info.Config.Name, "KV_") {
			continue
		}
		name := strings.TrimPrefix(info.Config.Name, "KV_")
		if _, exists := seen[name]; exists {
			continue
		}
		result = append(result, keyValueSummaryFromStream(name, info))
	}
	if err := streams.Err(); err != nil {
		return nil, fmt.Errorf("list mirrored key-value backing streams: %w", err)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Name < result[right].Name })
	return result, nil
}

func keyValueSummaryFromStream(name string, info *jetstream.StreamInfo) kvstore.BucketSummary {
	storage := kvstore.StorageFile
	if info.Config.Storage == jetstream.MemoryStorage {
		storage = kvstore.StorageMemory
	}
	return kvstore.BucketSummary{
		Name: name, Description: info.Config.Description, Values: info.State.Msgs, Bytes: info.State.Bytes,
		History: info.Config.MaxMsgsPerSubject, TTL: info.Config.MaxAge, Storage: storage, Replicas: info.Config.Replicas,
		Compressed: info.Config.Compression != jetstream.NoCompression, MaxBytes: info.Config.MaxBytes, MaxValueSize: info.Config.MaxMsgSize,
		Mirror: keyValueMirror(info.Config.Mirror), Sources: keyValueSources(info.Config.Sources),
	}
}

func (m *Monitor) ListObjectStoreBuckets(ctx context.Context) ([]objectstore.BucketSummary, error) {
	lister := m.jetstream.ObjectStores(ctx)
	result := make([]objectstore.BucketSummary, 0)
	seen := make(map[string]struct{})
	for status := range lister.Status() {
		storage := "FILE"
		if status.Storage() == jetstream.MemoryStorage {
			storage = "MEMORY"
		}
		summary := objectstore.BucketSummary{
			Name: status.Bucket(), Description: status.Description(), Size: status.Size(), TTL: status.TTL(),
			Storage: storage, Replicas: status.Replicas(), Compressed: status.IsCompressed(), Sealed: status.Sealed(), Metadata: cloneMetadata(status.Metadata()),
		}
		if detailed, ok := status.(interface{ StreamInfo() *jetstream.StreamInfo }); ok && detailed.StreamInfo() != nil {
			config := detailed.StreamInfo().Config
			summary.MaxBytes = config.MaxBytes
			summary.Mirror = objectStoreBucketName(config.Mirror)
			summary.Sources = objectStoreSourceNames(config.Sources)
		}
		result = append(result, summary)
		seen[summary.Name] = struct{}{}
	}
	if err := lister.Error(); err != nil {
		return nil, fmt.Errorf("list JetStream object-store buckets: %w", err)
	}
	// Pure mirrors have no local subjects and are omitted by the Object Store
	// status lister. Read their OBJ_ backing streams so the catalog remains
	// complete and exposes the replication relationship.
	streams := m.jetstream.ListStreams(ctx)
	for info := range streams.Info() {
		if !strings.HasPrefix(info.Config.Name, "OBJ_") {
			continue
		}
		name := strings.TrimPrefix(info.Config.Name, "OBJ_")
		if _, exists := seen[name]; exists {
			continue
		}
		result = append(result, objectStoreSummaryFromStream(name, info))
	}
	if err := streams.Err(); err != nil {
		return nil, fmt.Errorf("list mirrored object-store backing streams: %w", err)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Name < result[right].Name })
	return result, nil
}

func objectStoreSummaryFromStream(name string, info *jetstream.StreamInfo) objectstore.BucketSummary {
	storage := "FILE"
	if info.Config.Storage == jetstream.MemoryStorage {
		storage = "MEMORY"
	}
	return objectstore.BucketSummary{
		Name: name, Description: info.Config.Description, Size: info.State.Bytes, TTL: info.Config.MaxAge,
		Storage: storage, Replicas: info.Config.Replicas, Compressed: info.Config.Compression != jetstream.NoCompression,
		Sealed: info.Config.Sealed, MaxBytes: info.Config.MaxBytes, Metadata: cloneMetadata(info.Config.Metadata),
		Mirror: objectStoreBucketName(info.Config.Mirror), Sources: objectStoreSourceNames(info.Config.Sources),
	}
}

func (m *Monitor) CreateObjectStoreBucket(ctx context.Context, definition objectstore.Definition) error {
	var err error
	if definition.Mirror == "" && len(definition.Sources) == 0 {
		_, err = m.jetstream.CreateObjectStore(ctx, objectStoreConfig(definition))
	} else {
		_, err = m.jetstream.CreateStream(ctx, objectStoreStreamConfig(definition))
	}
	if err != nil {
		return fmt.Errorf("create object-store bucket %q: %w", definition.Name, err)
	}
	return nil
}

func (m *Monitor) UpdateObjectStoreBucket(ctx context.Context, name string, changes objectstore.Changes) error {
	stream, err := m.jetstream.Stream(ctx, "OBJ_"+name)
	if err != nil {
		return fmt.Errorf("load object-store bucket %q config: %w", name, err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return fmt.Errorf("load object-store bucket %q status: %w", name, err)
	}
	config := info.Config
	applyObjectStoreStreamChanges(&config, changes)
	if _, err := m.jetstream.UpdateStream(ctx, config); err != nil {
		return fmt.Errorf("update object-store bucket %q: %w", name, err)
	}
	return nil
}

func (m *Monitor) DeleteObjectStoreBucket(ctx context.Context, name string) error {
	if err := m.jetstream.DeleteObjectStore(ctx, name); err != nil {
		return fmt.Errorf("delete object-store bucket %q: %w", name, err)
	}
	return nil
}

func objectStoreConfig(definition objectstore.Definition) jetstream.ObjectStoreConfig {
	storage := jetstream.FileStorage
	if definition.Storage == objectstore.StorageMemory {
		storage = jetstream.MemoryStorage
	}
	return jetstream.ObjectStoreConfig{
		Bucket: definition.Name, Description: definition.Description, TTL: definition.TTL,
		MaxBytes: definition.MaxBytes, Storage: storage, Replicas: definition.Replicas,
		Compression: definition.Compression, Metadata: cloneMetadata(definition.Metadata),
	}
}

func applyObjectStoreStreamChanges(config *jetstream.StreamConfig, changes objectstore.Changes) {
	if changes.Description != nil {
		config.Description = *changes.Description
	}
	if changes.TTL != nil {
		config.MaxAge = *changes.TTL
	}
	if changes.MaxBytes != nil {
		config.MaxBytes = *changes.MaxBytes
	}
	if changes.Replicas != nil {
		config.Replicas = *changes.Replicas
	}
	if changes.Compression != nil {
		config.Compression = jetstream.NoCompression
		if *changes.Compression {
			config.Compression = jetstream.S2Compression
		}
	}
	if changes.Metadata != nil {
		config.Metadata = cloneMetadata(*changes.Metadata)
	}
}

func objectStoreStreamConfig(definition objectstore.Definition) jetstream.StreamConfig {
	storage := jetstream.FileStorage
	if definition.Storage == objectstore.StorageMemory {
		storage = jetstream.MemoryStorage
	}
	compression := jetstream.NoCompression
	if definition.Compression {
		compression = jetstream.S2Compression
	}
	config := jetstream.StreamConfig{
		Name: "OBJ_" + definition.Name, Description: definition.Description, MaxAge: definition.TTL,
		MaxBytes: definition.MaxBytes, Storage: storage, Replicas: definition.Replicas,
		Discard: jetstream.DiscardNew, AllowRollup: true, AllowDirect: true,
		Compression: compression, Metadata: cloneMetadata(definition.Metadata),
	}
	if definition.Mirror != "" {
		config.Mirror = objectStoreStreamSource(definition.Name, definition.Mirror)
		config.MirrorDirect = true
		return config
	}
	config.Subjects = objectStoreSubjects(definition.Name)
	for _, source := range definition.Sources {
		config.Sources = append(config.Sources, objectStoreStreamSource(definition.Name, source))
	}
	return config
}

func objectStoreSubjects(bucket string) []string {
	return []string{"$O." + bucket + ".C.>", "$O." + bucket + ".M.>"}
}

func objectStoreStreamSource(destination, source string) *jetstream.StreamSource {
	return &jetstream.StreamSource{
		Name: "OBJ_" + source,
		SubjectTransforms: []jetstream.SubjectTransformConfig{{
			Source: "$O." + source + ".>", Destination: "$O." + destination + ".>",
		}},
	}
}

func objectStoreBucketName(source *jetstream.StreamSource) string {
	if source == nil {
		return ""
	}
	return strings.TrimPrefix(source.Name, "OBJ_")
}

func objectStoreSourceNames(sources []*jetstream.StreamSource) []string {
	result := make([]string, 0, len(sources))
	for _, source := range sources {
		if name := objectStoreBucketName(source); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func (m *Monitor) ListObjects(ctx context.Context, bucketName string) ([]objectstore.ObjectSummary, error) {
	bucket, err := m.jetstream.ObjectStore(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("open object-store bucket %q: %w", bucketName, err)
	}
	objects, err := bucket.List(ctx)
	if errors.Is(err, jetstream.ErrNoObjectsFound) {
		return []objectstore.ObjectSummary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list objects in bucket %q: %w", bucketName, err)
	}
	result := make([]objectstore.ObjectSummary, 0, len(objects))
	for _, info := range objects {
		if info == nil {
			continue
		}
		result = append(result, objectSummary(bucketName, info))
	}
	return result, nil
}

func (m *Monitor) PutObject(ctx context.Context, command objectstore.UploadCommand) (objectstore.ObjectSummary, error) {
	bucket, err := m.jetstream.ObjectStore(ctx, command.Bucket)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("open object-store bucket %q: %w", command.Bucket, err)
	}
	info, err := bucket.Put(ctx, jetstream.ObjectMeta{
		Name: command.Name, Description: command.Description, Headers: cloneNATSHeader(command.Headers), Metadata: cloneMetadata(command.Metadata),
	}, command.Reader)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("upload object %q to bucket %q: %w", command.Name, command.Bucket, err)
	}
	return objectSummary(command.Bucket, info), nil
}

func (m *Monitor) OpenObject(ctx context.Context, bucketName, name string) (objectstore.DownloadSource, error) {
	bucket, err := m.jetstream.ObjectStore(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("open object-store bucket %q: %w", bucketName, err)
	}
	result, err := bucket.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("download object %q from bucket %q: %w", name, bucketName, err)
	}
	info, err := result.Info()
	if err != nil {
		_ = result.Close()
		return nil, fmt.Errorf("load object %q metadata: %w", name, err)
	}
	return &objectDownload{ObjectResult: result, info: objectSummary(bucketName, info)}, nil
}

func (m *Monitor) GetObject(ctx context.Context, bucketName, name string) (objectstore.ObjectSummary, error) {
	bucket, err := m.jetstream.ObjectStore(ctx, bucketName)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("open object-store bucket %q: %w", bucketName, err)
	}
	info, err := bucket.GetInfo(ctx, name)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("load object %q from bucket %q: %w", name, bucketName, err)
	}
	return objectSummary(bucketName, info), nil
}

func (m *Monitor) UpdateObject(ctx context.Context, command objectstore.UpdateObject) (objectstore.ObjectSummary, error) {
	bucket, err := m.jetstream.ObjectStore(ctx, command.Bucket)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("open object-store bucket %q: %w", command.Bucket, err)
	}
	meta := jetstream.ObjectMeta{
		Name: command.Name, Description: command.Description,
		Headers: cloneNATSHeader(command.Headers), Metadata: cloneMetadata(command.Metadata),
	}
	if err := bucket.UpdateMeta(ctx, command.CurrentName, meta); err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("update object %q in bucket %q: %w", command.CurrentName, command.Bucket, err)
	}
	info, err := bucket.GetInfo(ctx, command.Name)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("reload updated object %q: %w", command.Name, err)
	}
	return objectSummary(command.Bucket, info), nil
}

func (m *Monitor) DeleteObject(ctx context.Context, bucketName, name string) error {
	bucket, err := m.jetstream.ObjectStore(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("open object-store bucket %q: %w", bucketName, err)
	}
	if err := bucket.Delete(ctx, name); err != nil {
		return fmt.Errorf("delete object %q from bucket %q: %w", name, bucketName, err)
	}
	return nil
}

func (m *Monitor) CreateObjectLink(ctx context.Context, definition objectstore.LinkDefinition) (objectstore.ObjectSummary, error) {
	destination, err := m.jetstream.ObjectStore(ctx, definition.Bucket)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("open destination object-store bucket %q: %w", definition.Bucket, err)
	}
	target, err := m.jetstream.ObjectStore(ctx, definition.TargetBucket)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("open target object-store bucket %q: %w", definition.TargetBucket, err)
	}
	var info *jetstream.ObjectInfo
	if definition.IsBucketLink() {
		info, err = destination.AddBucketLink(ctx, definition.Name, target)
	} else {
		var targetInfo *jetstream.ObjectInfo
		targetInfo, err = target.GetInfo(ctx, definition.TargetObject)
		if err == nil {
			info, err = destination.AddLink(ctx, definition.Name, targetInfo)
		}
	}
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("create object-store link %q in bucket %q: %w", definition.Name, definition.Bucket, err)
	}
	return objectSummary(definition.Bucket, info), nil
}

func (m *Monitor) WatchObjects(ctx context.Context, bucketName string) (<-chan objectstore.ObjectEvent, error) {
	bucket, err := m.jetstream.ObjectStore(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("open object-store bucket %q: %w", bucketName, err)
	}
	watcher, err := bucket.Watch(ctx, jetstream.UpdatesOnly())
	if err != nil {
		return nil, fmt.Errorf("watch object-store bucket %q: %w", bucketName, err)
	}
	events := make(chan objectstore.ObjectEvent)
	go forwardObjectEvents(ctx, bucketName, watcher, events)
	return events, nil
}

func forwardObjectEvents(ctx context.Context, bucket string, watcher jetstream.ObjectWatcher, events chan<- objectstore.ObjectEvent) {
	defer close(events)
	defer watcher.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case info, open := <-watcher.Updates():
			if !open {
				return
			}
			if info == nil {
				continue
			}
			event := objectstore.ObjectEvent{Kind: objectstore.ObjectUpserted, Object: objectSummary(bucket, info)}
			if info.Deleted {
				event.Kind = objectstore.ObjectDeleted
			}
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}
}

type objectDownload struct {
	jetstream.ObjectResult
	info objectstore.ObjectSummary
}

func (d *objectDownload) Info() objectstore.ObjectSummary { return d.info }
func (d *objectDownload) Verify() error                   { return d.Error() }

func objectSummary(bucket string, info *jetstream.ObjectInfo) objectstore.ObjectSummary {
	summary := objectstore.ObjectSummary{
		Bucket: bucket, ID: info.NUID, Name: info.Name, Description: info.Description, Size: info.Size,
		Chunks: info.Chunks, ModifiedAt: info.ModTime, Digest: info.Digest, Deleted: info.Deleted,
		Headers: cloneHeader(info.Headers), Metadata: cloneMetadata(info.Metadata),
	}
	if info.Opts != nil && info.Opts.Link != nil {
		summary.Link = &objectstore.LinkTarget{Bucket: info.Opts.Link.Bucket, Name: info.Opts.Link.Name}
	}
	return summary
}

func cloneNATSHeader(header map[string][]string) nats.Header {
	if len(header) == 0 {
		return nil
	}
	result := make(nats.Header, len(header))
	for name, values := range header {
		result[name] = append([]string(nil), values...)
	}
	return result
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	result := make(map[string]string, len(metadata))
	for name, value := range metadata {
		result[name] = value
	}
	return result
}

func (m *Monitor) CreateKeyValueBucket(ctx context.Context, definition kvstore.Definition) error {
	storage := jetstream.FileStorage
	if definition.Storage == kvstore.StorageMemory {
		storage = jetstream.MemoryStorage
	}
	config := jetstream.KeyValueConfig{
		Bucket: definition.Name, Description: definition.Description, Storage: storage,
		History: uint8(definition.History), TTL: definition.TTL, MaxBytes: definition.MaxBytes,
		MaxValueSize: definition.MaxValueSize, Replicas: definition.Replicas, Compression: definition.Compression,
	}
	if definition.Mirror != "" {
		config.Mirror = &jetstream.StreamSource{Name: definition.Mirror}
	}
	for _, source := range definition.Sources {
		config.Sources = append(config.Sources, &jetstream.StreamSource{Name: source})
	}
	_, err := m.jetstream.CreateKeyValue(ctx, config)
	if err != nil {
		return fmt.Errorf("create key-value bucket %q: %w", definition.Name, err)
	}
	return nil
}

func keyValueMirror(source *jetstream.StreamSource) string {
	if source == nil {
		return ""
	}
	return strings.TrimPrefix(source.Name, "KV_")
}

func keyValueSources(sources []*jetstream.StreamSource) []string {
	result := make([]string, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			result = append(result, strings.TrimPrefix(source.Name, "KV_"))
		}
	}
	return result
}

func (m *Monitor) UpdateKeyValueBucket(ctx context.Context, name string, changes kvstore.Changes) error {
	bucket, err := m.jetstream.KeyValue(ctx, name)
	if err != nil {
		return fmt.Errorf("open key-value bucket %q: %w", name, err)
	}
	status, err := bucket.Status(ctx)
	if err != nil {
		return fmt.Errorf("load key-value bucket %q config: %w", name, err)
	}
	config := status.Config()
	applyKeyValueChanges(&config, changes)
	if _, err := m.jetstream.UpdateKeyValue(ctx, config); err != nil {
		return fmt.Errorf("update key-value bucket %q: %w", name, err)
	}
	return nil
}

func (m *Monitor) DeleteKeyValueBucket(ctx context.Context, name string) error {
	if err := m.jetstream.DeleteKeyValue(ctx, name); err != nil {
		return fmt.Errorf("delete key-value bucket %q: %w", name, err)
	}
	return nil
}

func (m *Monitor) ListKeyValueKeys(ctx context.Context, bucketName string) ([]string, error) {
	bucket, err := m.jetstream.KeyValue(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("open key-value bucket %q: %w", bucketName, err)
	}
	lister, err := bucket.ListKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list keys in key-value bucket %q: %w", bucketName, err)
	}
	seen := make(map[string]struct{})
	keys := make([]string, 0)
	for key := range lister.Keys() {
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list keys in key-value bucket %q: %w", bucketName, err)
	}
	sort.Strings(keys)
	return keys, nil
}

func (m *Monitor) GetKeyValueEntry(ctx context.Context, bucketName, key string) (kvstore.Entry, error) {
	bucket, err := m.jetstream.KeyValue(ctx, bucketName)
	if err != nil {
		return kvstore.Entry{}, fmt.Errorf("open key-value bucket %q: %w", bucketName, err)
	}
	entry, err := bucket.Get(ctx, key)
	if err != nil {
		return kvstore.Entry{}, fmt.Errorf("get key %q from key-value bucket %q: %w", key, bucketName, err)
	}
	return keyValueEntry(entry), nil
}

func (m *Monitor) ListKeyValueHistory(ctx context.Context, bucketName, key string) ([]kvstore.Entry, error) {
	bucket, err := m.jetstream.KeyValue(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("open key-value bucket %q: %w", bucketName, err)
	}
	revisions, err := bucket.History(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load history for key %q from key-value bucket %q: %w", key, bucketName, err)
	}
	entries := make([]kvstore.Entry, 0, len(revisions))
	for _, revision := range revisions {
		entries = append(entries, keyValueEntry(revision))
	}
	return entries, nil
}

func (m *Monitor) WatchKeyValue(ctx context.Context, bucketName, key string) (<-chan kvstore.Entry, error) {
	bucket, err := m.jetstream.KeyValue(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("open key-value bucket %q: %w", bucketName, err)
	}
	watcher, err := bucket.Watch(ctx, key, jetstream.UpdatesOnly())
	if err != nil {
		return nil, fmt.Errorf("watch key %q in key-value bucket %q: %w", key, bucketName, err)
	}
	events := make(chan kvstore.Entry)
	go func() {
		defer close(events)
		defer watcher.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case revision, ok := <-watcher.Updates():
				if !ok {
					return
				}
				if revision == nil {
					continue
				}
				entry := keyValueEntry(revision)
				select {
				case events <- entry:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return events, nil
}

func keyValueEntry(entry jetstream.KeyValueEntry) kvstore.Entry {
	operation := kvstore.OperationPut
	switch entry.Operation() {
	case jetstream.KeyValueDelete:
		operation = kvstore.OperationDelete
	case jetstream.KeyValuePurge:
		operation = kvstore.OperationPurge
	}
	return kvstore.Entry{
		Bucket: entry.Bucket(), Key: entry.Key(), Value: append([]byte(nil), entry.Value()...),
		Operation: operation, Revision: entry.Revision(), Created: entry.Created(), Delta: entry.Delta(),
	}
}

func (m *Monitor) PutKeyValueEntry(ctx context.Context, command kvstore.WriteEntry) (uint64, error) {
	bucket, err := m.jetstream.KeyValue(ctx, command.Bucket)
	if err != nil {
		return 0, fmt.Errorf("open key-value bucket %q: %w", command.Bucket, err)
	}
	var revision uint64
	if command.ExpectedRevision == nil {
		revision, err = bucket.Put(ctx, command.Key, command.Value)
	} else {
		revision, err = bucket.Update(ctx, command.Key, command.Value, *command.ExpectedRevision)
	}
	if err != nil {
		return 0, fmt.Errorf("put key %q in key-value bucket %q: %w", command.Key, command.Bucket, err)
	}
	return revision, nil
}

func (m *Monitor) RemoveKeyValueEntry(ctx context.Context, command kvstore.RemoveEntry) error {
	bucket, err := m.jetstream.KeyValue(ctx, command.Bucket)
	if err != nil {
		return fmt.Errorf("open key-value bucket %q: %w", command.Bucket, err)
	}
	options := make([]jetstream.KVDeleteOpt, 0, 1)
	if command.ExpectedRevision != nil {
		options = append(options, jetstream.LastRevision(*command.ExpectedRevision))
	}
	if command.Action == kvstore.RemovePurge {
		err = bucket.Purge(ctx, command.Key, options...)
	} else {
		err = bucket.Delete(ctx, command.Key, options...)
	}
	if err != nil {
		return fmt.Errorf("%s key %q in key-value bucket %q: %w", strings.ToLower(string(command.Action)), command.Key, command.Bucket, err)
	}
	return nil
}

func applyKeyValueChanges(config *jetstream.KeyValueConfig, changes kvstore.Changes) {
	if changes.Description != nil {
		config.Description = *changes.Description
	}
	if changes.History != nil {
		config.History = uint8(*changes.History)
	}
	if changes.TTL != nil {
		config.TTL = *changes.TTL
	}
	if changes.MaxBytes != nil {
		config.MaxBytes = *changes.MaxBytes
	}
	if changes.MaxValueSize != nil {
		config.MaxValueSize = *changes.MaxValueSize
	}
	if changes.Replicas != nil {
		config.Replicas = *changes.Replicas
	}
	if changes.Compression != nil {
		config.Compression = *changes.Compression
	}
}

func (m *Monitor) OperateStream(ctx context.Context, command streams.Command) error {
	switch command.Action {
	case streams.ActionCreate:
		_, err := m.jetstream.CreateStream(ctx, streamConfig(command.Definition))
		if err != nil {
			return fmt.Errorf("create stream %q: %w", command.Name, err)
		}
		return nil
	case streams.ActionUpdate:
		stream, err := m.jetstream.Stream(ctx, command.Name)
		if err != nil {
			return fmt.Errorf("open stream %q: %w", command.Name, err)
		}
		info, err := stream.Info(ctx)
		if err != nil {
			return fmt.Errorf("load stream %q config: %w", command.Name, err)
		}
		config := info.Config
		originalStorage := config.Storage
		applyStreamDefinition(&config, command.Definition)
		config.Storage = originalStorage
		if _, err := m.jetstream.UpdateStream(ctx, config); err != nil {
			return fmt.Errorf("update stream %q: %w", command.Name, err)
		}
		return nil
	case streams.ActionDelete:
		if err := m.jetstream.DeleteStream(ctx, command.Name); err != nil {
			return fmt.Errorf("delete stream %q: %w", command.Name, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported stream action %q", command.Action)
	}
}

func (m *Monitor) PurgeStream(ctx context.Context, command streams.PurgeCommand) error {
	stream, err := m.jetstream.Stream(ctx, command.Stream)
	if err != nil {
		return fmt.Errorf("open stream %q: %w", command.Stream, err)
	}
	options := make([]jetstream.StreamPurgeOpt, 0, 2)
	if command.Subject != "" {
		options = append(options, jetstream.WithPurgeSubject(command.Subject))
	}
	if command.Sequence > 0 {
		options = append(options, jetstream.WithPurgeSequence(command.Sequence))
	}
	if command.Keep > 0 {
		options = append(options, jetstream.WithPurgeKeep(command.Keep))
	}
	if err := stream.Purge(ctx, options...); err != nil {
		return fmt.Errorf("purge stream %q: %w", command.Stream, err)
	}
	return nil
}

func (m *Monitor) DeleteStreamMessage(ctx context.Context, command streams.MessageDeleteCommand) error {
	stream, err := m.jetstream.Stream(ctx, command.Stream)
	if err != nil {
		return fmt.Errorf("open stream %q: %w", command.Stream, err)
	}
	if command.Secure {
		err = stream.SecureDeleteMsg(ctx, command.Sequence)
	} else {
		err = stream.DeleteMsg(ctx, command.Sequence)
	}
	if err != nil {
		return fmt.Errorf("delete message %d from stream %q: %w", command.Sequence, command.Stream, err)
	}
	return nil
}

func streamConfig(definition streams.Definition) jetstream.StreamConfig {
	config := jetstream.StreamConfig{
		Name:              definition.Name,
		MaxConsumers:      -1,
		MaxMsgsPerSubject: -1,
		MaxMsgSize:        -1,
		Discard:           jetstream.DiscardOld,
	}
	applyStreamDefinition(&config, definition)
	return config
}

func applyStreamDefinition(config *jetstream.StreamConfig, definition streams.Definition) {
	config.Description = definition.Description
	config.Subjects = append([]string(nil), definition.Subjects...)
	config.Storage = jetstream.FileStorage
	if definition.Storage == streams.StorageMemory {
		config.Storage = jetstream.MemoryStorage
	}
	switch definition.Retention {
	case streams.RetentionInterest:
		config.Retention = jetstream.InterestPolicy
	case streams.RetentionWorkQueue:
		config.Retention = jetstream.WorkQueuePolicy
	default:
		config.Retention = jetstream.LimitsPolicy
	}
	config.MaxMsgs = definition.MaxMessages
	config.MaxBytes = definition.MaxBytes
	config.Replicas = definition.Replicas
	config.MaxAge = definition.MaxAge
	if definition.MaxMessageSize != 0 {
		config.MaxMsgSize = definition.MaxMessageSize
	}
	if definition.MaxMessagesPerSubject != 0 {
		config.MaxMsgsPerSubject = definition.MaxMessagesPerSubject
	}
	if definition.MaxConsumers != 0 {
		config.MaxConsumers = definition.MaxConsumers
	}
	config.Duplicates = definition.DuplicateWindow
	if definition.Discard == streams.DiscardNew {
		config.Discard = jetstream.DiscardNew
	} else {
		config.Discard = jetstream.DiscardOld
	}
	config.DiscardNewPerSubject = definition.DiscardNewPerSubject
	config.Compression = jetstream.NoCompression
	if definition.Compression {
		config.Compression = jetstream.S2Compression
	}
	config.AllowDirect, config.MirrorDirect = definition.AllowDirect, definition.MirrorDirect
	config.AllowRollup, config.DenyDelete, config.DenyPurge = definition.AllowRollup, definition.DenyDelete, definition.DenyPurge
	config.Sealed, config.NoAck, config.AllowMsgTTL = definition.Sealed, definition.NoAck, definition.AllowMessageTTL
	if definition.PlacementCluster != "" || len(definition.PlacementTags) > 0 {
		config.Placement = &jetstream.Placement{Cluster: definition.PlacementCluster, Tags: append([]string(nil), definition.PlacementTags...)}
	} else {
		config.Placement = nil
	}
	config.Metadata = cloneStringMap(definition.Metadata)
	config.SubjectTransform = streamSubjectTransformToNATS(definition.SubjectTransform)
	config.RePublish = streamRepublishToNATS(definition.Republish)
	config.Mirror = streamSourceToNATS(definition.Mirror)
	config.Sources = make([]*jetstream.StreamSource, 0, len(definition.Sources))
	for i := range definition.Sources {
		config.Sources = append(config.Sources, streamSourceToNATS(&definition.Sources[i]))
	}
}

func streamDiscard(value jetstream.DiscardPolicy) streams.Discard {
	if value == jetstream.DiscardNew {
		return streams.DiscardNew
	}
	return streams.DiscardOld
}

func streamPlacementCluster(value *jetstream.Placement) string {
	if value == nil {
		return ""
	}
	return value.Cluster
}

func streamPlacementTags(value *jetstream.Placement) []string {
	if value == nil {
		return nil
	}
	return append([]string(nil), value.Tags...)
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func streamSubjectTransform(value *jetstream.SubjectTransformConfig) *streams.SubjectTransform {
	if value == nil {
		return nil
	}
	return &streams.SubjectTransform{Source: value.Source, Destination: value.Destination}
}

func streamSubjectTransformToNATS(value *streams.SubjectTransform) *jetstream.SubjectTransformConfig {
	if value == nil {
		return nil
	}
	return &jetstream.SubjectTransformConfig{Source: value.Source, Destination: value.Destination}
}

func streamRepublish(value *jetstream.RePublish) *streams.Republish {
	if value == nil {
		return nil
	}
	return &streams.Republish{Source: value.Source, Destination: value.Destination, HeadersOnly: value.HeadersOnly}
}

func streamRepublishToNATS(value *streams.Republish) *jetstream.RePublish {
	if value == nil {
		return nil
	}
	return &jetstream.RePublish{Source: value.Source, Destination: value.Destination, HeadersOnly: value.HeadersOnly}
}

func streamSourceFromNATS(value *jetstream.StreamSource) *streams.Source {
	if value == nil {
		return nil
	}
	result := &streams.Source{Name: value.Name, StartSequence: value.OptStartSeq, StartTime: value.OptStartTime, FilterSubject: value.FilterSubject}
	for _, transform := range value.SubjectTransforms {
		result.SubjectTransforms = append(result.SubjectTransforms, streams.SubjectTransform{Source: transform.Source, Destination: transform.Destination})
	}
	if value.External != nil {
		result.External = &streams.External{APIPrefix: value.External.APIPrefix, DeliverPrefix: value.External.DeliverPrefix}
	}
	return result
}

func streamSourcesFromNATS(values []*jetstream.StreamSource) []streams.Source {
	result := make([]streams.Source, 0, len(values))
	for _, value := range values {
		if mapped := streamSourceFromNATS(value); mapped != nil {
			result = append(result, *mapped)
		}
	}
	return result
}

func streamSourceToNATS(value *streams.Source) *jetstream.StreamSource {
	if value == nil {
		return nil
	}
	result := &jetstream.StreamSource{Name: value.Name, OptStartSeq: value.StartSequence, OptStartTime: value.StartTime, FilterSubject: value.FilterSubject}
	for _, transform := range value.SubjectTransforms {
		result.SubjectTransforms = append(result.SubjectTransforms, jetstream.SubjectTransformConfig{Source: transform.Source, Destination: transform.Destination})
	}
	if value.External != nil {
		result.External = &jetstream.ExternalStream{APIPrefix: value.External.APIPrefix, DeliverPrefix: value.External.DeliverPrefix}
	}
	return result
}

func streamClusterName(value *jetstream.ClusterInfo) string {
	if value == nil {
		return ""
	}
	return value.Name
}
func streamLeader(value *jetstream.ClusterInfo) string {
	if value == nil {
		return ""
	}
	return value.Leader
}
func streamReplicaHealth(value *jetstream.ClusterInfo) []streams.ReplicaHealth {
	if value == nil {
		return nil
	}
	result := make([]streams.ReplicaHealth, 0, len(value.Replicas))
	for _, peer := range value.Replicas {
		if peer != nil {
			result = append(result, streams.ReplicaHealth{Name: peer.Name, Current: peer.Current, Offline: peer.Offline, Active: peer.Active, Lag: peer.Lag})
		}
	}
	return result
}
func streamSourceHealth(value *jetstream.StreamSourceInfo) *streams.SourceHealth {
	if value == nil {
		return nil
	}
	return &streams.SourceHealth{Name: value.Name, Lag: value.Lag, Active: value.Active}
}
func streamSourcesHealth(values []*jetstream.StreamSourceInfo) []streams.SourceHealth {
	result := make([]streams.SourceHealth, 0, len(values))
	for _, value := range values {
		if mapped := streamSourceHealth(value); mapped != nil {
			result = append(result, *mapped)
		}
	}
	return result
}

func streamStorage(storage jetstream.StorageType) streams.Storage {
	if storage == jetstream.MemoryStorage {
		return streams.StorageMemory
	}
	return streams.StorageFile
}

func streamRetention(retention jetstream.RetentionPolicy) streams.Retention {
	switch retention {
	case jetstream.InterestPolicy:
		return streams.RetentionInterest
	case jetstream.WorkQueuePolicy:
		return streams.RetentionWorkQueue
	default:
		return streams.RetentionLimits
	}
}

func (m *Monitor) ListConsumers(ctx context.Context) ([]consumers.Summary, error) {
	result := make([]consumers.Summary, 0)
	streamLister := m.jetstream.ListStreams(ctx)
	for streamInfo := range streamLister.Info() {
		stream, err := m.jetstream.Stream(ctx, streamInfo.Config.Name)
		if err != nil {
			return nil, fmt.Errorf("open stream %q: %w", streamInfo.Config.Name, err)
		}
		consumerLister := stream.ListConsumers(ctx)
		for info := range consumerLister.Info() {
			result = append(result, consumerSummary(info))
		}
		if err := consumerLister.Err(); err != nil {
			return nil, fmt.Errorf("list consumers for %q: %w", streamInfo.Config.Name, err)
		}
	}
	if err := streamLister.Err(); err != nil {
		return nil, fmt.Errorf("list streams for consumers: %w", err)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Stream == result[right].Stream {
			return result[left].Name < result[right].Name
		}
		return result[left].Stream < result[right].Stream
	})
	return result, nil
}

func (m *Monitor) OperateConsumer(ctx context.Context, command consumers.Command) error {
	stream, err := m.jetstream.Stream(ctx, command.Consumer.Stream())
	if err != nil {
		return fmt.Errorf("open stream %q: %w", command.Consumer.Stream(), err)
	}
	name := command.Consumer.Name()
	switch command.Action {
	case consumers.ActionPause:
		_, err = stream.PauseConsumer(ctx, name, m.now().Add(effectivelyIndefinitePause))
	case consumers.ActionResume:
		_, err = stream.ResumeConsumer(ctx, name)
	case consumers.ActionReset:
		_, err = stream.ResetConsumer(ctx, name)
	case consumers.ActionDelete:
		err = stream.DeleteConsumer(ctx, name)
	default:
		return fmt.Errorf("unsupported consumer action %q", command.Action)
	}
	if err != nil {
		return fmt.Errorf("%s consumer %q on stream %q: %w", command.Action, name, command.Consumer.Stream(), err)
	}
	return nil
}

func (m *Monitor) OperateConsumerConfig(ctx context.Context, command consumers.ConfigCommand) error {
	definition := command.Definition
	stream, err := m.jetstream.Stream(ctx, definition.Stream)
	if err != nil {
		return fmt.Errorf("open stream %q: %w", definition.Stream, err)
	}
	config := consumerConfigToNATS(definition)
	switch command.Action {
	case consumers.ConfigCreate:
		_, err = stream.CreateConsumer(ctx, config)
	case consumers.ConfigUpdate:
		_, err = stream.UpdateConsumer(ctx, config)
	default:
		return fmt.Errorf("unsupported consumer configuration action %d", command.Action)
	}
	if err != nil {
		return fmt.Errorf("configure consumer %q on stream %q: %w", definition.Name, definition.Stream, err)
	}
	return nil
}

func consumerSummary(info *jetstream.ConsumerInfo) consumers.Summary {
	filters := append([]string(nil), info.Config.FilterSubjects...)
	if info.Config.FilterSubject != "" {
		filters = append(filters, info.Config.FilterSubject)
	}
	lastActive := time.Time{}
	if info.Delivered.Last != nil {
		lastActive = *info.Delivered.Last
	}
	workerActive := info.PushBound || info.NumWaiting > 0
	return consumers.Summary{
		Stream:          info.Stream,
		Name:            info.Name,
		Status:          consumers.Classify(info.Paused, workerActive, info.NumPending, info.NumAckPending),
		Pending:         info.NumPending,
		AckPending:      info.NumAckPending,
		Redelivered:     info.NumRedelivered,
		Waiting:         info.NumWaiting,
		DeliveredStream: info.Delivered.Stream,
		AckFloorStream:  info.AckFloor.Stream,
		FilterSubjects:  filters,
		Durable:         info.Config.Durable != "",
		Push:            info.Config.DeliverSubject != "",
		CreatedAt:       info.Created,
		LastActiveAt:    lastActive,
		Config:          consumerConfigFromNATS(info.Stream, info.Name, info.Config),
	}
}

func consumerConfigToNATS(definition consumers.Definition) jetstream.ConsumerConfig {
	config := jetstream.ConsumerConfig{
		Name: definition.Name, Description: definition.Description,
		DeliverPolicy: consumerDeliverPolicyToNATS(definition.DeliverPolicy), OptStartSeq: definition.StartSequence, OptStartTime: definition.StartTime,
		AckPolicy: consumerAckPolicyToNATS(definition.AckPolicy), AckWait: definition.AckWait, MaxDeliver: definition.MaxDeliver,
		BackOff: append([]time.Duration(nil), definition.Backoff...), FilterSubjects: append([]string(nil), definition.FilterSubjects...),
		ReplayPolicy: consumerReplayPolicyToNATS(definition.ReplayPolicy), RateLimit: definition.RateLimit, SampleFrequency: definition.SampleFrequency,
		MaxWaiting: definition.MaxWaiting, MaxAckPending: definition.MaxAckPending, HeadersOnly: definition.HeadersOnly,
		MaxRequestBatch: definition.MaxRequestBatch, MaxRequestExpires: definition.MaxRequestExpires, MaxRequestMaxBytes: definition.MaxRequestMaxBytes,
		InactiveThreshold: definition.InactiveThreshold, Replicas: definition.Replicas, MemoryStorage: definition.MemoryStorage,
		Metadata: cloneStringMap(definition.Metadata), DeliverSubject: definition.DeliverSubject, DeliverGroup: definition.DeliverGroup,
		FlowControl: definition.FlowControl, IdleHeartbeat: definition.IdleHeartbeat,
	}
	if len(definition.FilterSubjects) == 1 {
		config.FilterSubject = definition.FilterSubjects[0]
		config.FilterSubjects = nil
	}
	if definition.Durable {
		config.Durable = definition.Name
	}
	return config
}

func consumerConfigFromNATS(stream, name string, config jetstream.ConsumerConfig) consumers.Definition {
	filters := append([]string(nil), config.FilterSubjects...)
	if config.FilterSubject != "" {
		filters = append(filters, config.FilterSubject)
	}
	return consumers.Definition{
		Stream: stream, Name: name, Description: config.Description, Durable: config.Durable != "",
		DeliverSubject: config.DeliverSubject, DeliverGroup: config.DeliverGroup,
		DeliverPolicy: consumerDeliverPolicyFromNATS(config.DeliverPolicy), StartSequence: config.OptStartSeq, StartTime: config.OptStartTime,
		AckPolicy: consumerAckPolicyFromNATS(config.AckPolicy), AckWait: config.AckWait, MaxDeliver: config.MaxDeliver,
		Backoff: append([]time.Duration(nil), config.BackOff...), FilterSubjects: filters,
		ReplayPolicy: consumerReplayPolicyFromNATS(config.ReplayPolicy), RateLimit: config.RateLimit, SampleFrequency: config.SampleFrequency,
		MaxWaiting: config.MaxWaiting, MaxAckPending: config.MaxAckPending, HeadersOnly: config.HeadersOnly,
		MaxRequestBatch: config.MaxRequestBatch, MaxRequestExpires: config.MaxRequestExpires, MaxRequestMaxBytes: config.MaxRequestMaxBytes,
		InactiveThreshold: config.InactiveThreshold, Replicas: config.Replicas, MemoryStorage: config.MemoryStorage,
		Metadata: cloneStringMap(config.Metadata), FlowControl: config.FlowControl, IdleHeartbeat: config.IdleHeartbeat,
	}
}

func consumerDeliverPolicyToNATS(policy consumers.DeliverPolicy) jetstream.DeliverPolicy {
	switch policy {
	case consumers.DeliverLast:
		return jetstream.DeliverLastPolicy
	case consumers.DeliverNew:
		return jetstream.DeliverNewPolicy
	case consumers.DeliverByStartSequence:
		return jetstream.DeliverByStartSequencePolicy
	case consumers.DeliverByStartTime:
		return jetstream.DeliverByStartTimePolicy
	case consumers.DeliverLastPerSubject:
		return jetstream.DeliverLastPerSubjectPolicy
	default:
		return jetstream.DeliverAllPolicy
	}
}

func consumerDeliverPolicyFromNATS(policy jetstream.DeliverPolicy) consumers.DeliverPolicy {
	switch policy {
	case jetstream.DeliverLastPolicy:
		return consumers.DeliverLast
	case jetstream.DeliverNewPolicy:
		return consumers.DeliverNew
	case jetstream.DeliverByStartSequencePolicy:
		return consumers.DeliverByStartSequence
	case jetstream.DeliverByStartTimePolicy:
		return consumers.DeliverByStartTime
	case jetstream.DeliverLastPerSubjectPolicy:
		return consumers.DeliverLastPerSubject
	default:
		return consumers.DeliverAll
	}
}

func consumerAckPolicyToNATS(policy consumers.AckPolicy) jetstream.AckPolicy {
	switch policy {
	case consumers.AckNone:
		return jetstream.AckNonePolicy
	case consumers.AckAll:
		return jetstream.AckAllPolicy
	default:
		return jetstream.AckExplicitPolicy
	}
}

func consumerAckPolicyFromNATS(policy jetstream.AckPolicy) consumers.AckPolicy {
	switch policy {
	case jetstream.AckNonePolicy:
		return consumers.AckNone
	case jetstream.AckAllPolicy:
		return consumers.AckAll
	default:
		return consumers.AckExplicit
	}
}

func consumerReplayPolicyToNATS(policy consumers.ReplayPolicy) jetstream.ReplayPolicy {
	if policy == consumers.ReplayOriginal {
		return jetstream.ReplayOriginalPolicy
	}
	return jetstream.ReplayInstantPolicy
}

func consumerReplayPolicyFromNATS(policy jetstream.ReplayPolicy) consumers.ReplayPolicy {
	if policy == jetstream.ReplayOriginalPolicy {
		return consumers.ReplayOriginal
	}
	return consumers.ReplayInstant
}

func (m *Monitor) RecentMessages(ctx context.Context, streamName string, limit int) ([]messages.Record, error) {
	return m.BrowseMessages(ctx, messages.HistoryQuery{Stream: streamName, Limit: limit})
}

func (m *Monitor) BrowseMessages(ctx context.Context, query messages.HistoryQuery) ([]messages.Record, error) {
	streamName, limit := query.Stream, query.Limit
	stream, err := m.jetstream.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("open stream %q: %w", streamName, err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("load stream %q state: %w", streamName, err)
	}
	if limit <= 0 || info.State.Msgs == 0 {
		return []messages.Record{}, nil
	}

	result := make([]messages.Record, 0, min(limit, int(info.State.Msgs)))
	lastSequence := info.State.LastSeq
	if query.EndSequence > 0 {
		lastSequence = min(lastSequence, query.EndSequence)
	}
	for sequence := lastSequence; sequence >= info.State.FirstSeq && len(result) < limit; sequence-- {
		if query.StartSequence > 0 && sequence < query.StartSequence {
			break
		}
		message, getErr := stream.GetMsg(ctx, sequence)
		if errors.Is(getErr, jetstream.ErrMsgNotFound) {
			if sequence == 0 {
				break
			}
			continue
		}
		if getErr != nil {
			return nil, fmt.Errorf("load message %d from %q: %w", sequence, streamName, getErr)
		}
		if query.StartTime != nil && message.Time.Before(*query.StartTime) {
			break
		}
		if query.Subject != "" && !matchesSubject(query.Subject, message.Subject) {
			if sequence == 0 {
				break
			}
			continue
		}
		result = append(result, messages.Record{
			Sequence: message.Sequence,
			Subject:  message.Subject,
			Time:     message.Time,
			Headers:  map[string][]string(message.Header),
			Payload:  append([]byte(nil), message.Data...),
			Stream:   streamName,
		})
		if sequence == 0 {
			break
		}
	}
	return result, nil
}

func matchesSubject(pattern, subject string) bool {
	patternTokens, subjectTokens := strings.Split(pattern, "."), strings.Split(subject, ".")
	for index, token := range patternTokens {
		if token == ">" {
			return index < len(subjectTokens)
		}
		if index >= len(subjectTokens) || (token != "*" && token != subjectTokens[index]) {
			return false
		}
	}
	return len(patternTokens) == len(subjectTokens)
}

func (m *Monitor) Watch(ctx context.Context, subjects []string) (<-chan messages.Record, error) {
	if len(subjects) == 0 {
		return nil, fmt.Errorf("watch messages: stream has no subjects")
	}

	incoming := make(chan *nats.Msg, 512)
	subscriptions := make([]*nats.Subscription, 0, len(subjects))
	for _, subject := range subjects {
		subscription, err := m.connection.ChanSubscribe(subject, incoming)
		if err != nil {
			unsubscribeAll(subscriptions)
			return nil, fmt.Errorf("watch subject %q: %w", subject, err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	if err := m.connection.FlushWithContext(ctx); err != nil {
		unsubscribeAll(subscriptions)
		return nil, fmt.Errorf("activate message watch: %w", err)
	}

	events := make(chan messages.Record, 512)
	go forwardMessages(ctx, incoming, events, subscriptions, m.now)
	return events, nil
}

func (m *Monitor) WatchOrdered(ctx context.Context, streamName string, subjects []string) (<-chan messages.Record, error) {
	stream, err := m.jetstream.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("open stream %q for ordered watch: %w", streamName, err)
	}
	consumer, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects: append([]string(nil), subjects...), DeliverPolicy: jetstream.DeliverNewPolicy,
		ReplayPolicy: jetstream.ReplayInstantPolicy, InactiveThreshold: 5 * time.Minute,
		Metadata: map[string]string{"nats-tui.mode": "ordered-watch"}, NamePrefix: "nats_tui_watch",
	})
	if err != nil {
		return nil, fmt.Errorf("create ordered consumer for %q: %w", streamName, err)
	}
	events := make(chan messages.Record, 512)
	consume, err := consumer.Consume(func(message jetstream.Msg) {
		metadata, metadataErr := message.Metadata()
		if metadataErr != nil {
			return
		}
		record := messages.Record{
			Sequence: metadata.Sequence.Stream, ConsumerSequence: metadata.Sequence.Consumer,
			Subject: message.Subject(), Time: metadata.Timestamp, Headers: cloneHeader(message.Headers()), Payload: append([]byte(nil), message.Data()...),
			Stream: metadata.Stream, Consumer: metadata.Consumer, Domain: metadata.Domain, Deliveries: metadata.NumDelivered, Pending: metadata.NumPending,
		}
		select {
		case events <- record:
		case <-ctx.Done():
		}
	})
	if err != nil {
		return nil, fmt.Errorf("start ordered watch for %q: %w", streamName, err)
	}
	go func() {
		select {
		case <-ctx.Done():
			consume.Stop()
		case <-consume.Closed():
		}
		<-consume.Closed()
		close(events)
	}()
	return events, nil
}

func forwardMessages(ctx context.Context, incoming <-chan *nats.Msg, events chan<- messages.Record, subscriptions []*nats.Subscription, now func() time.Time) {
	defer close(events)
	defer unsubscribeAll(subscriptions)
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-incoming:
			if message == nil {
				continue
			}
			record := messages.Record{
				Subject:       message.Subject,
				Time:          now(),
				Headers:       cloneHeader(message.Header),
				Payload:       append([]byte(nil), message.Data...),
				WatchPending:  len(incoming),
				WatchCapacity: cap(incoming),
			}
			select {
			case events <- record:
			case <-ctx.Done():
				return
			}
		}
	}
}

func cloneHeader(header nats.Header) map[string][]string {
	result := make(map[string][]string, len(header))
	for name, values := range header {
		result[name] = append([]string(nil), values...)
	}
	return result
}

func unsubscribeAll(subscriptions []*nats.Subscription) {
	for _, subscription := range subscriptions {
		_ = subscription.Unsubscribe()
	}
}

func (m *Monitor) Close() {
	m.connection.Close()
}
