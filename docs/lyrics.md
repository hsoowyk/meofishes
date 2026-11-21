# Lyrics

## Where they come from

Lyrics plugins (`fnt` with the `lyrics` capability) are asked in the order of
**Settings → Plugin priority → Lyrics**. All are queried in parallel; the
first one (by priority) with **word sync** wins, otherwise the first with line
sync, otherwise plain text. Results are cached in `~/.cache/meofishes/lyrics`
(the ↻ button in the fullscreen player searches again without the cache).

The built-in **Meo Lyrics** chains its own providers (order and keys in
**Settings → Lyrics → Lyrics providers**):

| provider | needs | notes |
|---|---|---|
| Lyprio | base URL + HMAC signing key from its maintainers | off until configured |
| BetterLyrics | nothing | word sync (TTML) |
| lrc.red | the app name, a link and a contact e-mail in the User-Agent | sent automatically |
| Unison | name and link in the User-Agent; personal/non-commercial use | community lyrics |
| Paxsenix | nothing for lyrics.paxsenix.org | Apple Music word sync |
| Kugou | nothing | word sync (KRC) |

meofishes identifies itself to these services as
`meofishes/<version> (+https://git.5418.org/kywoosh/meofishes; mail@kywoosh.xyz)`.
Users never have to enter contact details.

Other lyrics plugins in the catalog: **LRCLIB** (line sync).

## Formats

| format | sync |
|---|---|
| TTML (Apple) | words/syllables, background vocals (`x-bg`), duet agents |
| LRC | lines; enhanced LRC `<mm:ss.xx>` gives words |
| KRC (Kugou) | words (encrypted, decoded by meofishes) |
| plain | none |

## How they are shown

- **Word-synced** lyrics light up word by word: `-meo-word-animation`
  `fill` (gradient sweep), `fade`, `bounce`, `glow` or `none`.
- **Line-synced** lyrics light up as a **whole line** (never word by word):
  `-meo-line-animation: fade | none`.
- Background vocals are drawn smaller under the line; duets use the opposite
  side (`-meo-opposite-align`).
- Instrumental gaps longer than 5 s show three breathing dots.
- Click a line to jump there.
- The line being sung also appears in the left panel (`#CurrentLyric`) and
  swipes to the next one.
- No lyrics? The fullscreen player shows a frequency visualizer instead
  ([streaming.md](streaming.md#visualizer)).

All of it is adjustable in **Settings → Lyrics** and in CSS
([themes/properties.md](themes/properties.md#lyrics--lyricsview-fulllyrics)).
**Lyrics offset** moves the lyrics earlier or later when a provider is off.

## Testing

```sh
meofishes lyrics "The Weeknd" "Blinding Lights" 200
MEO_WORDS=1 meofishes lyrics "Artist" "Title"     # print word timings
```

Developer mode → **Lyrics** shows every provider attempt of the last search
and the timings of the current line.
