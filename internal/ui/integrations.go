package ui

import (
	"context"
	"strings"
	"time"

	"meofishes/internal/config"
	"meofishes/internal/discord"
	"meofishes/internal/sponsorblock"
)

// ---------------------------------------------------------------- SponsorBlock

// sbFetch looks up the parts of the new song's YouTube video to skip.
func (u *UI) sbFetch() {
	u.sbSegs, u.sbLast = nil, 0
	cur := u.current()
	if !u.cfg.Prefs.SponsorBlock || cur == nil {
		return
	}
	page := ""
	if u.stream != nil {
		page = u.stream.PageURL
	}
	id := ytID(*cur, page)
	if id == "" {
		return
	}
	if u.sb == nil {
		u.sb = sponsorblock.New(config.UserAgent)
	}
	gen := u.gen
	var segs []sponsorblock.Segment
	cats := u.sbCategories()
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var err error
		segs, err = u.sb.Segments(ctx, id, cats)
		return err
	}, func(err error) {
		if err != nil {
			u.core.Log("sponsorblock", "%s: %v", id, err)
			return
		}
		if gen == u.gen {
			u.sbSegs = segs
		}
	})
}

func (u *UI) sbCategories() []string {
	if c := strings.TrimSpace(u.cfg.Prefs.SponsorCats); c != "" {
		return strings.Split(c, ",")
	}
	return []string{"music_offtopic", "sponsor", "selfpromo", "interaction"}
}

// sbTick skips a segment the position is in (called by the 200 ms timer).
func (u *UI) sbTick(pos int64) {
	if len(u.sbSegs) == 0 || (u.lt.room != "" && !u.lt.host) {
		return
	}
	to, cat := sponsorblock.SkipTo(u.sbSegs, pos)
	if to == 0 || to == u.sbLast {
		return
	}
	u.sbLast = to
	if d := u.duration(); d > 0 && to >= d-1000 {
		u.Next(false) // the rest of the video is not music
		return
	}
	u.SeekTo(to)
	u.toast(tr("Skipped: {part}", "part", sbName(cat)))
}

func sbName(cat string) string {
	switch cat {
	case "music_offtopic":
		return tr("part without music")
	case "intro":
		return tr("intro")
	case "outro":
		return tr("outro")
	case "sponsor":
		return tr("sponsor")
	case "selfpromo":
		return tr("self-promotion")
	case "interaction":
		return tr("“subscribe” reminder")
	case "preview":
		return tr("preview")
	}
	return cat
}

// ---------------------------------------------------------------- Discord

// discordSync shows the current song on Discord (in a background goroutine:
// the socket may block).
func (u *UI) discordSync() {
	if !u.cfg.Prefs.Discord || strings.TrimSpace(u.cfg.Prefs.DiscordAppID) == "" {
		u.discordStop()
		return
	}
	if u.discord == nil {
		u.discord = &discord.Client{AppID: strings.TrimSpace(u.cfg.Prefs.DiscordAppID)}
		u.discordCh = make(chan *discord.Presence, 1)
		d, ch := u.discord, u.discordCh
		go func() {
			last := ""
			for p := range ch {
				msg := ""
				if err := d.Set(p); err != nil {
					msg = err.Error()
				}
				if msg != last && msg != "" {
					u.core.Log("discord", "%s", msg)
				}
				last = msg
			}
			d.Close()
		}()
	}
	var p *discord.Presence
	st := u.state()
	if cur := u.current(); cur != nil && !st.Idle {
		p = &discord.Presence{Title: cur.Title, Artist: cur.Artist, Album: cur.Album, Paused: st.Paused}
		if strings.HasPrefix(cur.Cover, "https://") && len(cur.Cover) < 256 {
			p.CoverURL = cur.Cover
		}
		if !st.Paused {
			now := time.Now().UnixMilli()
			p.StartMs = now - st.PositionMs
			if d := u.duration(); d > 0 {
				p.EndMs = p.StartMs + d
			}
		}
	}
	select { // keep only the newest presence
	case <-u.discordCh:
	default:
	}
	u.discordCh <- p
}

func (u *UI) discordStop() {
	if u.discordCh != nil {
		close(u.discordCh)
		u.discordCh = nil
	}
	u.discord = nil
}
