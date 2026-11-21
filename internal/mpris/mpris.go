// Package mpris publishes the player on D-Bus (MPRIS 2), so media keys, the
// desktop's media widget (KDE, GNOME), the lock screen and playerctl can see
// and control meofishes. It also sends desktop notifications.
package mpris

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	busName    = "org.mpris.MediaPlayer2.meofishes"
	objPath    = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	rootIface  = "org.mpris.MediaPlayer2"
	playerIfac = "org.mpris.MediaPlayer2.Player"
)

// Player is what MPRIS clients control. Calls arrive on D-Bus goroutines:
// implementations must hop to the UI thread themselves.
type Player interface {
	PlayPause()
	Play()
	Pause()
	Stop()
	Next()
	Previous()
	SeekBy(offsetMs int64)
	SeekTo(positionMs int64)
	SetVolume(v float64) // 0..1
	SetShuffle(on bool)
	SetLoop(mode string) // None | Track | Playlist
	SetRate(r float64)
	Raise()
	Quit()
}

// Track is the playing song.
type Track struct {
	Key, Title, Artist, Album, ArtURL string
	LengthMs                          int64
}

// State is the current playback state.
type State struct {
	Track   Track
	Status  string // Playing | Paused | Stopped
	Volume  float64
	Shuffle bool
	Loop    string
	Rate    float64
}

// Server is the published player.
type Server struct {
	conn  *dbus.Conn
	props *prop.Properties
	p     Player
	mu    sync.Mutex
	last  State
	notID uint32
}

type root struct{ s *Server }

func (r root) Raise() *dbus.Error { r.s.p.Raise(); return nil }
func (r root) Quit() *dbus.Error  { r.s.p.Quit(); return nil }

// playerMethods is the org.mpris.MediaPlayer2.Player method table.
func (s *Server) playerMethods() map[string]any {
	return map[string]any{
		"Next":      func() *dbus.Error { s.p.Next(); return nil },
		"Previous":  func() *dbus.Error { s.p.Previous(); return nil },
		"Pause":     func() *dbus.Error { s.p.Pause(); return nil },
		"PlayPause": func() *dbus.Error { s.p.PlayPause(); return nil },
		"Stop":      func() *dbus.Error { s.p.Stop(); return nil },
		"Play":      func() *dbus.Error { s.p.Play(); return nil },
		"Seek":      func(offsetUs int64) *dbus.Error { s.p.SeekBy(offsetUs / 1000); return nil },
		"SetPosition": func(id dbus.ObjectPath, posUs int64) *dbus.Error {
			s.p.SeekTo(posUs / 1000)
			return nil
		},
		"OpenUri": func(string) *dbus.Error { return nil },
	}
}

var playerIntrospect = []introspect.Method{
	{Name: "Next"}, {Name: "Previous"}, {Name: "Pause"}, {Name: "PlayPause"}, {Name: "Stop"}, {Name: "Play"},
	{Name: "Seek", Args: []introspect.Arg{{Name: "Offset", Type: "x", Direction: "in"}}},
	{Name: "SetPosition", Args: []introspect.Arg{{Name: "TrackId", Type: "o", Direction: "in"}, {Name: "Position", Type: "x", Direction: "in"}}},
	{Name: "OpenUri", Args: []introspect.Arg{{Name: "Uri", Type: "s", Direction: "in"}}},
}

