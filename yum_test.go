// +build integration

package gym

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	retCode := m.Run()
	teardown()
	os.Exit(retCode)
}

func TestLsMeta(t *testing.T) {
	repo := "http://mirror.centos.org/centos/7/os/x86_64"
	r := NewRepo("/tmp/repo", repo, nil)

	if err := r.SyncMeta(); err != nil {
		t.Fatal(err)
	}
	if err := r.Sync("zsh", 2); err != nil {
		t.Fatal(err)
	}
}

func TestDownload(t *testing.T) {
	url := "http://mirror.centos.org/centos/7/os/x86_64/Packages/LibRaw-0.14.8-5.el7.20120830git98d925.x86_64.rpm"
	dest := "/tmp/LibRaw-0.14.8-5.el7.20120830git98d925.x86_64.rpm"
	checksum := "b5b9f746d4e1a95c6ee8f5da381dd6bc339f1dc1e018c06c2b4f0b3c3446f558"
	checksumType := "sha256"
	r := NewRepo("/tmp", "", nil)
	if _, err := r.download(url, dest, checksum, checksumType); err != nil {
		t.Error(err)
	}
}

func TestEmptySqliteDB(t *testing.T) {
	r := NewRepo("./testdata/emptysqlite", "", nil)
	err := r.rpmList("")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncFromRepofile(t *testing.T) {
	repos, err := NewRepoList("./testdata/fedora.repo", "/tmp/fedora", false, "22", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Errorf("got %d repos, wanted %d repos", len(repos), 1)
	}
	r := repos[0]
	url := "http://ftp.linux.cz/pub/linux/fedora/linux/releases/22/Server/x86_64/os"
	if r.RemoteURL != url {
		t.Errorf("got url %s, should be %s", r.RemoteURL, url)

	}
}

func teardown() {
	os.RemoveAll("/tmp/repo")
	os.Remove("/tmp/LibRaw-0.14.8-5.el7.20120830git98d925.x86_64.rpm")
}
