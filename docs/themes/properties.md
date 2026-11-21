# `-meo-*` properties

Qt stylesheets cannot describe animations or the widgets meofishes paints
itself, so these extra properties do. Put them in any rule; the most specific
selector wins (`Kind#name`, then `#name`, then `[role]`, then `Kind`).
Default values are in parentheses.

## Every widget: enter animation

Played when a widget appears (panels at start, rows of a list, cards…).

| property | values |
|---|---|
| `-meo-enter` | `none` · `fade` · `slide-up` · `slide-down` · `slide-left` · `slide-right` (none) |
| `-meo-enter-duration` | duration (400ms) |
| `-meo-enter-delay` | duration (0) |
| `-meo-enter-stagger` | extra delay per item of a list (0) |
| `-meo-enter-easing` | easing (out-cubic) |
| `-meo-enter-distance` | length (24px) |

## Page transitions — `#Pages`

`-meo-transition` (`fade`, `slide-up`, `slide-left`, `slide-right`, `zoom`,
`none`), `-meo-transition-duration` (300ms), `-meo-transition-easing`
(out-cubic), `-meo-transition-distance` (24px).

## Animated backgrounds of buttons, rows and cards

Icon buttons (`[role="control"]`, `[role="window"]`, `[role="nav"]`),
`#TrackRow`, `#Card`, `#LibraryItem`, `#Logo`:

| property | |
|---|---|
| `-meo-bg`, `-meo-hover-bg`, `-meo-active-bg` | background in each state (animated) |
| `-meo-icon-color`, `-meo-hover-icon-color`, `-meo-active-icon-color` | icon color in each state |
| `-meo-icon-size` | icon size (18px; 20px in nav) |
| `-meo-hover-duration`, `-meo-hover-easing` | state animation (160ms, out-quad) |
| `-meo-radius` | corner radius |
| `-meo-press-scale` | scale while pressed (1) |

Nav icons (`[role="nav"]`) also: `-meo-size` (46px), `-meo-indicator-color`,
`-meo-indicator-width` (3px; 0 hides the active bar).

## Window

| selector | properties |
|---|---|
| `#Root`, `#FullPlayer` (backdrops) | `-meo-backdrop-from`, `-meo-backdrop-to`, `-meo-backdrop-angle` (160), `-meo-backdrop-dynamic` (take the cover color), `-meo-backdrop-dynamic-strength` (0.35), `-meo-backdrop-dynamic-duration` (1200ms), `-meo-backdrop-glow` (color of the radial light), `-meo-backdrop-glow-x`/`-y` (0..1), `-meo-backdrop-glow-radius` (0.8), `-meo-backdrop-glow-pulse` (period; 0 = still) |
| `#FullPlayer` only | `-meo-backdrop-image-opacity` (0.55), `-meo-backdrop-image-blur` (18, smaller = blurrier), `-meo-backdrop-image-shade` (color over the image), `-meo-open-duration`, `-meo-open-easing`, `-meo-close-duration`, `-meo-close-easing` |
| `#TopBar` | `-meo-height` (52px) |
| `#Logo` | `-meo-logo-color` (default: the theme text color), `-meo-logo-size` (26px) |
| `#LogoMenu` | `-meo-open-duration` (260ms), `-meo-open-easing` |
| `#LeftPanel` | `-meo-width` (290px) |
| `#Toast` | `-meo-show-duration` (240ms), `-meo-hide-after` (3000ms) |
| `#Tour` | `-meo-dim-color`, `-meo-spot-color`, `-meo-spot-radius` |
| `#TrackRow` | `-meo-drop-color` (drag & drop line) |

## Covers — `CoverView`

`#NowCardCover`, `#CardCover`, `#RowCover`, `#PlaylistCover`, `#LibraryItemCover`…

`-meo-radius` (12px; `999px` = round), `-meo-crossfade` (400ms),
`-meo-zoom-on-change` (1 = off; 0.9 grows new covers from 90 %),
`-meo-zoom-easing`, `-meo-placeholder` (color), `-meo-placeholder-icon`,
`-meo-shadow` (`x y blur color`).

## Tilting cover — `#FullCover`

