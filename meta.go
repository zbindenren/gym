package gym

import (
	"encoding/xml"
	"io/ioutil"
	"path"
)

type metaFile struct {
	name         string
	fileType     string
	checksumType string
	checksum     string
	size         int
	href         string
}

type metaFiles []metaFile

func (metaFiles *metaFiles) get(fileType string) (metaFile, bool) {
	for _, m := range *metaFiles {
		if m.fileType == fileType {
			return m, true
		}
	}
	return metaFile{}, false
}

func newMetafiles(pathToXML string) (metaFiles, error) {
	data, err := ioutil.ReadFile(pathToXML)
	if err != nil {
		return nil, err
	}
	var rm = repomd{}
	if err := xml.Unmarshal(data, &rm); err != nil {
		return nil, err
	}
	l := []metaFile{}
	for _, d := range rm.Data {
		l = append(l, metaFile{
			name:         path.Base(d.Location.Href),
			fileType:     d.Type,
			checksumType: d.Checksum.Type,
			checksum:     d.Checksum.Value,
			size:         d.Size,
			href:         d.Location.Href,
		})
	}
	return l, nil
}

// following types are needed for xml parsing
type repomd struct {
	XMLName xml.Name `xml:"repomd"`
	Data    []data   `xml:"data"`
}

type data struct {
	Type     string   `xml:"type,attr"`
	Location location `xml:"location"`
	Checksum checksum `xml:"checksum"`
	Size     int      `xml:"size"`
}

type location struct {
	Href string `xml:"href,attr"`
}

type checksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type rpmPackage struct {
	Checksum checksum `xml:"checksum"`
	Location location `xml:"location"`
	Size     size     `xml:"size"`
}

type size struct {
	Archive int `xml:"archive,attr"`
}
