package rekordbox

import (
	"encoding/xml"
	"fmt"
	"io"
)

// Collection is a parsed rekordbox XML export.
type Collection struct {
	ProductName    string
	ProductVersion string

	// DeclaredEntries is the COLLECTION Entries attribute. It is kept separate
	// from len(Tracks) on purpose: a disagreement between the two is how a
	// truncated export is detected.
	DeclaredEntries int

	// HasCollection and HasPlaylists record whether the elements were present
	// at all, which distinguishes an empty collection from a wrong file.
	HasCollection bool
	HasPlaylists  bool

	// PlaylistCount is the Count attribute of the root playlist node.
	PlaylistCount int

	Tracks []Track
}

// Track is one <TRACK> element, limited to the fields these tools use. The real
// export carries roughly 25 attributes; the rest are ignored.
//
// The xml tags sit directly on this exported type rather than on a private
// mirror struct. Every attribute name already matches its field name, so a
// separate decode struct plus a field-by-field copy would only add a second
// place for a field to go missing, without buying any independence.
//
// AverageBpm and Tonality stay strings because they are formatted values
// ("108.00", "11A") that no consumer needs as numbers yet.
type Track struct {
	TrackID    string `xml:"TrackID,attr"`
	Name       string `xml:"Name,attr"`
	Artist     string `xml:"Artist,attr"`
	Album      string `xml:"Album,attr"`
	Genre      string `xml:"Genre,attr"`
	Location   string `xml:"Location,attr"`
	AverageBpm string `xml:"AverageBpm,attr"`
	Tonality   string `xml:"Tonality,attr"`
	Comments   string `xml:"Comments,attr"`
	DateAdded  string `xml:"DateAdded,attr"`
	Rating     int    `xml:"Rating,attr"`
	PlayCount  int    `xml:"PlayCount,attr"`
	TotalTime  int    `xml:"TotalTime,attr"`
	Year       int    `xml:"Year,attr"`
}

type xmlDoc struct {
	XMLName   xml.Name       `xml:"DJ_PLAYLISTS"`
	Product   xmlProduct     `xml:"PRODUCT"`
	Coll      *xmlCollection `xml:"COLLECTION"`
	Playlists *xmlPlaylists  `xml:"PLAYLISTS"`
}

type xmlProduct struct {
	Name    string `xml:"Name,attr"`
	Version string `xml:"Version,attr"`
}

// xmlCollection is the envelope around the track list. Two decoder behaviours
// worth recording, both confirmed by experiment rather than assumed:
//
//   - Only TRACK elements nested inside COLLECTION are captured. A stray TRACK
//     elsewhere in the document is silently dropped, not an error.
//   - Were a document to contain two COLLECTION elements, their tracks would be
//     appended into a single slice while Entries kept only the last value,
//     rather than failing. A real export has exactly one.
type xmlCollection struct {
	Entries int     `xml:"Entries,attr"`
	Tracks  []Track `xml:"TRACK"`
}

// xmlPlaylists captures only the root node. The playlist tree is not needed for
// pruning; dj playlist will extend this when it needs the hierarchy.
type xmlPlaylists struct {
	Root *xmlPlaylistNode `xml:"NODE"`
}

type xmlPlaylistNode struct {
	Name  string `xml:"Name,attr"`
	Count int    `xml:"Count,attr"`
}

// Parse decodes a rekordbox XML export.
//
// The document is decoded whole rather than streamed. The reference export is
// 16 MB and the Collection it retains is about 4 MB; transient allocation
// during the decode is roughly an order of magnitude larger than the file,
// which is unremarkable for a command that runs once per invocation.
func Parse(r io.Reader) (*Collection, error) {
	var doc xmlDoc
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode rekordbox xml: %w", err)
	}

	c := &Collection{
		ProductName:    doc.Product.Name,
		ProductVersion: doc.Product.Version,
		HasCollection:  doc.Coll != nil,
		HasPlaylists:   doc.Playlists != nil,
	}

	if doc.Coll != nil {
		c.DeclaredEntries = doc.Coll.Entries
		c.Tracks = doc.Coll.Tracks
	}

	if doc.Playlists != nil && doc.Playlists.Root != nil {
		c.PlaylistCount = doc.Playlists.Root.Count
	}

	return c, nil
}
