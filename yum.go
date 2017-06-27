package gym

import (
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	// sql driver for sql db
	"gopkg.in/inconshreveable/log15.v2"
	"gopkg.in/ini.v1"
)

var (
	// Log is exported so that is is usable for other packages
	Log       log15.Logger
	logFormat log15.Format
	logLevel  log15.Lvl
)

func init() {
	// setup initial logging
	logFormat = log15.TerminalFormat()
	logLevel = log15.LvlInfo
	Log = log15.New()
	lh := log15.LvlFilterHandler(log15.LvlInfo, log15.StreamHandler(os.Stdout, logFormat))
	Log.SetHandler(log15.MultiHandler(lh, createExitOnCritHandler()))
}

// NoColor disables color log output
func NoColor() {
	logFormat = log15.LogfmtFormat()
	updateLogger()
}

// Debug enables debug messages
func Debug() {
	logLevel = log15.LvlDebug
	updateLogger()
}

func updateLogger() {
	lh := log15.LvlFilterHandler(logLevel, log15.StreamHandler(os.Stdout, logFormat))
	Log.SetHandler(log15.MultiHandler(lh, createExitOnCritHandler()))
}

// Repo represents a Yum repository
type Repo struct {
	LocalPath  string
	RemoteURL  string
	Name       string
	Enabled    bool
	Client     *http.Client
	rpmc       chan *rpm
	resultc    chan *result
	errorc     chan error
	done       chan bool
	total      int
	totalBytes int64
}

// NewRepo creates a new repository
func NewRepo(local string, remote string, transport *http.Transport) *Repo {
	l := strings.TrimRight(local, "/")
	r := strings.TrimRight(remote, "/")
	client := new(http.Client)
	if transport != nil {
		client = &http.Client{Transport: transport}
	}
	repo := Repo{
		Client:    client,
		LocalPath: l,
		RemoteURL: r,
		resultc:   make(chan *result),
		done:      make(chan bool),
	}
	return &repo
}

// RepoList represents a list of yum repositories
type RepoList []Repo

// Find returns Repo with Name name
func (rl RepoList) Find(name string) *Repo {
	for _, r := range rl {
		if r.Name == name {
			return &r
		}
	}
	return nil
}

// NewRepoList creates an new RepoList
func NewRepoList(pathToYumConf string, dest string, insecure bool, release string, baseArch string) (RepoList, error) {
	repos := RepoList{}
	cfg, err := ini.Load(pathToYumConf)
	if err != nil {
		return repos, err
	}
	for _, s := range cfg.Sections() {
		if len(s.Keys()) > 0 {
			urlKey, err := s.GetKey("baseurl")
			if err != nil {
				return repos, err
			}
			url := strings.Replace(strings.Replace(urlKey.Value(), "$basearch", baseArch, -1), "$releasever", release, -1)
			transport, err := ConfigureTransport(insecure, s.Key("sslclientcert").String(), s.Key("sslclientkey").String(), s.Key("sslcacert").String())
			if err != nil {
				return repos, err
			}
			r := NewRepo(path.Join(dest, s.Name()), url, transport)
			r.Name = s.Name()
			enabled, err := s.Key("enabled").Int()
			if err == nil {
				if enabled == 1 {
					r.Enabled = true
				}
			}
			repos = append(repos, *r)
		}
	}
	return repos, nil
}

