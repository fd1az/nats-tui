package natsmonitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/clcsolutions/nats-tui/internal/adapters/localfiles"
	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	apprequestreply "github.com/clcsolutions/nats-tui/internal/app/requestreply"
	appstreams "github.com/clcsolutions/nats-tui/internal/app/streams"
	apptopology "github.com/clcsolutions/nats-tui/internal/app/topology"
	"github.com/clcsolutions/nats-tui/internal/config"
	domainrequestreply "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestTopologyUsesSystemAPIOrReportsMissingCapability(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(monitor.Close)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot, err := monitor.Topology(ctx)
	if errors.Is(err, apptopology.ErrSystemAccountRequired) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Servers()) == 0 {
		t.Fatal("system API returned an empty topology")
	}
}

func TestWatchReceivesPublishedMessage(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}

	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	subject := fmt.Sprintf("nats-tui.integration.%d", time.Now().UnixNano())
	events, err := monitor.Watch(ctx, []string{subject})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if err := monitor.connection.Publish(subject, []byte(`{"live":true}`)); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if err := monitor.connection.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	select {
	case event := <-events:
		if event.Subject != subject || string(event.Payload) != `{"live":true}` {
			t.Fatalf("event = %#v", event)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for live event")
	}
}

func TestListKeyValueBucketsReturnsOperationalSummary(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bucketName := fmt.Sprintf("NATS_TUI_KV_%d", time.Now().UnixNano())
	definition := kvstore.Definition{
		Name: bucketName, Description: "integration catalog", History: 5, TTL: time.Hour,
		Storage: kvstore.StorageMemory, Replicas: 1, MaxBytes: -1, MaxValueSize: -1, Compression: true,
	}
	if err := kvstore.NewCreateBucketHandler(monitor).Handle(ctx, definition); err != nil {
		t.Fatalf("CreateKeyValueBucket() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = monitor.jetstream.DeleteKeyValue(cleanupCtx, bucketName)
	})
	bucket, err := monitor.jetstream.KeyValue(ctx, bucketName)
	if err != nil {
		t.Fatalf("KeyValue() error = %v", err)
	}
	status, err := bucket.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	advanced := status.Config()
	advanced.Metadata = map[string]string{"owner": "integration"}
	if _, err := monitor.jetstream.UpdateKeyValue(ctx, advanced); err != nil {
		t.Fatalf("add advanced metadata error = %v", err)
	}
	description := "updated safely"
	history := 6
	if err := kvstore.NewUpdateBucketHandler(monitor).Handle(ctx, bucketName, kvstore.Changes{Description: &description, History: &history}); err != nil {
		t.Fatalf("UpdateKeyValueBucket() error = %v", err)
	}
	status, err = bucket.Status(ctx)
	if err != nil || status.Config().Description != description || status.Config().History != 6 || status.Metadata()["owner"] != "integration" {
		t.Fatalf("updated config = %#v, error = %v", status.Config(), err)
	}
	if _, err := bucket.Put(ctx, "session.42", []byte(`{"active":true}`)); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := bucket.Put(ctx, "config.alpha", []byte(`{"enabled":true}`)); err != nil {
		t.Fatalf("Put() second key error = %v", err)
	}
	keys, err := kvstore.NewListKeysHandler(monitor).Handle(ctx, bucketName)
	if err != nil || len(keys) != 2 || keys[0] != "config.alpha" || keys[1] != "session.42" {
		t.Fatalf("keys = %#v, error = %v", keys, err)
	}
	entry, err := kvstore.NewGetEntryHandler(monitor).Handle(ctx, bucketName, "session.42")
	if err != nil || string(entry.Value) != `{"active":true}` || entry.Operation != kvstore.OperationPut || entry.Revision == 0 || entry.Created.IsZero() {
		t.Fatalf("entry = %#v, error = %v", entry, err)
	}
	badRevision := entry.Revision - 1
	if _, err := kvstore.NewPutEntryHandler(monitor).Handle(ctx, kvstore.WriteEntry{Bucket: bucketName, Key: "session.42", Value: []byte(`{"active":false}`), ExpectedRevision: &badRevision}); err == nil {
		t.Fatal("CAS write accepted a stale revision")
	}
	watchEvents, err := kvstore.NewWatchHandler(monitor).Handle(ctx, bucketName, "session.42")
	if err != nil {
		t.Fatalf("WatchKeyValue() error = %v", err)
	}
	updatedRevision, err := kvstore.NewPutEntryHandler(monitor).Handle(ctx, kvstore.WriteEntry{Bucket: bucketName, Key: "session.42", Value: []byte(`{"active":false}`), ExpectedRevision: &entry.Revision})
	if err != nil || updatedRevision <= entry.Revision {
		t.Fatalf("CAS revision = %d, error = %v", updatedRevision, err)
	}
	select {
	case event := <-watchEvents:
		if event.Revision != updatedRevision || event.Key != "session.42" || string(event.Value) != `{"active":false}` {
			t.Fatalf("KV watch event = %#v", event)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for KV watch event")
	}

	catalog, err := monitor.ListKeyValueBuckets(ctx)
	if err != nil {
		t.Fatalf("ListKeyValueBuckets() error = %v", err)
	}
	found := false
	for _, summary := range catalog {
		if summary.Name == bucketName {
			if summary.Values != 3 || summary.History != 6 || summary.TTL != time.Hour || summary.Storage != "MEMORY" || summary.Replicas != 1 || !summary.Compressed {
				t.Fatalf("bucket summary = %#v", summary)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created KV bucket was not returned by catalog")
	}
	timeline, err := kvstore.NewListHistoryHandler(monitor).Handle(ctx, bucketName, "session.42")
	if err != nil || len(timeline) != 2 || timeline[0].Operation != kvstore.OperationPut || timeline[1].Revision != updatedRevision || string(timeline[1].Value) != `{"active":false}` {
		t.Fatalf("timeline = %#v, error = %v", timeline, err)
	}
	staleDeleteRevision := updatedRevision - 1
	if err := kvstore.NewRemoveEntryHandler(monitor).Handle(ctx, kvstore.RemoveEntry{Bucket: bucketName, Key: "session.42", Action: kvstore.RemoveDelete, ExpectedRevision: &staleDeleteRevision}); err == nil {
		t.Fatal("delete accepted a stale revision")
	}
	if err := kvstore.NewRemoveEntryHandler(monitor).Handle(ctx, kvstore.RemoveEntry{Bucket: bucketName, Key: "session.42", Action: kvstore.RemoveDelete, ExpectedRevision: &updatedRevision}); err != nil {
		t.Fatalf("Delete key error = %v", err)
	}
	if _, err := bucket.Get(ctx, "session.42"); err == nil {
		t.Fatal("deleted key is still readable")
	}
	restored, err := kvstore.NewRestoreHandler(monitor, monitor).Handle(ctx, kvstore.RestoreEntry{Bucket: bucketName, Key: "session.42", Revision: entry.Revision})
	if err != nil || restored.Revision <= updatedRevision || string(restored.Value) != `{"active":true}` {
		t.Fatalf("restored entry = %#v, error = %v", restored, err)
	}
	restoredEntry, err := bucket.Get(ctx, "session.42")
	if err != nil {
		t.Fatalf("get restored latest value: %v", err)
	}
	if string(restoredEntry.Value()) != `{"active":true}` {
		t.Fatalf("restored latest value = %q", restoredEntry.Value())
	}
	configEntry, err := bucket.Get(ctx, "config.alpha")
	if err != nil {
		t.Fatalf("Get purge candidate error = %v", err)
	}
	configRevision := configEntry.Revision()
	if err := kvstore.NewRemoveEntryHandler(monitor).Handle(ctx, kvstore.RemoveEntry{Bucket: bucketName, Key: "config.alpha", Action: kvstore.RemovePurge, ExpectedRevision: &configRevision}); err != nil {
		t.Fatalf("Purge key error = %v", err)
	}
	revisions, err := bucket.History(ctx, "config.alpha")
	if err != nil || len(revisions) != 1 || revisions[0].Operation() != jetstream.KeyValuePurge {
		t.Fatalf("purged history length=%d error=%v", len(revisions), err)
	}
	if err := kvstore.NewDeleteBucketHandler(monitor).Handle(ctx, bucketName); err != nil {
		t.Fatalf("DeleteKeyValueBucket() error = %v", err)
	}
	if _, err := monitor.jetstream.KeyValue(ctx, bucketName); err == nil {
		t.Fatal("KV bucket still exists after delete")
	}
}

func TestListObjectStoreBucketsReturnsOperationalSummary(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	name := fmt.Sprintf("NATS_TUI_OBJECT_%d", time.Now().UnixNano())
	store, err := monitor.jetstream.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket: name, Description: "integration objects", Storage: jetstream.MemoryStorage, Replicas: 1, Compression: true,
	})
	if err != nil {
		t.Fatalf("CreateObjectStore() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = monitor.jetstream.DeleteObjectStore(cleanupCtx, name)
	})
	objectInfo, err := store.Put(ctx, jetstream.ObjectMeta{
		Name: "artifact.bin", Description: "integration artifact",
		Headers: nats.Header{"Content-Type": []string{"application/octet-stream"}}, Metadata: map[string]string{"environment": "test"},
	}, strings.NewReader("artifact"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	catalog, err := monitor.ListObjectStoreBuckets(ctx)
	if err != nil {
		t.Fatalf("ListObjectStoreBuckets() error = %v", err)
	}
	objects, err := objectstore.NewListObjectsHandler(monitor).Handle(ctx, name)
	if err != nil || len(objects) != 1 {
		t.Fatalf("objects = %#v, error = %v", objects, err)
	}
	object := objects[0]
	contentTypes := object.Headers["Content-Type"]
	if object.Name != "artifact.bin" || object.Description != "integration artifact" || object.Size != 8 || object.Chunks == 0 || object.Digest != objectInfo.Digest || object.Metadata["environment"] != "test" || len(contentTypes) != 1 || contentTypes[0] != "application/octet-stream" {
		t.Fatalf("object summary = %#v", object)
	}
	transferDirectory := t.TempDir()
	uploadPath := transferDirectory + "/upload.bin"
	downloadPath := transferDirectory + "/download.bin"
	if err := os.WriteFile(uploadPath, []byte("streamed and verified"), 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	watchEvents, err := objectstore.NewWatchObjectsHandler(monitor).Handle(watchCtx, name)
	if err != nil {
		t.Fatalf("start object watch: %v", err)
	}
	waitForObject := func(kind objectstore.ObjectEventKind, objectName string) objectstore.ObjectEvent {
		t.Helper()
		select {
		case event := <-watchEvents:
			if event.Kind != kind || event.Object.Name != objectName {
				t.Fatalf("object event = %#v, want kind=%v name=%q", event, kind, objectName)
			}
			return event
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for object event kind=%v name=%q", kind, objectName)
			return objectstore.ObjectEvent{}
		}
	}
	files := localfiles.NewOSObjectTransfer()
	uploaded, err := files.UploadObject(ctx, monitor, objectstore.UploadFile{Bucket: name, Path: uploadPath, Name: "streamed.bin"}, nil)
	if err != nil || uploaded.Digest == "" || uploaded.Size != 21 {
		t.Fatalf("uploaded object = %#v, error = %v", uploaded, err)
	}
	uploadEvent := waitForObject(objectstore.ObjectUpserted, "streamed.bin")
	if uploadEvent.Object.ID == "" {
		t.Fatal("upload watch event has no stable object ID")
	}
	downloaded, err := files.DownloadObject(ctx, monitor, objectstore.DownloadFile{Bucket: name, Name: "streamed.bin", Path: downloadPath}, nil)
	if err != nil || !downloaded.Verified {
		t.Fatalf("downloaded object = %#v, error = %v", downloaded, err)
	}
	downloadedBytes, err := os.ReadFile(downloadPath)
	if err != nil || string(downloadedBytes) != "streamed and verified" {
		t.Fatalf("downloaded bytes = %q, error = %v", downloadedBytes, err)
	}
	newName, newDescription := "stable.bin", "promoted by integration test"
	metadata := map[string]string{"channel": "stable"}
	edited, err := objectstore.NewEditObjectHandler(monitor, monitor).Handle(ctx, objectstore.EditObject{
		Bucket: name, Name: "streamed.bin", NewName: &newName, Description: &newDescription, Metadata: &metadata,
	})
	if err != nil || edited.Name != newName || edited.Description != newDescription || edited.Metadata["channel"] != "stable" {
		t.Fatalf("edited object = %#v, error = %v", edited, err)
	}
	editEvent := waitForObject(objectstore.ObjectUpserted, newName)
	if editEvent.Object.ID != uploadEvent.Object.ID {
		t.Fatalf("rename changed stable identity from %q to %q", uploadEvent.Object.ID, editEvent.Object.ID)
	}
	if err := objectstore.NewDeleteObjectHandler(monitor).Handle(ctx, name, newName); err != nil {
		t.Fatalf("delete edited object: %v", err)
	}
	waitForObject(objectstore.ObjectDeleted, newName)
	remaining, err := objectstore.NewListObjectsHandler(monitor).Handle(ctx, name)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	for _, candidate := range remaining {
		if candidate.Name == newName {
			t.Fatalf("deleted object remains in catalog: %#v", candidate)
		}
	}
	targetName := name + "_TARGET"
	targetStore, err := monitor.jetstream.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: targetName, Storage: jetstream.MemoryStorage})
	if err != nil {
		t.Fatalf("create link target bucket: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = monitor.jetstream.DeleteObjectStore(cleanupCtx, targetName)
	})
	if _, err := targetStore.PutString(ctx, "v1.tar", "linked release"); err != nil {
		t.Fatalf("put link target: %v", err)
	}
	objectLink, err := objectstore.NewCreateLinkHandler(monitor).Handle(ctx, objectstore.LinkDefinition{
		Bucket: name, Name: "latest", TargetBucket: targetName, TargetObject: "v1.tar",
	})
	if err != nil || objectLink.Link == nil || objectLink.Link.Bucket != targetName || objectLink.Link.Name != "v1.tar" {
		t.Fatalf("object link = %#v, error = %v", objectLink, err)
	}
	bucketLink, err := objectstore.NewCreateLinkHandler(monitor).Handle(ctx, objectstore.LinkDefinition{
		Bucket: name, Name: "releases", TargetBucket: targetName,
	})
	if err != nil || bucketLink.Link == nil || bucketLink.Link.Bucket != targetName || bucketLink.Link.Name != "" {
		t.Fatalf("bucket link = %#v, error = %v", bucketLink, err)
	}
	linkedValue, err := store.GetString(ctx, "latest")
	if err != nil || linkedValue != "linked release" {
		t.Fatalf("linked value = %q, error = %v", linkedValue, err)
	}
	for _, bucket := range catalog {
		if bucket.Name == name {
			if bucket.Description != "integration objects" || bucket.Storage != "MEMORY" || bucket.Replicas != 1 || !bucket.Compressed || bucket.Size == 0 {
				t.Fatalf("object bucket = %#v", bucket)
			}
			return
		}
	}
	t.Fatal("created Object Store bucket was not returned")
}

func TestObjectStoreBucketLifecycle(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	name := fmt.Sprintf("NATS_TUI_OBJECT_CRUD_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = monitor.DeleteObjectStoreBucket(cleanupCtx, name)
	})

	definition := objectstore.Definition{
		Name: name, Description: "created by integration test", Storage: objectstore.StorageMemory,
		MaxBytes: -1, Replicas: 1, Metadata: map[string]string{"owner": "platform"},
	}
	if err := objectstore.NewCreateBucketHandler(monitor).Handle(ctx, definition); err != nil {
		t.Fatalf("CreateObjectStoreBucket() error = %v", err)
	}
	description, ttl, maxBytes, compression := "updated by integration test", time.Hour, int64(1<<20), true
	metadata := map[string]string{"owner": "runtime", "environment": "test"}
	changes := objectstore.Changes{
		Description: &description, TTL: &ttl, MaxBytes: &maxBytes, Compression: &compression, Metadata: &metadata,
	}
	if err := objectstore.NewUpdateBucketHandler(monitor).Handle(ctx, name, changes); err != nil {
		t.Fatalf("UpdateObjectStoreBucket() error = %v", err)
	}
	catalog, err := objectstore.NewListBucketsHandler(monitor).Handle(ctx)
	if err != nil {
		t.Fatalf("ListObjectStoreBuckets() error = %v", err)
	}
	found := false
	for _, bucket := range catalog {
		if bucket.Name != name {
			continue
		}
		found = true
		if bucket.Description != description || bucket.TTL != ttl || bucket.MaxBytes != maxBytes || !bucket.Compressed || bucket.Metadata["owner"] != "runtime" {
			t.Fatalf("updated object bucket = %#v", bucket)
		}
	}
	if !found {
		t.Fatal("created Object Store bucket was not returned")
	}
	if err := objectstore.NewDeleteBucketHandler(monitor).Handle(ctx, name); err != nil {
		t.Fatalf("DeleteObjectStoreBucket() error = %v", err)
	}
	if _, err := monitor.jetstream.ObjectStore(ctx, name); err == nil {
		t.Fatal("Object Store bucket still exists after delete")
	}
}

func TestCreateObjectStoreMirrorAndSources(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	suffix := time.Now().UnixNano()
	sourceA := fmt.Sprintf("NATS_TUI_OBJ_SRC_A_%d", suffix)
	sourceB := fmt.Sprintf("NATS_TUI_OBJ_SRC_B_%d", suffix)
	mirror := fmt.Sprintf("NATS_TUI_OBJ_MIRROR_%d", suffix)
	aggregate := fmt.Sprintf("NATS_TUI_OBJ_AGG_%d", suffix)
	for _, name := range []string{sourceA, sourceB, mirror, aggregate} {
		name := name
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cleanupCancel()
			_ = monitor.DeleteObjectStoreBucket(cleanupCtx, name)
		})
	}
	create := objectstore.NewCreateBucketHandler(monitor)
	base := objectstore.Definition{Storage: objectstore.StorageMemory, MaxBytes: -1, Replicas: 1}
	for _, name := range []string{sourceA, sourceB} {
		definition := base
		definition.Name = name
		if err := create.Handle(ctx, definition); err != nil {
			t.Fatalf("create source %q: %v", name, err)
		}
	}
	storeA, err := monitor.jetstream.ObjectStore(ctx, sourceA)
	if err != nil {
		t.Fatalf("open source A: %v", err)
	}
	storeB, err := monitor.jetstream.ObjectStore(ctx, sourceB)
	if err != nil {
		t.Fatalf("open source B: %v", err)
	}
	if _, err := storeA.PutString(ctx, "a.txt", "from A"); err != nil {
		t.Fatalf("put source A: %v", err)
	}
	if _, err := storeB.PutString(ctx, "b.txt", "from B"); err != nil {
		t.Fatalf("put source B: %v", err)
	}
	mirrorDefinition := base
	mirrorDefinition.Name, mirrorDefinition.Mirror = mirror, sourceA
	if err := create.Handle(ctx, mirrorDefinition); err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	aggregateDefinition := base
	aggregateDefinition.Name, aggregateDefinition.Sources = aggregate, []string{sourceA, sourceB}
	if err := create.Handle(ctx, aggregateDefinition); err != nil {
		t.Fatalf("create aggregate: %v", err)
	}

	wantObject := func(bucket, name, value string) {
		t.Helper()
		store, openErr := monitor.jetstream.ObjectStore(ctx, bucket)
		if openErr != nil {
			t.Fatalf("open replicated bucket %q: %v", bucket, openErr)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			actual, getErr := store.GetString(ctx, name)
			if getErr == nil && actual == value {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("object %s/%s did not replicate", bucket, name)
	}
	wantObject(mirror, "a.txt", "from A")
	wantObject(aggregate, "a.txt", "from A")
	wantObject(aggregate, "b.txt", "from B")
	description := "read-only regional mirror"
	if err := objectstore.NewUpdateBucketHandler(monitor).Handle(ctx, mirror, objectstore.Changes{Description: &description}); err != nil {
		t.Fatalf("update mirror without losing replication: %v", err)
	}
	if _, err := storeA.PutString(ctx, "after-update.txt", "still mirrored"); err != nil {
		t.Fatalf("put source after mirror update: %v", err)
	}
	wantObject(mirror, "after-update.txt", "still mirrored")

	catalog, err := objectstore.NewListBucketsHandler(monitor).Handle(ctx)
	if err != nil {
		t.Fatalf("list replicated buckets: %v", err)
	}
	seenMirror, seenSources := false, false
	for _, bucket := range catalog {
		switch bucket.Name {
		case mirror:
			seenMirror = bucket.Mirror == sourceA && bucket.Description == description
		case aggregate:
			seenSources = len(bucket.Sources) == 2 && bucket.Sources[0] == sourceA && bucket.Sources[1] == sourceB
		}
	}
	if !seenMirror || !seenSources {
		t.Fatalf("mirror=%v sources=%v catalog=%#v", seenMirror, seenSources, catalog)
	}
}

func TestCorePublishAndRequestCollectsMultipleReplies(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error=%v", err)
	}
	t.Cleanup(monitor.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	suffix := time.Now().UnixNano()
	publishSubject := fmt.Sprintf("nats_tui.integration.publish.%d", suffix)
	requestSubject := fmt.Sprintf("nats_tui.integration.request.%d", suffix)

	published, err := monitor.connection.SubscribeSync(publishSubject)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = published.Unsubscribe() })
	message, _ := domainrequestreply.NewMessage(publishSubject, []byte(`{"ok":true}`), map[string][]string{"X-Trace": {"publish"}})
	if err := apprequestreply.NewPublishHandler(monitor).Handle(ctx, message); err != nil {
		t.Fatalf("publish: %v", err)
	}
	received, err := published.NextMsgWithContext(ctx)
	if err != nil || string(received.Data) != `{"ok":true}` || received.Header.Get("X-Trace") != "publish" {
		t.Fatalf("published message=%#v error=%v", received, err)
	}

	responder, err := monitor.connection.Subscribe(requestSubject, func(message *nats.Msg) {
		for _, node := range []string{"a", "b"} {
			response := &nats.Msg{Subject: message.Reply, Data: []byte(fmt.Sprintf(`{"node":%q}`, node)), Header: nats.Header{"X-Node": {node}}}
			_ = monitor.connection.PublishMsg(response)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = responder.Unsubscribe() })
	if err := monitor.connection.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	requestMessage, _ := domainrequestreply.NewMessage(requestSubject, []byte(`{"id":42}`), nil)
	request, _ := domainrequestreply.NewRequest(requestMessage, 2*time.Second, 2)
	replies, err := monitor.Request(ctx, request)
	if err != nil || len(replies) != 2 || replies[0].Headers()["X-Node"][0] != "a" || string(replies[1].Payload()) != `{"node":"b"}` {
		t.Fatalf("replies=%#v error=%v", replies, err)
	}
	missingMessage, _ := domainrequestreply.NewMessage(requestSubject+".missing", nil, nil)
	missingRequest, _ := domainrequestreply.NewRequest(missingMessage, time.Second, 1)
	if _, err := monitor.Request(ctx, missingRequest); !errors.Is(err, apprequestreply.ErrNoResponders) {
		t.Fatalf("no responders error=%v", err)
	}
}

