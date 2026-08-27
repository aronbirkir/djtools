package rekordbox

import "testing"

const (
	macMusic = "/Users/aron/DJ/music"
	winMusic = "e:/music"
)

func TestClassifyLocation(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		// host defaults to HostPOSIX, which is the zero value.
		host Host
		// musicDir defaults to macMusic when empty.
		musicDir string
		wantKind Kind
		wantPath string
	}{
		{
			name:     "local mp3 with escaped spaces",
			raw:      "file://localhost/Users/aron/DJ/music/Easy/11A%20108%20Bonga%20(Original%20Mix)%20-%20Dj%20Pantelis.mp3",
			wantKind: KindLocal,
			wantPath: "/Users/aron/DJ/music/Easy/11A 108 Bonga (Original Mix) - Dj Pantelis.mp3",
		},
		{
			// 28 real paths contain a literal '+'. QueryUnescape would turn it
			// into a space and orphan every one of them.
			name:     "literal plus is preserved",
			raw:      "file://localhost/Users/aron/DJ/music/House/02A%20064%20Put%20Your%20Hands%20Up%20for%20Detroit%20+%20Pump%20Up%20the%20Volume%20(USA%2012%20Mix)%20-%20Fedde%20Le%20Grand%20+%20M.mp3",
			wantKind: KindLocal,
			wantPath: "/Users/aron/DJ/music/House/02A 064 Put Your Hands Up for Detroit + Pump Up the Volume (USA 12 Mix) - Fedde Le Grand + M.mp3",
		},
		{
			// Decoding twice would turn "10%" into a broken escape.
			name:     "escaped percent decodes once",
			raw:      "file://localhost/Users/aron/DJ/music/A2022-07/09A%20108%2010%25%20-%20KAYTRANADA%20feat.%20Kali%20Uchis.mp3",
			wantKind: KindLocal,
			wantPath: "/Users/aron/DJ/music/A2022-07/09A 108 10% - KAYTRANADA feat. Kali Uchis.mp3",
		},
		{
			name:     "escaped apostrophe",
			raw:      "file://localhost/Users/aron/DJ/music/Pop/11A%20123%20Don%27t%20Stop%20The%20Music%20-%20Rihanna.mp3",
			wantKind: KindLocal,
			wantPath: "/Users/aron/DJ/music/Pop/11A 123 Don't Stop The Music - Rihanna.mp3",
		},
		{
			name:     "escaped ampersand",
			raw:      "file://localhost/Users/aron/DJ/music/Pop/11B%20121%20Raspberry%20Beret%20-%20Prince%20%26%20The%20Revolution.mp3",
			wantKind: KindLocal,
			wantPath: "/Users/aron/DJ/music/Pop/11B 121 Raspberry Beret - Prince & The Revolution.mp3",
		},
		{
			// High-byte escapes are written in lowercase hex. This is a
			// combining diaeresis, "Zoe" + U+0308 -- the only NFD-spelled path
			// in the entire export, and it happens to be a stale entry.
			name:     "lowercase high-byte escapes decode to utf-8",
			raw:      "file://localhostE:/music/Dance/05A%20140%20You%20Got%20To%20Go%20(Seven%20Lions%20Dubstep%20Remix)%20-%20Above%20%26%20Beyond%20Feat.%20Zoe%cc%88%20Johnston.mp3",
			wantKind: KindStale,
			wantPath: "E:/music/Dance/05A 140 You Got To Go (Seven Lions Dubstep Remix) - Above & Beyond Feat. Zoe\u0308 Johnston.mp3",
		},
		{
			name:     "lowercase windows drive is stale",
			raw:      "file://localhoste:/music/Funky/09A%20126%20Comin%20(Original%20Mix)%20-%20Antoine%20Clamaran,%20Agua%20Sin%20Gas.mp3",
			wantKind: KindStale,
			wantPath: "e:/music/Funky/09A 126 Comin (Original Mix) - Antoine Clamaran, Agua Sin Gas.mp3",
		},
		{
			name:     "uppercase windows drive is stale",
			raw:      "file://localhostE:/music/Funky/9A%20120%20Get%20Down,%20JB!%20(Original%20Mix)%20-%20Me%20%26%20My%20Toothbrush.mp3",
			wantKind: KindStale,
			wantPath: "E:/music/Funky/9A 120 Get Down, JB! (Original Mix) - Me & My Toothbrush.mp3",
		},
		{
			name:     "windows C drive is stale",
			raw:      "file://localhostC:/Users/aron/Music/PioneerDJ/Demo%20Tracks/Demo%20Track%201.mp3",
			wantKind: KindStale,
			wantPath: "C:/Users/aron/Music/PioneerDJ/Demo Tracks/Demo Track 1.mp3",
		},
		{
			name:     "tidal stream is not a file",
			raw:      "file://localhosttidal:tracks:62303032",
			wantKind: KindNonFile,
			wantPath: "",
		},
		{
			name:     "sampler wav outside music dir is foreign",
			raw:      "file://localhost/Users/aron/Music/rekordbox/Sampler/GROOVE%20CIRCUIT/PRESET/4-Floor%20Breaks%20Kit/House1.wav",
			wantKind: KindForeign,
			wantPath: "/Users/aron/Music/rekordbox/Sampler/GROOVE CIRCUIT/PRESET/4-Floor Breaks Kit/House1.wav",
		},
		{
			name:     "missing file scheme is not a file",
			raw:      "/Users/aron/DJ/music/Pop/whatever.mp3",
			wantKind: KindNonFile,
			wantPath: "",
		},
		{
			// A directory whose name merely starts with the music dir must not
			// be treated as inside it.
			name:     "sibling directory prefix is foreign",
			raw:      "file://localhost/Users/aron/DJ/music-archive/old.mp3",
			wantKind: KindForeign,
			wantPath: "/Users/aron/DJ/music-archive/old.mp3",
		},
		{
			name:     "invalid escape is not a file",
			raw:      "file://localhost/Users/aron/DJ/music/Pop/bad%zz.mp3",
			wantKind: KindNonFile,
			wantPath: "",
		},

		// The same collection, judged from the retired Windows machine. Every
		// classification flips: what is stale on the Mac is the live library
		// there, and vice versa. This is the only coverage the Windows branch
		// can get, since that machine no longer exists to test on.
		{
			name:     "on windows a drive path under music is local",
			raw:      "file://localhoste:/music/Funky/09A%20126%20Comin%20(Original%20Mix)%20-%20Antoine%20Clamaran,%20Agua%20Sin%20Gas.mp3",
			host:     HostWindows,
			musicDir: winMusic,
			wantKind: KindLocal,
			wantPath: "e:/music/Funky/09A 126 Comin (Original Mix) - Antoine Clamaran, Agua Sin Gas.mp3",
		},
		{
			name:     "on windows a drive path outside music is foreign",
			raw:      "file://localhostC:/Users/aron/Music/PioneerDJ/Demo%20Tracks/Demo%20Track%201.mp3",
			host:     HostWindows,
			musicDir: winMusic,
			wantKind: KindForeign,
			wantPath: "C:/Users/aron/Music/PioneerDJ/Demo Tracks/Demo Track 1.mp3",
		},
		{
			name:     "on windows a posix path is stale",
			raw:      "file://localhost/Users/aron/DJ/music/Easy/11A%20108%20Bonga%20(Original%20Mix)%20-%20Dj%20Pantelis.mp3",
			host:     HostWindows,
			musicDir: winMusic,
			wantKind: KindStale,
			wantPath: "/Users/aron/DJ/music/Easy/11A 108 Bonga (Original Mix) - Dj Pantelis.mp3",
		},
		{
			name:     "on windows tidal is still not a file",
			raw:      "file://localhosttidal:tracks:62303032",
			host:     HostWindows,
			musicDir: winMusic,
			wantKind: KindNonFile,
			wantPath: "",
		},
		{
			// NTFS and APFS are both case-insensitive, so a differently cased
			// music dir must still match.
			name:     "music dir comparison ignores case",
			raw:      "file://localhost/Users/aron/DJ/MUSIC/Pop/a.mp3",
			wantKind: KindLocal,
			wantPath: "/Users/aron/DJ/MUSIC/Pop/a.mp3",
		},
		{
			name:     "windows backslashes are equivalent to forward slashes",
			raw:      "file://localhoste:\\music\\Funky\\a.mp3",
			host:     HostWindows,
			musicDir: winMusic,
			wantKind: KindLocal,
			wantPath: "e:\\music\\Funky\\a.mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			music := tt.musicDir
			if music == "" {
				music = macMusic
			}
			got := ClassifyLocation(tt.raw, music, tt.host)
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", got.Kind, tt.wantKind)
			}
			if got.Path != tt.wantPath {
				t.Errorf("Path =\n  %q\nwant\n  %q", got.Path, tt.wantPath)
			}
			if got.Raw != tt.raw {
				t.Errorf("Raw = %q, want %q", got.Raw, tt.raw)
			}
		})
	}
}

func TestKindString(t *testing.T) {
	for kind, want := range map[Kind]string{
		KindLocal:   "local",
		KindStale:   "stale",
		KindForeign: "foreign",
		KindNonFile: "nonfile",
	} {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}
