package player

// AddVideo adds a separate video stream to what is playing, without
// stopping the audio: mpv keeps the current audio and syncs the video to it
// by timestamp (same YouTube video, so the clocks match).
func (m *MPV) AddVideo(url string) error {
	_ = m.command("set_property", "vid", "auto")
	return m.command("video-add", url, "select")
}

// RemoveVideo turns the video off and drops the external video tracks; the
// audio keeps playing.
func (m *MPV) RemoveVideo() error {
	err := m.command("set_property", "vid", "no")
	if v, e := m.Get("track-list"); e == nil {
		list, _ := v.([]any)
		for _, x := range list {
			if t, ok := x.(map[string]any); ok && t["type"] == "video" && t["external"] == true {
				if id, ok := t["id"].(float64); ok {
					_ = m.command("video-remove", int(id))
				}
			}
		}
	}
	return err
}

// VideoPlaying reports that video frames are really being shown: a video
// output is configured, the video has a size and the clock is running.
func (m *MPV) VideoPlaying() bool {
	if v, err := m.Get("vo-configured"); err != nil || v != true {
		return false
	}
	w, err := m.Get("video-params/w")
	if err != nil {
		return false
	}
	if f, ok := w.(float64); !ok || f <= 0 {
		return false
	}
	if p, err := m.Get("paused-for-cache"); err == nil && p == true {
		return false
	}
	return true
}