func TestCreateKeyValueMirrorAndSources(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	suffix := time.Now().UnixNano()
	sourceA := fmt.Sprintf("NATS_TUI_SRC_A_%d", suffix)
	sourceB := fmt.Sprintf("NATS_TUI_SRC_B_%d", suffix)
	mirror := fmt.Sprintf("NATS_TUI_MIRROR_%d", suffix)
	aggregate := fmt.Sprintf("NATS_TUI_AGG_%d", suffix)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		for _, name := range []string{aggregate, mirror, sourceB, sourceA} {
			_ = monitor.jetstream.DeleteKeyValue(cleanupCtx, name)
		}
	})
	base := func(name string) kvstore.Definition {
		return kvstore.Definition{Name: name, Storage: kvstore.StorageMemory, History: 1, Replicas: 1, MaxBytes: -1, MaxValueSize: -1}
	}
	for _, definition := range []kvstore.Definition{base(sourceA), base(sourceB)} {
		if err := monitor.CreateKeyValueBucket(ctx, definition); err != nil {
			t.Fatalf("create source %q: %v", definition.Name, err)
		}
	}
	mirrorDefinition := base(mirror)
	mirrorDefinition.Mirror = sourceA
	if err := monitor.CreateKeyValueBucket(ctx, mirrorDefinition); err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	aggregateDefinition := base(aggregate)
	aggregateDefinition.Sources = []string{sourceA, sourceB}
	if err := monitor.CreateKeyValueBucket(ctx, aggregateDefinition); err != nil {
		t.Fatalf("create sourced bucket: %v", err)
	}
	catalog, err := monitor.ListKeyValueBuckets(ctx)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	seenMirror, seenSources := false, false
	for _, bucket := range catalog {
		switch bucket.Name {
		case mirror:
			seenMirror = bucket.Mirror == sourceA
		case aggregate:
			seenSources = len(bucket.Sources) == 2 && bucket.Sources[0] == sourceA && bucket.Sources[1] == sourceB
		}
	}
	if !seenMirror || !seenSources {
		t.Fatalf("mirror=%v sources=%v catalog=%#v", seenMirror, seenSources, catalog)
	}
}

