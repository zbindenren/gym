// +build integration

package gym

import (
	"os"
	"path"
	"testing"
)

var (
	snapshotDir = "/tmp/snapshot"
)

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

func TestSnapshotCopy(t *testing.T) {
	defer teardown()
	r := NewRepo("./testdata/repo", "", nil)
	if err := r.Snapshot(snapshotDir, false, true, 1); err != nil {
		t.Error(err)
	}
	filenames := []string{path.Join(snapshotDir, "repo/repodata/repomd.xml"), path.Join(snapshotDir, "repo/Packages/GeoIP-devel-1.5.0-9.el7.i686.rpm")}
	for _, filename := range filenames {
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Errorf("snapshot create failed, file %s does not exist", filename)
		}
	}
}

func TestSnapshotLink(t *testing.T) {
	defer teardown()
	r := NewRepo("./testdata/repo", "", nil)
	if err := r.Snapshot(snapshotDir, true, false, 1); err != nil {
		t.Error(err)
	}
	filenames := []string{path.Join(snapshotDir, "repo/repodata/repomd.xml"), path.Join(snapshotDir, "repo/Packages/GeoIP-devel-1.5.0-9.el7.i686.rpm")}
	for _, filename := range filenames {
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Errorf("snapshot create failed, file %s does not exist", filename)
		}
	}
}

func TestSnapshotInvalidSource(t *testing.T) {
	invalidRepos := []string{"./testdata", "/does/not/exist"}
	for _, repo := range invalidRepos {
		r := NewRepo(repo, "", nil)
		if err := r.Snapshot(snapshotDir, false, true, 1); err == nil {
			t.Errorf("%s is not a valid repository, but no error produced", repo)
		}
	}
}

func TestSnapshotDestExists(t *testing.T) {
	defer teardown()
	r := NewRepo("./testdata/repo", "", nil)
	if err := r.Snapshot(snapshotDir, false, true, 1); err != nil {
		t.Error(err)
	}
	if err := r.Snapshot(snapshotDir, false, true, 1); err == nil {
		t.Errorf("destination %s already exists, but no error produced", path.Join(snapshotDir, "repo"))
	}
}
