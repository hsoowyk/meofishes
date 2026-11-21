# Command line

Control the open app (like a media remote):

```
meofishes now [--json]                what is playing and the lyric line being sung
meofishes resume | pause | toggle | stop | next | prev
meofishes play "query"                search every source and play the first result
meofishes seek 1:30 | 90 | +10 | -10
meofishes volume [50 | +5 | -5]
meofishes mute | like | unlike | shuffle | repeat [off|all|one]
meofishes lyrics                      the lyrics of the song playing
meofishes queue | show | fullscreen | quit
meofishes sleep [minutes | end | off] sleep timer (fades out, then pauses)
meofishes speed [0.5 … 2]             playback speed (pitch kept)
```

They talk to the app through `$XDG_RUNTIME_DIR/meofishes.sock` (exit code 3
when meofishes is not open). Opening meofishes again just brings the window
back.

Installing and tools:

```
meofishes                             open the app
meofishes lyrics "Artist" "Title" [s] test the lyrics chain (MEO_WORDS=1 prints word timings)
meofishes search <plugin> <query>     run a plugin's search and resolve the first result
meofishes check <plugin folder>       validate a plugin.json
meofishes css <theme folder>          print the compiled Qt stylesheet of a theme
meofishes modules                     list the Go modules plugins can request
meofishes types                       list the plugin types and the capabilities they allow
meofishes setup                       install the default themes and plugins (used by install.sh)
meofishes update [--check|--force]    install the latest build (the CI's "nightly" release)
meofishes uninstall [--purge]         remove meofishes (--purge: settings, library and caches too)
meofishes version                     print the version
```

All commands use your real configuration and plugins.

`playerctl` works too (MPRIS): `playerctl -p meofishes play-pause`,
`playerctl -p meofishes metadata`.
