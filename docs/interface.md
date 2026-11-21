# Interface map

Every part of the window has an **object name** (`#Name` in CSS) or a
**role** (`[role="…"]`). The painted widgets read `-meo-*` properties — see
[themes/properties.md](themes/properties.md). In developer mode, **F12** shows
the name, role and properties of the widget under the mouse.

## Window

| object | what it is |
|---|---|
| `#Root` | the window background (a painted Backdrop) |
| `#TopBar` | title bar row; drag to move, double-click to maximize |
| `#Logo` | the logo button (painted; `-meo-logo-color`, `-meo-logo-size`) |
| `#LogoMenu` | the menu that slides out of the logo (`-meo-open-duration`, `-meo-open-easing`) |
| `#NavHome`, `#NavSearch`, `#NavLibrary`, `#NavQueue`, `#NavSettings`, `#NavDev` | menu icons, `[role="nav"]` |
| `#LogoMenuHide` | the arrow that hides the menu |
| `#TopTitle` | the name of the current page |
| `#WinMinimize`, `#WinMaximize`, `#WinClose` | window buttons, `[role="window"]` |
| `#Body` | everything below the top bar |
| `#ResizeGrip` | invisible borders used to resize the frameless window |
| `#Toast` | short messages |
| `#FpsMeter` | frame counter (developer mode) |
| `#Tour`, `#TourBubble`, `#TourTitle`, `#TourText` | the tutorial |

## Left panel

| object | what it is |
|---|---|
| `#LeftPanel` | the panel (`-meo-width`) |
| `#LibraryHeading`, `#NewPlaylist`, `#ImportPlaylist` | header and its buttons |
| `#LibraryNav` | the scrollable list; rows are `#LibraryItem` with `#LibraryItemCover` |
| `#CurrentLyric` | the line being sung (LyricLine: swipe animation) |
| `#NowCard` | the player card |
| `#NowCardCover`, `#NowCardTitle`, `#NowCardArtist`, `#NowCardLike`, `#NowCardSeek` | its parts |
| `#NowCardPrev`, `#NowCardPlay`, `#NowCardNext`, `#NowCardLyrics`, `#NowCardQueue`, `#NowCardVolume` | its buttons |

## Pages (`#Pages`)

| page | objects |
|---|---|
| Home `#HomePage` | `#Greeting`, `#HomeSearch`, rows `#RecentRow`, `#TopRow`, `#RecommendedRow`, `#PopularRow` (cards `#Card` with `#CardCover`), `#HomeSource` |
| Search `#SearchPage` | `#SearchBox`, `#SourcePicker`, `#SearchInfo`, `#SearchHistory` (buttons `[role="chip-button"]`, `#SearchHistoryClear`), `#SearchList` |
| Library `#LibraryPage` | cards `QFrame[role="card"]#PlaylistCard` with `#PlaylistCover` |
| Playlist `#PlaylistPage` | `#PlaylistHeaderCover`, `#PlaylistTitle`, `#PlaylistSub`, `#PlaylistTracks` |
| Queue `#QueuePage` | `#QueueList` |
| Settings `#SettingsPage` | `#SettingsNav` (buttons `[role="settings-nav"]`), `#SettingsStack`, rows `#SettingRow` with `[role="setting-title"]`, `[role="setting-desc"]`, `#Switch`, `#ColorSwatch`, `#SettingReset` |
| Developer `#DevPage` | `#DevTabs`, `#DevText` |

Song rows (all lists): `#TrackRow` (property `playing="true"` on the one
playing), `#RowDrag`, `#RowCover`, `[role="row-title"]`, `[role="row-sub"]`,
`#RowLike`, `[role="row-meta"]`, `#RowMore`.

## Fullscreen player (`#FullPlayer`)

`#FullClose`, `#FullVideo` (button and video area), `#FullWindow`,
`#FullCenter`, `#FullCover` (TiltCover: title and artist on hover), `#FullPlay`,
`#FullBar` with `#FullPrev`, `#FullNext`, `#FullShuffle`, `#FullRepeat`,
`#FullQueue`, `#FullLyricsToggle`, `#FullArtist`, `#FullLike`,
`#FullVolumeIcon`, `#FullSleep`, `#FullMore`; `#FullSeekBar`,
`#FullTimeElapsed`, `#FullTimeTotal`; the side panel `#FullSide` with
`#FullRight` (stack), `#FullLyrics` (LyricsView), `#FullVisualizer`,
`#FullLyricsStatus`, `#FullLyricsRetry`. Tray menu: `#TrayMenu`.

## Roles used by labels and buttons

`title`, `subtitle`, `heading`, `hint`, `time`, `kbd`, `card-title`,
`card-sub`, `card-error`, `chip`, `row-title`, `row-sub`, `row-meta`,
`control` (round icon buttons), `primary`, `danger`, `nav`, `window`,
`settings-nav`, `card` (QFrame).

Layouts are part of the app (not plugins). To rearrange things, hide or
resize them with CSS (`-meo-width`, `min-width`, `max-height: 0`…).
