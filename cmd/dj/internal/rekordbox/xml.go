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

// Track is one <TRACK> element, limited to the fields these tools use.
// AverageBpm and Tonality stay strings because they are formatted values
// ("108.00", "11A") that no consumer needs as numbers yet.
type Track struct {
	TrackID    string
	Name       string
	Artist     string
	Album      string
	Genre      string
	Location   string
	AverageBpm string
	Tonality   string
	Comments   string
	DateAdded  string
	Rating     int
	PlayCount  int
	TotalTime  int
	Year       int
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

type xmlCollection struct {
	Entries int        `xml:"Entries,attr"`
	Tracks  []xmlTrack `xml:"TRACK"`
}

type xmlTrack struct {
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

// xmlPlaylists captures only the root node. The playlist tree is not needed for
// pruning; dj playlist will extend this when it needs the hierarchy.
type xmlPlaylists struct {
	Root *xmlPlaylistNode `xml:"NODE"`
}

type xmlPlaylistNode struct {
	Name  string `xml:"Name,attr"`
	Count int    `xml:"Count,attr"`
}

// Parse decodes a rekordbox XML export. The reference export is 17 MB, so it is
// read whole rather than streamed.
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
		c.Tracks = make([]Track, 0, len(doc.Coll.Tracks))
		for _, t := range doc.Coll.Tracks {
			c.Tracks = append(c.Tracks, Track{
				TrackID:    t.TrackID,
				Name:       t.Name,
				Artist:     t.Artist,
				Album:      t.Album,
				Genre:      t.Genre,
				Location:   t.Location,
				AverageBpm: t.AverageBpm,
				Tonality:   t.Tonality,
				Comments:   t.Comments,
				DateAdded:  t.DateAdded,
				Rating:     t.Rating,
				PlayCount:  t.PlayCount,
				TotalTime:  t.TotalTime,
				Year:       t.Year,
			})
		}
	}

	if doc.Playlists != nil && doc.Playlists.Root != nil {
		c.PlaylistCount = doc.Playlists.Root.Count
	}

	return c, nil
}