func TestListConsumersReturnsGlobalLagSummary(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}

	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	suffix := time.Now().UnixNano()
	streamName := fmt.Sprintf("NATS_TUI_CONSUMER_%d", suffix)
	subject := fmt.Sprintf("nats-tui.consumer.%d", suffix)
	stream, err := monitor.jetstream.CreateStream(ctx, jetstream.StreamConfig{Name: streamName, Subjects: []string{subject}, Storage: jetstream.MemoryStorage})
	if err != nil {
		t.Fatalf("CreateStream() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = monitor.jetstream.DeleteStream(cleanupCtx, streamName)
	})
	for range 3 {
		if _, err := monitor.jetstream.Publish(ctx, subject, []byte(`{"pending":true}`)); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}
	consumerName := "nats-tui-worker"
	if _, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{Name: consumerName, Durable: consumerName, AckPolicy: jetstream.AckExplicitPolicy, FilterSubject: subject}); err != nil {
		t.Fatalf("CreateConsumer() error = %v", err)
	}

	catalog, err := monitor.ListConsumers(ctx)
	if err != nil {
		t.Fatalf("ListConsumers() error = %v", err)
	}
	for _, consumer := range catalog {
		if consumer.Stream == streamName && consumer.Name == consumerName {
			if consumer.Pending != 3 || consumer.Lag() != 3 {
				t.Fatalf("consumer pending=%d lag=%d, want 3", consumer.Pending, consumer.Lag())
			}
			return
		}
	}
	t.Fatal("created consumer was not returned by global catalog")
}