`-meo-tilt-max` (degrees, 12), `-meo-tilt-smoothing` (0.01 slow … 1 instant;
0.12), `-meo-hover-scale` (1.03), `-meo-hover-blur` (blur of the cover behind
the title on hover; smaller = blurrier), `-meo-info-color`,
`-meo-info-font-family`, `-meo-shadow-color`, `-meo-shadow-size` (28px),
`-meo-radius`, `-meo-placeholder`, `-meo-placeholder-icon`.

The fullscreen player's play button is `#FullPlay` and the bar with the
other buttons `#FullBar`.

## Potion background — `#FullPlayer`

`-meo-potion` (true: the cover's three main colors swirl like a potion),
`-meo-potion-speed` (1), `-meo-potion-fps` (30; 0 = the screen's refresh
rate), `-meo-potion-strength` (0..1). With the potion off, the blurred cover
(`-meo-backdrop-image-*`) is used.

## Seek and volume bars — `Slider`

`#NowCardSeek`, `#FullSeekBar`, `#FullVolumeBar`:

`-meo-track-color`, `-meo-fill-color`, `-meo-fill-color-2` (gradient end),
`-meo-handle-color`, `-meo-handle-size` (12px), `-meo-height` (4px),
`-meo-hover-height`, `-meo-radius`, `-meo-handle-on-hover` (true),
`-meo-smooth` (value easing, 0).

## Lyrics — `LyricsView` (`#FullLyrics`)

| property | |
|---|---|
| `-meo-font-family`, `-meo-font-size` (28px), `-meo-font-weight` (700) | text |
| `-meo-color` | lines not being sung |
| `-meo-past-color` | lines already sung |
| `-meo-active-color` | sung part of the active line |
| `-meo-upcoming-color` | unsung part of the active line |
| `-meo-word-animation` | word-synced lyrics: `fill` · `fade` · `bounce` · `glow` · `font` · `none` |
| `-meo-sung-font-family`, `-meo-sung-font-weight`, `-meo-sung-italic`, `-meo-sung-underline` | the font a sung word switches to (`font`) |
| `-meo-back-bg`, `-meo-back-color` | the "Back to the current line" button |
| `-meo-word-fill` | `gradient` · `solid` (for `fill`) |
| `-meo-fill-softness` | gradient edge (10px) |
| `-meo-line-animation` | line-synced lyrics: `fade` · `none` — the whole line lights up at once |
| `-meo-line-sync-fill` | `true` sweeps line-synced lines left to right instead (false) |
| `-meo-glow-color`, `-meo-glow-radius` | glow behind sung words (0 = off) |
| `-meo-word-lift` | how much a sung word rises |
| `-meo-active-scale`, `-meo-inactive-scale`, `-meo-scale-duration` | line sizes |
| `-meo-scroll-duration`, `-meo-scroll-easing` | auto scroll |
| `-meo-focus-position` | where the active line sits (0 top … 1 bottom; 0.35) |
| `-meo-distance-fade` | extra fade per line away from the active one |
| `-meo-align`, `-meo-opposite-align` | `left` · `center` · `right` (duets use the opposite side) |
| `-meo-line-spacing`, `-meo-padding` | spacing |
| `-meo-bg-vocal-color`, `-meo-bg-vocal-scale` | background vocals |
| `-meo-interlude-dots`, `-meo-dot-color` | breathing dots in instrumental gaps |
| `-meo-credit-color`, `-meo-hover-color`, `-meo-empty-color` | credits, hovered line (click to seek), messages |

## Current lyric line — `LyricLine` (`#CurrentLyric`)

`-meo-height` (56px), `-meo-font-family`, `-meo-font-size` (17px),
`-meo-font-weight` (700), `-meo-active-color`, `-meo-align`,
`-meo-swipe` (`up` · `down` · `left` · `right` · `fade`),
`-meo-swipe-duration` (420ms), `-meo-swipe-easing`, `-meo-swipe-distance`.

## Visualizer — `#FullVisualizer`

`-meo-line-color`, `-meo-fill-color`, `-meo-line-width` (3px), `-meo-bands`
(48), `-meo-amplitude` (0.8 of the height), `-meo-mirror` (false).

## Settings switch — `#Switch`

`-meo-track-color`, `-meo-on-color`, `-meo-knob-color`.
