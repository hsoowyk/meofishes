package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"meofishes/internal/sysproc"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"meofishes/internal/core"
	"meofishes/internal/lyrics"
)

// process runs an external executable and talks newline-delimited JSON over
// its stdin/stdout. Any language works; Go plugins use meofishes/sdk.
//
//	host → plugin  {"id":1,"method":"search","params":{"query":"…"}}
//	plugin → host  {"id":1,"result":{"tracks":[…]}}   or {"id":1,"error":"…"}
//	plugin → host  {"method":"log","params":{"message":"…"}}   (notification)
type process struct {
	m        *Manifest
	dir      string
	host     *Host
	settings func() map[string]string
	env      []string

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	nextID  atomic.Int64
	pending sync.Map // id -> chan rpcResp
	caps    map[string]bool
	dead    error
	// notify receives notifications other than "log" (e.g. "player.end").
	notifyMu sync.Mutex
	notify   func(method string, params json.RawMessage)
}

// OnNotify sets the handler of notifications sent by the plugin.
func (p *process) OnNotify(f func(method string, params json.RawMessage)) {
	p.notifyMu.Lock()
	p.notify = f
	p.notifyMu.Unlock()
}

type rpcReq struct {
	ID     int64  `json:"id,omitempty"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type rpcResp struct {
	ID     int64           `json:"id"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

func newProcess(m *Manifest, dir string, host *Host, settings func() map[string]string, env []string) (*process, error) {
	p := &process{m: m, dir: dir, host: host, settings: settings, env: env}
	if err := p.start(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *process) start() error {
	args := strings.Fields(p.m.Entry)
	if len(args) == 0 {
		return errors.New("empty entry")
	}
	bin := args[0]
	if !filepath.IsAbs(bin) {
		if local := filepath.Join(p.dir, bin); fileExists(local) {
			bin = local
		} else if lp, err := exec.LookPath(bin); err == nil {
			bin = lp
		} else {
			return fmt.Errorf("executable %q not found (was the plugin built?)", args[0])
		}
	}
	cmd := exec.Command(bin, args[1:]...)
	cmd.Dir = p.dir
	cmd.Env = append(os.Environ(), "MEOFISHES_PLUGIN="+p.m.ID, "MEOFISHES_VERSION="+p.host.Version)
	cmd.Env = append(cmd.Env, p.env...)
	sysproc.Child(cmd) // dies with meofishes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd, p.stdin, p.dead = cmd, stdin, nil
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			p.host.logf(p.m.ID, "stderr: %s", sc.Text())
		}
	}()
	go p.readLoop(stdout)
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.dead = fmt.Errorf("plugin process exited: %v", err)
		p.mu.Unlock()
		p.pending.Range(func(k, v any) bool {
			v.(chan rpcResp) <- rpcResp{Error: "plugin process exited"}
			p.pending.Delete(k)
			return true
		})
	}()

	var init struct {
		Capabilities []string `json:"capabilities"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = p.rpc(ctx, "initialize", map[string]any{
		"plugin": p.m.ID, "version": p.host.Version, "userAgent": p.host.UserAgent, "quality": p.host.Quality,
		"settings": p.settings(), "dataDir": filepath.Join(p.host.DataDir, p.m.ID),
	}, &init)
	if err != nil {
		_ = p.Close()
		return fmt.Errorf("initialize: %w", err)
	}
	p.caps = map[string]bool{}
	for _, c := range init.Capabilities {
		p.caps[c] = true
	}
	return nil
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

func (p *process) readLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	for sc.Scan() {
		var msg rpcResp
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			p.host.logf(p.m.ID, "bad message: %s", sc.Text())
			continue
		}
		if msg.Method == "log" {
			var lp struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(msg.Params, &lp)
			p.host.logf(p.m.ID, "%s", lp.Message)
			continue
		}
		if msg.Method != "" && msg.ID == 0 {
			p.notifyMu.Lock()
			f := p.notify
			p.notifyMu.Unlock()
			if f != nil {
				f(msg.Method, msg.Params)
			}
			continue
		}
		if ch, ok := p.pending.LoadAndDelete(msg.ID); ok {
			ch.(chan rpcResp) <- msg
		}
	}
}

func (p *process) send(r rpcReq) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dead != nil {
		return p.dead
	}
	_, err = p.stdin.Write(append(data, '\n'))
	return err
}

func (p *process) rpc(ctx context.Context, method string, params, out any) error {
	id := p.nextID.Add(1)
	ch := make(chan rpcResp, 1)
	p.pending.Store(id, ch)
	if err := p.send(rpcReq{ID: id, Method: method, Params: params}); err != nil {
		p.pending.Delete(id)
		return err
	}
	select {
	case <-ctx.Done():
		p.pending.Delete(id)
		return ctx.Err()
	case r := <-ch:
		if r.Error != "" {
			if r.Error == "not found" {
				return lyrics.ErrNotFound
			}
			return errors.New(r.Error)
		}
		if out != nil && len(r.Result) > 0 && string(r.Result) != "null" {
			return json.Unmarshal(r.Result, out)
		}
		return nil
	}
}

// Caps are the capabilities the process announced in "initialize".
// Older plugins announce "source"/"lyrics"/"hook"; they are mapped.
func (p *process) Caps() map[string]bool {
	c := map[string]bool{}
	for k := range p.caps {
		switch k {
		case "source":
			c[CapSearch], c[CapStream] = true, true
		case "hook", "event":
			c[CapEvents] = true
		default:
			c[k] = true
		}
	}
	if c[CapSearch] && !c[CapStream] {
		c[CapStream] = true
	}
	if c["resolve"] {
		c[CapStream], c[CapMatch] = true, true
	}
	return c
}

// Call sends any method to the plugin (player plugins use player.*).
func (p *process) Call(ctx context.Context, method string, params, out any) error {
	return p.rpc(ctx, method, params, out)
}

func (p *process) Recommend(ctx context.Context, seed core.Track) (*core.SearchResult, error) {
	var r core.SearchResult
	if err := p.rpc(ctx, "recommend", map[string]any{"track": seed}, &r); err != nil {
		return nil, err
	}
	for i := range r.Tracks {
		r.Tracks[i].Source = p.m.ID
	}
	return &r, nil
}

func (p *process) Playlists(ctx context.Context) ([]core.Collection, error) {
	var out []core.Collection
	err := p.rpc(ctx, "playlists", nil, &out)
	return out, err
}

func (p *process) Artwork(ctx context.Context, q lyrics.Query) (string, error) {
	var r struct {
		URL string `json:"url"`
	}
	err := p.rpc(ctx, "artwork", map[string]any{"query": map[string]any{
		"title": q.Title, "artist": q.Artist, "album": q.Album, "durationMs": q.DurationMs, "isrc": q.ISRC, "videoId": q.VideoID,
	}}, &r)
	if err == nil && r.URL == "" {
		err = lyrics.ErrNotFound
	}
	return r.URL, err
}

func (p *process) Search(ctx context.Context, q string) (*core.SearchResult, error) {
	var r core.SearchResult
	if err := p.rpc(ctx, "search", map[string]any{"query": q}, &r); err != nil {
		return nil, err
	}
	for i := range r.Tracks {
		r.Tracks[i].Source = p.m.ID
	}
	return &r, nil
}

func (p *process) Home(ctx context.Context) (*core.SearchResult, error) {
	var r core.SearchResult
	if !p.caps["home"] {
		return &r, nil
	}
	if err := p.rpc(ctx, "home", nil, &r); err != nil {
		return nil, err
	}
	for i := range r.Tracks {
		r.Tracks[i].Source = p.m.ID
	}
	return &r, nil
}

func (p *process) Open(ctx context.Context, c core.Collection) (*core.Collection, error) {
	if !p.caps["open"] {
		return &c, nil
	}
	var out core.Collection
	if err := p.rpc(ctx, "open", map[string]any{"collection": c}, &out); err != nil {
		return nil, err
	}
	for i := range out.Tracks {
		out.Tracks[i].Source = p.m.ID
	}
	return &out, nil
}

func (p *process) Resolve(ctx context.Context, t core.Track) (*Stream, error) {
	var st Stream
	if err := p.rpc(ctx, "resolve", map[string]any{"track": t}, &st); err != nil {
		return nil, err
	}
	if st.URL == "" {
		st.URL = t.StreamURL
		st.Page = !looksLikeMedia(st.URL)
	}
	return &st, nil
}

func (p *process) Lyrics(ctx context.Context, q lyrics.Query) (*core.Lyrics, error) {
	var raw map[string]any
	if err := p.rpc(ctx, "lyrics", map[string]any{"query": map[string]any{
		"title": q.Title, "artist": q.Artist, "album": q.Album, "durationMs": q.DurationMs, "isrc": q.ISRC, "videoId": q.VideoID,
	}}, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, lyrics.ErrNotFound
	}
	if content, ok := raw["content"].(string); ok {
		l, err := lyrics.Parse(toString(raw["format"]), content)
		if err != nil {
			return nil, err
		}
		l.Credit = toString(raw["credit"])
		return l, nil
	}
	var l core.Lyrics
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	l.Normalize()
	return &l, nil
}

func (p *process) Event(name string, payload any) {
	_ = p.send(rpcReq{Method: "event", Params: map[string]any{"name": name, "payload": payload}})
}

func (p *process) SettingsChanged() {
	_ = p.send(rpcReq{Method: "settings", Params: p.settings()})
}

func (p *process) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil && p.dead == nil {
		proc := p.cmd.Process
		_ = proc.Signal(os.Interrupt)
		go func() {
			time.Sleep(2 * time.Second)
			p.mu.Lock()
			alive := p.dead == nil
			p.mu.Unlock()
			if alive {
				_ = proc.Kill()
			}
		}()
	}
	return nil
}