func TestConsumerOperationsPauseResumeResetAndDelete(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	suffix := time.Now().UnixNano()
	streamName := fmt.Sprintf("NATS_TUI_OPERATIONS_%d", suffix)
	subject := fmt.Sprintf("nats-tui.operations.%d", suffix)
	stream, err := monitor.jetstream.CreateStream(ctx, jetstream.StreamConfig{Name: streamName, Subjects: []string{subject}, Storage: jetstream.MemoryStorage})
	if err != nil {
		t.Fatalf("CreateStream() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = monitor.jetstream.DeleteStream(cleanupCtx, streamName)
	})
	if _, err := monitor.jetstream.Publish(ctx, subject, []byte(`{"operation":true}`)); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	consumerName := "operations-worker"
	if _, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{Name: consumerName, Durable: consumerName, AckPolicy: jetstream.AckExplicitPolicy}); err != nil {
		t.Fatalf("CreateConsumer() error = %v", err)
	}
	ref, err := consumers.NewRef(streamName, consumerName)
	if err != nil {
		t.Fatalf("NewRef() error = %v", err)
	}

	operate := func(action consumers.Action) {
		t.Helper()
		if err := monitor.OperateConsumer(ctx, consumers.Command{Consumer: ref, Action: action}); err != nil {
			t.Fatalf("OperateConsumer(%s) error = %v", action, err)
		}
	}
	operate(consumers.ActionPause)
	consumer, err := stream.Consumer(ctx, consumerName)
	if err != nil {
		t.Fatalf("Consumer() after pause error = %v", err)
	}
	info, err := consumer.Info(ctx)
	if err != nil || !info.Paused {
		t.Fatalf("consumer paused=%v info error=%v", info != nil && info.Paused, err)
	}
	operate(consumers.ActionResume)
	info, err = consumer.Info(ctx)
	if err != nil || info.Paused {
		t.Fatalf("consumer paused=%v after resume, error=%v", info != nil && info.Paused, err)
	}
	operate(consumers.ActionReset)
	operate(consumers.ActionDelete)
	if _, err := stream.Consumer(ctx, consumerName); err == nil {
		t.Fatal("consumer still exists after delete")
	}
}