// Sync synchronizes remote RPMs to the local filesystem
func (r *Repo) Sync(filter string, numWorkers int) error {
	if err := r.rpmList(filter); err != nil {
		return err
	}
	Log.Info("starting rpm sync", "name", r.Name, "totalPackages", r.total, "totalBytes", r.totalBytes)
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			r.downloadWorker(id)
			wg.Done()
		}(i + 1)
	}

	go func() {
		wg.Wait()
		close(r.resultc)
	}()

	var currentBytes int64
	for res := range r.resultc {
		if res.err != nil {
			Log.Error(path.Base(res.rpm.relPath), "status", res.status, "workerid", res.workerID, "err", res.err)
		} else {
			currentBytes = currentBytes + int64(res.rpm.size)
			progress := float64(currentBytes)
			progressMsg := "%.f bytes"
			if r.totalBytes > 0 {
				progressMsg = "%.2f%%"
				progress = progress * float64(100) / float64(r.totalBytes)
			}
			if res.err != nil {
			}
			Log.Info(ellipsis(path.Base(res.rpm.relPath), 40), "status", res.status, "err", res.err, "progress", fmt.Sprintf(progressMsg, progress), "numBytes", res.bytesDownloaded, "workerid", res.workerID)
		}
	}

	if err := <-r.errorc; err != nil {
		return err
	}
	return nil
}

// SyncMeta downloads the repository's metadata comps.xml, repomd.xml filelist.xml etc...
func (r *Repo) SyncMeta() error {
	if _, err := r.download(r.RemoteURL+"/repodata/repomd.xml", path.Join(r.LocalPath, ".newrepodata", "repomd.xml"), "", ""); err != nil {
		return err
	}
	metaFiles, err := r.lsMeta()
	if err != nil {
		return err
	}

	errorc := make(chan error, len(metaFiles))
	var wg sync.WaitGroup
	for _, m := range metaFiles {
		wg.Add(1)
		go func(m metaFile) {
			defer wg.Done()
			if _, err := r.download(r.RemoteURL+"/"+m.href, path.Join(r.LocalPath, "/.new"+m.href), m.checksum, m.checksumType); err != nil {
				errorc <- fmt.Errorf("download failed, url=%s, dest=%s, err=%s", r.RemoteURL+"/"+m.href, path.Join(r.LocalPath), err)
			}
		}(m)
	}

	wg.Wait()
	// check if there was an error
	select {
	case err, ok := <-errorc:
		if ok {
			return err
		}
	default:
	}

	if err := os.RemoveAll(path.Join(r.LocalPath, "repodata")); err != nil {
		return err
	}
	if err := os.Rename(path.Join(r.LocalPath, ".newrepodata"), path.Join(r.LocalPath, "repodata")); err != nil {
		return err
	}
	return nil
}

func (r *Repo) Snapshot(dest string, link bool, createRepo bool, numWorkers int) error {
	if _, err := os.Stat(path.Join(r.LocalPath, "repodata/repomd.xml")); err != nil {
		return fmt.Errorf("%s is not a valid repository, repomd.xml does not exist", r.LocalPath)
	}
	destination := path.Join(dest, path.Base(r.LocalPath))
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("destination %s already exists", destination)
	}

	if err := r.rpmList(""); err != nil {
		return err
	}
	Log.Info("creating snapshot", "name", r.Name, "src", r.LocalPath, "dest", destination)
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			r.snapshotWorker(destination, link, id)
			wg.Done()
		}(i + 1)
	}

	go func() {
		wg.Wait()
		close(r.resultc)
	}()

	mode := "copy"
	if link {
		mode = "link"
	}
	for res := range r.resultc {
		if res.err != nil {
			Log.Error(path.Base(res.rpm.relPath), "status", res.status, "workerid", res.workerID, "err", res.err)
		} else {
			Log.Info(ellipsis(path.Base(res.rpm.relPath), 40), "mode", mode, "err", res.err, "workerid", res.workerID)
		}
	}

	if err := <-r.errorc; err != nil {
		return err
	}
	if !createRepo {
		return copyDir(path.Join(r.LocalPath, "repodata"), destination)
	}

	cmdString, err := exec.LookPath("createrepo")
	if err != nil {
		return err
	}
	metaFiles, err := r.lsMeta()
	if err != nil {
		return err
	}
	args := []string{"-d"}
	if meta, ok := metaFiles.get("group"); ok {
		args = append(args, path.Join(r.LocalPath, meta.href))
	}
	args = append(args, destination)
	cmd := exec.Command(cmdString, args...)
	out, err := cmd.CombinedOutput()
	Log.Debug("run create repo", "cmd", strings.Join(cmd.Args, " "), "out", string(out))
	if err != nil {
		return fmt.Errorf("create repo failed, err: %s, output: %s", err, string(out))
	}
	return nil
}

