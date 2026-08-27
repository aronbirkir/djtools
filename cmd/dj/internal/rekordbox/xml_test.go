package rekordbox

import (
	"strings"
	"testing"
)

const twoTrackXML = `<?xml version="1.0" encoding="UTF-8"?>
<DJ_PLAYLISTS Version="1.0.0">
  <PRODUCT Name="rekordbox" Version="7.2.18" Company="AlphaTheta"/>
  <COLLECTION Entries="2">
    <TRACK TrackID="253822049" Name="Bonga (Original Mix)" Artist="DJ Pantelis"
           Genre="Other" AverageBpm="108.00" Tonality="11A" Rating="0"
           PlayCount="7" TotalTime="218" Year="0" DateAdded="2016-02-19"
           Comments="5" Location="file://localhost/Users/aron/DJ/music/Easy/a.mp3"/>
    <TRACK TrackID="122796268" Name="Don&apos;t Stop The Music" Artist="Rihanna"
           Genre="Pop" AverageBpm="122.70" Tonality="11A" Rating="3"
           PlayCount="3" TotalTime="267" Year="2008" DateAdded="2017-09-08"
           Comments="11A - 7" Location="file://localhost/Users/aron/DJ/music/Pop/b.mp3">
      <TEMPO Inizio="0.330" Bpm="122.70" Metro="4/4" Battito="1"/>
    </TRACK>
  </COLLECTION>
  <PLAYLISTS>
    <NODE Type="0" Name="ROOT" Count="25"/>
  </PLAYLISTS>
</DJ_PLAYLISTS>`

func TestParseCollection(t *testing.T) {
	c, err := Parse(strings.NewReader(twoTrackXML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !c.HasCollection {
		t.Error("HasCollection = false, want true")
	}
	if !c.HasPlaylists {
		t.Error("HasPlaylists = false, want true")
	}
	if c.DeclaredEntries != 2 {
		t.Errorf("DeclaredEntries = %d, want 2", c.DeclaredEntries)
	}
	if len(c.Tracks) != 2 {
		t.Fatalf("len(Tracks) = %d, want 2", len(c.Tracks))
	}
	if c.PlaylistCount != 25 {
		t.Errorf("PlaylistCount = %d, want 25", c.PlaylistCount)
	}
	if c.ProductVersion != "7.2.18" {
		t.Errorf("ProductVersion = %q, want %q", c.ProductVersion, "7.2.18")
	}

	// The XML decoder must resolve &apos; for us.
	want := "Don't Stop The Music"
	if c.Tracks[1].Name != want {
		t.Errorf("Tracks[1].Name = %q, want %q", c.Tracks[1].Name, want)
	}
	if c.Tracks[1].Rating != 3 {
		t.Errorf("Tracks[1].Rating = %d, want 3", c.Tracks[1].Rating)
	}
	if c.Tracks[0].Genre != "Other" {
		t.Errorf("Tracks[0].Genre = %q, want %q", c.Tracks[0].Genre, "Other")
	}
	if c.Tracks[0].AverageBpm != "108.00" {
		t.Errorf("Tracks[0].AverageBpm = %q, want %q", c.Tracks[0].AverageBpm, "108.00")
	}
}

// A declared count that disagrees with the number of TRACK elements is the
// signature of a truncated export, which is the failure that deletes a library.
func TestParseRecordsDeclaredEntriesEvenWhenWrong(t *testing.T) {
	x := strings.Replace(twoTrackXML, `Entries="2"`, `Entries="8617"`, 1)
	c, err := Parse(strings.NewReader(x))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.DeclaredEntries != 8617 {
		t.Errorf("DeclaredEntries = %d, want 8617", c.DeclaredEntries)
	}
	if len(c.Tracks) != 2 {
		t.Errorf("len(Tracks) = %d, want 2", len(c.Tracks))
	}
}

func TestParseMissingSections(t *testing.T) {
	t.Run("no collection", func(t *testing.T) {
		c, err := Parse(strings.NewReader(
			`<DJ_PLAYLISTS><PLAYLISTS><NODE Name="ROOT" Count="0"/></PLAYLISTS></DJ_PLAYLISTS>`))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if c.HasCollection {
			t.Error("HasCollection = true, want false")
		}
		if len(c.Tracks) != 0 {
			t.Errorf("len(Tracks) = %d, want 0", len(c.Tracks))
		}
	})

	t.Run("no playlists", func(t *testing.T) {
		c, err := Parse(strings.NewReader(
			`<DJ_PLAYLISTS><COLLECTION Entries="0"></COLLECTION></DJ_PLAYLISTS>`))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if c.HasPlaylists {
			t.Error("HasPlaylists = true, want false")
		}
		if !c.HasCollection {
			t.Error("HasCollection = false, want true")
		}
	})
}

func TestParseMalformed(t *testing.T) {
	if _, err := Parse(strings.NewReader(`<DJ_PLAYLISTS><COLLECTION`)); err == nil {
		t.Fatal("Parse succeeded on malformed XML, want error")
	}
}