func TestStreamOperationsCreateAdvancedUpdateAndDelete(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(monitor.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	suffix := time.Now().UnixNano()
	name := fmt.Sprintf("NATS_TUI_STREAM_%d", suffix)
	subject := fmt.Sprintf("nats-tui.stream.%d.>", suffix)
	definition := appstreams.Definition{
		Name: name, Description: "created by integration test", Subjects: []string{subject}, Storage: appstreams.StorageMemory,
		Retention: appstreams.RetentionLimits, MaxMessages: 100, MaxBytes: -1, Replicas: 1,
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = monitor.jetstream.DeleteStream(cleanupCtx, name)
	})
	if err := monitor.OperateStream(ctx, appstreams.Command{Action: appstreams.ActionCreate, Name: name, Definition: definition}); err != nil {
		t.Fatalf("create stream error = %v", err)
	}
	stream, err := monitor.jetstream.Stream(ctx, name)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil || info.Config.MaxMsgs != 100 || info.Config.Storage != jetstream.MemoryStorage {
		t.Fatalf("created config = %#v, error=%v", info.Config, err)
	}
	advanced := info.Config
	advanced.AllowDirect = true
	if _, err := monitor.jetstream.UpdateStream(ctx, advanced); err != nil {
		t.Fatalf("enable advanced config error = %v", err)
	}
	definition.Description = "updated safely"
	definition.MaxMessages = 200
	definition.AllowDirect = true
	if err := monitor.OperateStream(ctx, appstreams.Command{Action: appstreams.ActionUpdate, Name: name, Definition: definition}); err != nil {
		t.Fatalf("update stream error = %v", err)
	}
	info, err = stream.Info(ctx)
	if err != nil || info.Config.Description != "updated safely" || info.Config.MaxMsgs != 200 || !info.Config.AllowDirect {
		t.Fatalf("updated config = %#v, error=%v", info.Config, err)
	}
	for index := 0; index < 3; index++ {
		if _, err := monitor.jetstream.Publish(ctx, strings.TrimSuffix(subject, ".>")+".created", []byte(fmt.Sprintf(`{"id":%d}`, index))); err != nil {
			t.Fatalf("publish test message: %v", err)
		}
	}
	if err := monitor.PurgeStream(ctx, appstreams.PurgeCommand{Stream: name, Keep: 1}); err != nil {
		t.Fatalf("purge stream: %v", err)
	}
	info, err = stream.Info(ctx)
	if err != nil || info.State.Msgs != 1 {
		t.Fatalf("messages after purge=%d error=%v", info.State.Msgs, err)
	}
	if err := monitor.DeleteStreamMessage(ctx, appstreams.MessageDeleteCommand{Stream: name, Sequence: info.State.LastSeq, Secure: true}); err != nil {
		t.Fatalf("secure delete message: %v", err)
	}
	info, err = stream.Info(ctx)
	if err != nil || info.State.Msgs != 0 {
		t.Fatalf("messages after delete=%d error=%v", info.State.Msgs, err)
	}
	if err := monitor.OperateStream(ctx, appstreams.Command{Action: appstreams.ActionDelete, Name: name}); err != nil {
		t.Fatalf("delete stream error = %v", err)
	}
	if _, err := monitor.jetstream.Stream(ctx, name); err == nil {
		t.Fatal("stream still exists after delete")
	}
}