// rpmList reads the available rpms from sqlite db and puts the RPM in a channel for later processing
func (r *Repo) rpmList(filter string) error {
	metaFiles, err := r.lsMeta()
	if err != nil {
		return err
	}
	primary, ok := metaFiles.get("primary_db")
	if ok {
		return r.rpmListFromSqlite(filter, primary)
	}
	primary, ok = metaFiles.get("primary")
	if !ok {
		return errors.New("now primary db sqlite or xml file found")
	}
	return r.rpmListFromXML(filter, primary)

}

// rpmListFromSqlite reads the available rpms from sqlite db and puts the RPM in a channel for later processing
func (r *Repo) rpmListFromSqlite(filter string, primary metaFile) error {
	r.rpmc = make(chan *rpm)
	r.errorc = make(chan error, 1)

	tmpFile, err := uncompress(path.Join(r.LocalPath, "repodata", primary.name))
	if err != nil {
		return err
	}

	r.total, err = countResult(tmpFile.Name(), filter)
	if err != nil {
		return err
	}
	if r.total == 0 {
		close(r.rpmc)
		close(r.errorc)
		r.totalBytes = 0
		return nil
	}
	r.totalBytes, err = totalBytes(tmpFile.Name(), filter)
	if err != nil {
		return err
	}
	query := "select location_href, size_archive, checksum_type, pkgId from packages"
	if len(filter) > 0 {
		query = query + " where location_href like '%" + filter + "%'"
	}
	go func() {
		// Close rpms channel after we have rpm list
		defer close(r.rpmc)
		defer close(r.errorc)
		i := 0
		r.errorc <- processSqlite(tmpFile.Name(), query, func(rows *sql.Rows) error {
			for rows.Next() {
				i++
				var locationHref, checksum, checksumType string
				var sizeArchive int
				if err := rows.Scan(&locationHref, &sizeArchive, &checksumType, &checksum); err != nil {
					return err
				}
				rpm := newRPM(locationHref, checksum, checksumType, sizeArchive)
				rpm.downloadID = i
				select {
				case r.rpmc <- rpm:
				case <-r.done:
					return errors.New("rpmlist canceled")
				}
			}
			return nil
		})
	}()
	return nil
}

// rpmListFromXML reads the available rpms from xml and puts the RPM in a channel for later processing
func (r *Repo) rpmListFromXML(filter string, primary metaFile) error {
	r.rpmc = make(chan *rpm)
	r.errorc = make(chan error, 1)

	tmpFile, err := uncompress(path.Join(r.LocalPath, "repodata", primary.name))
	if err != nil {
		return err
	}

	if len(filter) > 0 {
	}
	go func() {
		// Close rpms channel after we have rpm list
		defer close(r.rpmc)
		defer close(r.errorc)
		i := 0
		r.errorc <- processXML(tmpFile.Name(), func(decoder *xml.Decoder) error {
			for {
				i++
				// Read tokens from the XML document in a stream.
				t, _ := decoder.Token()
				if t == nil {
					break
				}
				switch se := t.(type) {
				case xml.StartElement:
					if se.Name.Local == "package" {
						var p rpmPackage
						decoder.DecodeElement(&p, &se)
						rpm := newRPM(p.Location.Href, p.Checksum.Value, p.Checksum.Type, p.Size.Archive)
						rpm.downloadID = i
						if len(filter) > 0 && !strings.Contains(rpm.relPath, filter) {
							continue
						}
						select {
						case r.rpmc <- rpm:
						case <-r.done:
							return errors.New("rpmlist canceled")
						}
					}
				}
			}
			return nil
		})
	}()
	return nil
}

