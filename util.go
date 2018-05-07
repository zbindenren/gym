package gym

import (
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/xi2/xz"
	"gopkg.in/inconshreveable/log15.v2"
)

// ProcessSQLFunc is the function type called for the rows created by processSqlite.
type processSQLFunc func(rows *sql.Rows) error

// processSqlite makes a sqlite db connection and performs query on the sqlite db.
// The resulting rows can be processed by ProcessFunc.
func processSqlite(pathToDB string, query string, processFn processSQLFunc) error {
	db, err := sql.Open("sqlite3", pathToDB)
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	err = processFn(rows)
	rows.Close()
	return err
}

// ProcessXMLFunc is the function type called for an xml decoder.
type processXMLFunc func(t *xml.Decoder) error

func processXML(pathToXML string, processFn processXMLFunc) error {
	f, err := os.Open(pathToXML)
	if err != nil {
		return err
	}
	defer f.Close()
	return processFn(xml.NewDecoder(f))
}

func countResult(pathToDB string, filter string) (int, error) {
	db, err := sql.Open("sqlite3", pathToDB)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	query := "select count(*) from packages"
	if len(filter) > 0 {
		query = query + " where location_href like '%" + filter + "%'"
	}
	var count int
	if err := db.QueryRow(query, 1).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func totalBytes(pathToDB string, filter string) (int64, error) {
	db, err := sql.Open("sqlite3", pathToDB)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	query := "select sum(size_archive) from packages"
	if len(filter) > 0 {
		query = query + " where location_href like '%" + filter + "%'"
	}
	var total int64
	if err := db.QueryRow(query, 1).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func uncompress(pathToFile string) (*os.File, error) {
	var r io.Reader
	fh, err := os.Open(pathToFile)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	switch path.Ext(pathToFile) {
	case ".bz2":
		r = bzip2.NewReader(fh)
	case ".gz":
		r, err = gzip.NewReader(fh)
		if err != nil {
			return nil, err
		}
	case ".xz":
		r, err = xz.NewReader(fh, xz.DefaultDictMax)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s has wrong file extension, currently supported %s", pathToFile, ".bz2, .gz, .xz")
	}

	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, err
	}
	defer tmpFile.Close()
	if _, err := io.Copy(tmpFile, r); err != nil {
		return nil, err
	}
	return tmpFile, nil
}

// ConfigureTransport configures the http client transport (ssl, proxy)
func ConfigureTransport(insecure bool, clientCertFile string, clientKeyFile string, caCerts ...string) (*http.Transport, error) {
	Log.Debug("configure transport",
		"insecure", insecure,
		"clientCertFile", clientCertFile,
		"clientKeyFile", clientKeyFile,
		"cacerts", strings.Join(caCerts, ","),
	)
	transport := new(http.Transport)
	tlsConfig := &tls.Config{InsecureSkipVerify: insecure}
	if len(clientCertFile) != 0 && len(clientKeyFile) != 0 {
		clientCert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}
	if len(caCerts) > 0 {
		caCertPool := x509.NewCertPool()
		for _, cert := range caCerts {
			if len(cert) == 0 {
				continue
			}
			caCert, err := ioutil.ReadFile(cert)
			if err != nil {
				return nil, err
			}
			caCertPool.AppendCertsFromPEM(caCert)
		}
		tlsConfig.RootCAs = caCertPool
	}
	tlsConfig.BuildNameToCertificate()
	transport.TLSClientConfig = tlsConfig
	proxyURL, err := url.Parse(os.Getenv("http_proxy"))
	if err == nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return transport, nil
}

func checksumOK(pathToFile string, checksumType string, checksum string) bool {
	fh, err := ioutil.ReadFile(pathToFile)
	if err != nil {
		return false
	}
	if len(checksumType) == 0 {
		return true
	}
	calculatedChecksum := ""
	switch checksumType {
	case "sha256":
		calculatedChecksum = fmt.Sprintf("%x", sha256.Sum256(fh))
	default:

		calculatedChecksum = fmt.Sprintf("%x", sha1.Sum(fh))
	}
	return calculatedChecksum == checksum
}

func ellipsis(s string, max int) string {
	if len(s) <= max {
		return s + strings.Repeat(".", max-len(s))
	}
	return s[:max]
}

func createExitOnCritHandler() log15.Handler {
	h := log15.FilterHandler(func(r *log15.Record) bool {
		if r.Lvl == log15.LvlCrit {
			os.Exit(1)
		}
		return false
	}, log15.StdoutHandler)
	return h
}

func copyFile(source, dest string) error {
	s, err := os.Open(source)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}

func copyDir(source, dest string) error {
	filepath.Walk(source, func(currentPath string, info os.FileInfo, _ error) error {
		destPath := path.Join(dest, currentPath[len(path.Dir(source)):])
		if info.IsDir() {
			Log.Debug("creating directory", "path", destPath)
			if err := os.Mkdir(destPath, info.Mode()); err != nil {
				return err
			}
			return nil
		}
		Log.Debug("copy file", "source", currentPath, "dest", destPath)
		return copyFile(currentPath, destPath)
	})
	return nil
}