// Start connects to the session bus and publishes the player.
func Start(p Player) (*Server, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	s := &Server{conn: conn, p: p}
	if err := conn.Export(root{s}, objPath, rootIface); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.ExportMethodTable(s.playerMethods(), objPath, playerIfac); err != nil {
		conn.Close()
		return nil, err
	}
	ro := func(v any) *prop.Prop { return &prop.Prop{Value: v, Emit: prop.EmitTrue} }
	rw := func(v any, cb func(*prop.Change) *dbus.Error) *prop.Prop {
		return &prop.Prop{Value: v, Writable: true, Emit: prop.EmitTrue, Callback: cb}
	}
	props, err := prop.Export(conn, objPath, prop.Map{
		rootIface: {
			"CanQuit": ro(true), "CanRaise": ro(true), "HasTrackList": ro(false),
			"Identity": ro("meofishes"), "DesktopEntry": ro("meofishes"),
			"SupportedUriSchemes": ro([]string{}), "SupportedMimeTypes": ro([]string{}),
		},
		playerIfac: {
			"PlaybackStatus": ro("Stopped"),
			"LoopStatus": rw("None", func(c *prop.Change) *dbus.Error {
				if v, ok := c.Value.(string); ok {
					p.SetLoop(v)
				}
				return nil
			}),
			"Rate": rw(1.0, func(c *prop.Change) *dbus.Error {
				if v, ok := c.Value.(float64); ok {
					p.SetRate(v)
				}
				return nil
			}),
			"Shuffle": rw(false, func(c *prop.Change) *dbus.Error {
				if v, ok := c.Value.(bool); ok {
					p.SetShuffle(v)
				}
				return nil
			}),
			"Metadata": ro(map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack"))}),
			"Volume": rw(1.0, func(c *prop.Change) *dbus.Error {
				if v, ok := c.Value.(float64); ok {
					p.SetVolume(v)
				}
				return nil
			}),
			"Position":      {Value: int64(0), Emit: prop.EmitFalse},
			"MinimumRate":   ro(0.25),
			"MaximumRate":   ro(4.0),
			"CanGoNext":     ro(true),
			"CanGoPrevious": ro(true),
			"CanPlay":       ro(true),
			"CanPause":      ro(true),
			"CanSeek":       ro(true),
			"CanControl":    ro(true),
		},
	})
	if err != nil {
		conn.Close()
		return nil, err
	}
	s.props = props
	node := &introspect.Node{Name: string(objPath), Interfaces: []introspect.Interface{
		introspect.IntrospectData,
		prop.IntrospectData,
		{Name: rootIface, Methods: introspect.Methods(root{s}), Properties: props.Introspection(rootIface)},
		{Name: playerIfac, Methods: playerIntrospect, Properties: props.Introspection(playerIfac),
			Signals: []introspect.Signal{{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}}}},
	}}
	_ = conn.Export(introspect.NewIntrospectable(node), objPath, "org.freedesktop.DBus.Introspectable")
	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		if err == nil {
			err = errors.New("the MPRIS name is taken (another meofishes?)")
		}
		return nil, err
	}
	return s, nil
}

func trackID(key string) dbus.ObjectPath {
	h := sha1.Sum([]byte(key))
	return dbus.ObjectPath("/org/meofishes/track/t" + hex.EncodeToString(h[:8]))
}

// Update publishes the state (only what changed is signalled).
func (s *Server) Update(st State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.Track != s.last.Track {
		md := map[string]dbus.Variant{
			"mpris:trackid": dbus.MakeVariant(trackID(st.Track.Key)),
			"xesam:title":   dbus.MakeVariant(st.Track.Title),
			"xesam:artist":  dbus.MakeVariant([]string{st.Track.Artist}),
			"xesam:album":   dbus.MakeVariant(st.Track.Album),
		}
		if st.Track.LengthMs > 0 {
			md["mpris:length"] = dbus.MakeVariant(st.Track.LengthMs * 1000)
		}
		if st.Track.ArtURL != "" {
			md["mpris:artUrl"] = dbus.MakeVariant(st.Track.ArtURL)
		}
		s.props.SetMust(playerIfac, "Metadata", md)
	}
	if st.Status != s.last.Status && st.Status != "" {
		s.props.SetMust(playerIfac, "PlaybackStatus", st.Status)
	}
	if st.Volume != s.last.Volume {
		s.props.SetMust(playerIfac, "Volume", st.Volume)
	}
	if st.Shuffle != s.last.Shuffle {
		s.props.SetMust(playerIfac, "Shuffle", st.Shuffle)
	}
	if st.Loop != s.last.Loop && st.Loop != "" {
		s.props.SetMust(playerIfac, "LoopStatus", st.Loop)
	}
	if st.Rate != s.last.Rate && st.Rate > 0 {
		s.props.SetMust(playerIfac, "Rate", st.Rate)
	}
	s.last = st
}

// SetPosition updates the position clients read (no signal, per the spec).
func (s *Server) SetPosition(ms int64) {
	s.props.SetMust(playerIfac, "Position", ms*1000)
}

// Seeked tells clients the position jumped.
func (s *Server) Seeked(ms int64) {
	s.SetPosition(ms)
	_ = s.conn.Emit(objPath, playerIfac+".Seeked", ms*1000)
}

// Notify shows a desktop notification, replacing the previous one.
func (s *Server) Notify(summary, body, icon string) {
	obj := s.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	s.mu.Lock()
	replace := s.notID
	s.mu.Unlock()
	call := obj.Call("org.freedesktop.Notifications.Notify", 0, "meofishes", replace, icon, summary, body,
		[]string{}, map[string]dbus.Variant{"desktop-entry": dbus.MakeVariant("meofishes")}, int32(4000))
	var id uint32
	if call.Err == nil && call.Store(&id) == nil {
		s.mu.Lock()
		s.notID = id
		s.mu.Unlock()
	}
}

// Close unpublishes the player.
func (s *Server) Close() {
	_, _ = s.conn.ReleaseName(busName)
	s.conn.Close()
}