// download url and verify checksum of downloaded file, if shaType is empty no verification is done
func (r *Repo) download(url string, dest string, checksum string, shaType string) (int64, error) {
	Log.Debug(ellipsis(path.Base(url), 40), "destdir", path.Dir(dest), "sumType", shaType, "checksum", checksum)
	if _, err := os.Stat(dest); err == nil {
		if len(shaType) > 0 && checksumOK(dest, shaType, checksum) {
			return 0, nil
		}
	}
	size, err := r.Download(url, dest)
	if err != nil {
		return 0, err
	}
	if !checksumOK(dest, shaType, checksum) {
		return size, errors.New("checksum missmatch")
	}
	return size, nil
}

// download url and verify checksum of downloaded file, if shaType is empty no verification is done
func (r *Repo) Download(url string, dest string) (int64, error) {
	if err := os.MkdirAll(path.Dir(dest), 0755); err != nil {
		return 0, err
	}

	out, err := os.Create(dest)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	// req.Header.Add("Accept-Encoding", "gzip") //otherwise the client decompresses *.gz files, that is not what we want
	resp, err := r.Client.Do(req)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode > 299 {
		return 0, fmt.Errorf("http status: %s", resp.Status)
	}
	defer resp.Body.Close()
	size, err := io.Copy(out, resp.Body)
	if err != nil {
		return 0, err
	}
	return size, nil
}

type result struct {
	rpm             *rpm
	err             error
	workerID        int
	progress        string
	bytesDownloaded int64
	status          string
}

func newResult(rpm *rpm, workerID int, bytesDownloaded int64, err error) *result {
	status := "downld"
	if bytesDownloaded == 0 {
		status = "cached"
	}
	if err != nil {
		status = "failed"
	}
	return &result{
		rpm:             rpm,
		workerID:        workerID,
		status:          status,
		bytesDownloaded: bytesDownloaded,
		err:             err,
	}
}

// downloadWorker gets its rpms from a channel and downloads the corresponding rpm
func (r *Repo) downloadWorker(id int) {
	i := 0
	for rpm := range r.rpmc {
		i++
		bytesDownloaded, err := r.download(r.RemoteURL+"/"+rpm.relPath, path.Join(r.LocalPath, rpm.relPath), rpm.checksum, rpm.checksumType)
		res := newResult(rpm, id, bytesDownloaded, err)
		select {
		case r.resultc <- res:
		case <-r.done:
			Log.Info("sync canceled")
			return
		}
	}
}

func (r *Repo) snapshotWorker(dest string, link bool, id int) {
	i := 0
	for rpm := range r.rpmc {
		i++
		err := r.copyOrLink(dest, rpm, link)
		res := newResult(rpm, id, 0, err)
		select {
		case r.resultc <- res:
		case <-r.done:
			Log.Info("snapshot canceled")
			return
		}
	}
}

func (r *Repo) copyOrLink(destDir string, rpm *rpm, link bool) error {
	source := path.Join(r.LocalPath, rpm.relPath)
	destPath := path.Join(destDir, rpm.relPath)
	if err := os.MkdirAll(path.Dir(destPath), 0755); err != nil {
		return err
	}
	if link {
		sourceAbs, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		Log.Debug("link rpm", "source", source, "dest", destPath)
		return os.Symlink(sourceAbs, destPath)
	}
	// copy rpm
	Log.Debug("copy rpm", "source", source, "dest", destPath)
	if err := copyFile(source, destPath); err != nil {
		return err
	}
	if !checksumOK(destPath, rpm.checksumType, rpm.checksum) {
		return errors.New("checksum missmatch")
	}
	return nil
}

func (r *Repo) lsMeta() (metaFiles, error) {
	if _, err := os.Stat(path.Join(r.LocalPath, ".newrepodata", "repomd.xml")); err == nil {
		return newMetafiles(path.Join(r.LocalPath, ".newrepodata", "repomd.xml"))
	}
	return newMetafiles(path.Join(r.LocalPath, "repodata", "repomd.xml"))
}
