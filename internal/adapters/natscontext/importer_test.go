package natscontext

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

type mapFS struct{ fstest.MapFS }

func (m mapFS) ReadDir(path string) ([]fs.DirEntry, error) { return fs.ReadDir(m.MapFS, path) }
func (m mapFS) ReadFile(path string) ([]byte, error)       { return fs.ReadFile(m.MapFS, path) }

func TestImporterMapsOfficialContextFieldsDeterministically(t *testing.T) {
	files := mapFS{fstest.MapFS{
		"contexts/prod.json": {Data: []byte(`{"url":"nats://a:4222,nats://b:4222","user_jwt":"file:///secure/user.jwt","user_seed":"file:///secure/user.seed","ca":"/tls/ca.pem","cert":"/tls/client.pem","key":"/tls/client.key"}`)},
		"contexts/README":    {Data: []byte("ignored")},
	}}
	items, err := NewImporter(files).Profiles(t.Context(), "contexts")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "prod" || items[0].UserJWT != "/secure/user.jwt" || len(items[0].Servers) != 1 {
		t.Fatalf("items=%+v", items)
	}
	profile, err := items[0].Profile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.Authentication().Kind() != "jwt" || !profile.TLS().Enabled() {
		t.Fatalf("profile=%+v", profile)
	}
}