func TestStreamMirrorAndMultipleSources(t *testing.T) {
	server := os.Getenv("NATS_TUI_TEST_SERVER")
	if server == "" {
		t.Skip("NATS_TUI_TEST_SERVER is not set")
	}
	monitor, err := Connect(config.Settings{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(monitor.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	suffix := time.Now().UnixNano()
	originA, originB := fmt.Sprintf("NATS_TUI_ORIGIN_A_%d", suffix), fmt.Sprintf("NATS_TUI_ORIGIN_B_%d", suffix)
	mirror, aggregate := fmt.Sprintf("NATS_TUI_MIRROR_%d", suffix), fmt.Sprintf("NATS_TUI_AGGREGATE_%d", suffix)
	for _, name := range []string{aggregate, mirror, originB, originA} {
		name := name
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cleanupCancel()
			_ = monitor.jetstream.DeleteStream(cleanupCtx, name)
		})
	}
	base := func(name, subject string) appstreams.Definition {
		return appstreams.Definition{Name: name, Subjects: []string{subject}, Storage: appstreams.StorageMemory, Retention: appstreams.RetentionLimits, MaxMessages: -1, MaxBytes: -1, MaxMessagesPerSubject: -1, MaxMessageSize: -1, MaxConsumers: -1, Replicas: 1, Discard: appstreams.DiscardOld}
	}
	originADefinition := base(originA, fmt.Sprintf("origin.a.%d.>", suffix))
	originADefinition.AllowDirect = true
	for _, definition := range []appstreams.Definition{originADefinition, base(originB, fmt.Sprintf("origin.b.%d.>", suffix))} {
		if err := monitor.OperateStream(ctx, appstreams.Command{Action: appstreams.ActionCreate, Name: definition.Name, Definition: definition}); err != nil {
			t.Fatal(err)
		}
	}
	mirrorDefinition := base(mirror, "")
	mirrorDefinition.Subjects = nil
	mirrorDefinition.Mirror = &appstreams.Source{Name: originA, StartSequence: 1}
	mirrorDefinition.MirrorDirect = true
	if err := monitor.OperateStream(ctx, appstreams.Command{Action: appstreams.ActionCreate, Name: mirror, Definition: mirrorDefinition}); err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	aggregateDefinition := base(aggregate, "")
	aggregateDefinition.Subjects = nil
	aggregateDefinition.Sources = []appstreams.Source{{Name: originA}, {Name: originB}}
	if err := monitor.OperateStream(ctx, appstreams.Command{Action: appstreams.ActionCreate, Name: aggregate, Definition: aggregateDefinition}); err != nil {
		t.Fatalf("create aggregate: %v", err)
	}
	items, err := monitor.ListStreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seenMirror, seenAggregate := false, false
	var mirrorItem appstreams.Summary
	for _, item := range items {
		if item.Name == mirror {
			mirrorItem = item
			seenMirror = item.Mirror != nil && item.Mirror.Name == originA && item.MirrorDirect
		}
		if item.Name == aggregate {
			seenAggregate = len(item.Sources) == 2 && item.Sources[0].Name == originA && item.Sources[1].Name == originB
		}
	}
	if !seenMirror || !seenAggregate {
		t.Fatalf("mirror=%v aggregate=%v mirror_item=%+v", seenMirror, seenAggregate, mirrorItem)
	}
}
